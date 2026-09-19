package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"zhonghuawenhua_backend/internal/api"
	"zhonghuawenhua_backend/internal/config"
	"zhonghuawenhua_backend/internal/model"
	"zhonghuawenhua_backend/pkg/ai"
	"zhonghuawenhua_backend/pkg/grading"
)

// WriteThoughtsService 「写想法」节点的状态获取、AI 评分提交业务。
//
// 数据流：
//   - 题目定义存于 node_contents.params_json（结构见 WriteThoughtsParams）
//   - 学生每次 submit 插入 node_submissions 一行（保留全部历史）
//   - 进度缓存在 node_progress（attempt_count/error_count/revealed/completed）
//   - 草稿存于 node_progress.draft_json 的 write-thoughts 分区
//
// 评分逻辑：使用 AI（write-thoughts.md 提示词）对主观题评分，由后端根据失败次数
// 控制 instruction_mode（方向指引 / 方法引导 / 完整示范），次数上限由题目 MaxErrors 决定。

// writeThoughtsLessonMap questionID 到课文标题的映射。
// q1 = 《纸的发明》, q2 = 《赵州桥》, q3 = 《一幅名扬中外的画》
var writeThoughtsLessonMap = map[string]string{
	"q1": "《纸的发明》",
	"q2": "《赵州桥》",
	"q3": "《一幅名扬中外的画》",
}

// loadWriteThoughtsParams 加载写想法节点参数。
func loadWriteThoughtsParams(ctx context.Context, repos *Repositories, nodeID int64) (*WriteThoughtsParams, error) {
	var p WriteThoughtsParams
	if err := loadNodeParams(ctx, repos, nodeID, &p); err != nil {
		return nil, err
	}
	if len(p.Questions) == 0 {
		return nil, ErrNodeContentNotConfigured
	}
	// 填充默认值
	for i := range p.Questions {
		if p.Questions[i].MaxErrors == 0 {
			p.Questions[i].MaxErrors = 3
		}
	}
	return &p, nil
}

// WriteThoughtsService 「写想法」节点业务服务。
type WriteThoughtsService struct {
	repos   *Repositories
	llm     ai.LLMClient
	prompts config.PromptsConfig
}

// NewWriteThoughtsService 创建 WriteThoughtsService。
func NewWriteThoughtsService(repos *Repositories, llm ai.LLMClient, prompts config.PromptsConfig) *WriteThoughtsService {
	return &WriteThoughtsService{repos: repos, llm: llm, prompts: prompts}
}

// GetParams 获取「写想法」节点静态参数（页面固定信息）。
// 返回标题、说明、导入视频/气泡文字，不含题目定义（题目在 GetState 中返回）。
// 同时记录学生进入节点的时间（用于 getending 首条耗时估算），首次进入后不再覆盖。
func (s *WriteThoughtsService) GetParams(ctx context.Context, classID, nodeID, userID int64) (*api.WriteThoughtParams, error) {
	if err := s.repos.EnsureClassMember(ctx, classID, userID); err != nil {
		return nil, err
	}
	s.recordEntryTime(ctx, classID, nodeID, userID)
	params, err := loadWriteThoughtsParams(ctx, s.repos, nodeID)
	if err != nil {
		return nil, err
	}
	resp := &api.WriteThoughtParams{
		Title:    params.Title,
		Subtitle: params.Subtitle,
	}
	if len(params.Description) > 0 {
		resp.Description = params.Description
	}
	resp.IntroVideo = params.IntroVideo
	resp.IntroBubbleText = params.IntroBubbleText
	return resp, nil
}

// GetState 获取「写想法」节点状态。
//
// 返回所有题目定义 + 每道题的提交历史（submitLogs）。
func (s *WriteThoughtsService) GetState(ctx context.Context, classID, nodeID, userID int64) (*api.WriteThoughtState, error) {
	if err := s.repos.EnsureClassMember(ctx, classID, userID); err != nil {
		return nil, err
	}
	// 1. 加载题目定义
	params, err := loadWriteThoughtsParams(ctx, s.repos, nodeID)
	if err != nil {
		return nil, err
	}

	// 2. 加载该用户该节点的全部提交记录
	submissions, err := s.repos.NodeSubmission.ListByUserNode(ctx, userID, nodeID, 100, 0)
	if err != nil {
		return nil, fmt.Errorf("list submissions: %w", err)
	}

	// 3. 按题目分组组装 submitLogs
	// submissions 默认按 submitted_at DESC 返回，submitLogs 也按此顺序
	logsByQ := make(map[string][]api.WriteThoughtSubmitResult, len(params.Questions))
	for _, sub := range submissions {
		if sub.QuestionID == nil {
			continue
		}
		qid := *sub.QuestionID
		// 仅保留该节点的写想法提交（submit_type 为 submit 或 get-answer）
		if sub.SubmitType != "submit" && sub.SubmitType != "get-answer" {
			continue
		}
		logsByQ[qid] = append(logsByQ[qid], sub.ToThoughtResp())
	}

	// 4. 组装响应
	state := &api.WriteThoughtState{}
	for _, q := range params.Questions {
		q := q // 捕获循环变量
		item := struct {
			ID          string                         `json:"id"`
			Placeholder *string                        `json:"placeholder,omitempty"`
			SubmitLogs  []api.WriteThoughtSubmitResult `json:"submitLogs"`
			Title       string                         `json:"title"`
		}{
			ID:         q.ID,
			Title:      q.Title,
			SubmitLogs: logsByQ[q.ID],
		}
		if q.Placeholder != "" {
			item.Placeholder = &q.Placeholder
		}
		if item.SubmitLogs == nil {
			item.SubmitLogs = []api.WriteThoughtSubmitResult{}
		}
		state.Questions = append(state.Questions, item)
	}
	return state, nil
}

// Submit 提交批改（异步 AI 评分）。
//
// 提交类型:
//   - submit: 调用 AI 评分；不合格时按失败次数切换 instruction_mode，次数达上限后揭示参考答案并标记完成
//   - get-answer: 使用 AI 生成个性化参考答案，标记为完成
//
// 该接口为异步设计：立即插入 is_processing=true 的提交记录并返回 submitId，
// 后台 goroutine 完成 AI 评分后回写结果，前端通过 GetSubmission 轮询 is_processing 获取最终结果。
// 通用提交/回写/进度基础逻辑见 async_submit.go 的 SubmitAsync。
func (s *WriteThoughtsService) Submit(ctx context.Context, classID, nodeID, userID int64, req SubmitRequest) (*api.AsyncSubmitResp, error) {
	// 0. 防御性校验：确认用户是该班级成员
	if err := s.repos.EnsureClassMember(ctx, classID, userID); err != nil {
		return nil, err
	}
	// 0.5 提交类型枚举校验：非法类型不得绕过空输入/上限/幂等校验
	if !isValidSubmitType(req.Type) {
		return nil, ErrInvalidSubmitType
	}
	// 1. 加载题目定义
	params, err := loadWriteThoughtsParams(ctx, s.repos, nodeID)
	if err != nil {
		return nil, err
	}
	q := params.FindQuestion(req.QuestionID)
	if q == nil {
		return nil, ErrQuestionNotFound
	}

	// 强类型校验：submit 类型必须携带非空作答内容，避免写入空 payload
	if req.Type == "submit" && strings.TrimSpace(req.Input) == "" {
		return nil, ErrEmptyInput
	}

	// 后端兜底：submit 类型若该题历史失败次数已达上限（提交超过三次），
	// 直接拒绝，不调用 AI、不写入提交记录。
	maxErrors := getMaxErrors(q.MaxErrors)
	if req.Type == "submit" && s.countQuestionErrors(ctx, userID, nodeID, req.QuestionID) >= maxErrors {
		return nil, ErrSubmitLimitReached
	}

	var payload []byte
	if req.Input != "" {
		payload, _ = json.Marshal(map[string]string{"input": req.Input})
	}

	// 2. 通用异步提交：立即返回 submitId，后台 goroutine 执行 AI 业务并回写
	submitID, err := SubmitAsync(ctx, s.repos, AsyncSubmitTask{
		ClassID:     classID,
		NodeID:      nodeID,
		UserID:      userID,
		NodeType:    model.NodeTypeWriteThoughts,
		QuestionID:  req.QuestionID,
		SubmitType:  req.Type,
		PayloadJSON: payload,
		Process: func(ctx context.Context) (*AsyncSubmitResult, error) {
			return s.processWriteThoughtsAnswer(ctx, nodeID, userID, params, q, req)
		},
		FinalizeProgress: func(ctx context.Context, sub *model.NodeSubmission) error {
			return s.finalizeWriteThoughtsProgress(ctx, classID, nodeID, userID, params, sub)
		},
		LLM:     s.llm,
		Prompts: s.prompts,
	})
	if err != nil {
		return nil, err
	}

	tip := "提交成功"
	return &api.AsyncSubmitResp{SubmitID: submitID, Tip: &tip}, nil
}

// processWriteThoughtsAnswer 执行写感想节点特有的 AI 业务（评分/生成参考答案），返回回写结果。
// 在 SubmitAsync 启动的后台 goroutine 中调用。
func (s *WriteThoughtsService) processWriteThoughtsAnswer(ctx context.Context, nodeID, userID int64, params *WriteThoughtsParams, q *WriteThoughtQuestionDef, req SubmitRequest) (*AsyncSubmitResult, error) {
	maxErrors := getMaxErrors(q.MaxErrors)

	if req.Type == "get-answer" {
		// get-answer: AI 生成个性化参考答案
		refAnswer, err := s.generateReferenceAnswer(ctx, params, q, userID, req.Input)
		if err != nil {
			return nil, err
		}
		return &AsyncSubmitResult{
			IsPassed:        false,
			IsCompleted:     true,
			Revealed:        true,
			ReferenceAnswer: refAnswer,
		}, nil
	}

	// submit: 统计历史失败次数，AI 评分
	prevFail := s.countQuestionErrors(ctx, userID, nodeID, req.QuestionID)
	instructionMode := s.modeForFailCount(prevFail, maxErrors)

	grade, err := s.gradeByAI(ctx, params, q, userID, req.Input, instructionMode)
	if err != nil {
		return nil, err
	}

	// 每次提交都回写 AI 反馈（通过给鼓励、失败给讲解）
	res := &AsyncSubmitResult{IsPassed: grade.IsPassed, Feedback: grade.Feedback}
	if grade.IsPassed {
		res.IsCompleted = true
	} else {
		// 本次失败后累计失败次数，达到上限时揭示参考答案并标记完成
		curFail := prevFail + 1
		if curFail >= maxErrors {
			res.IsCompleted = true
			res.Revealed = true
			// 参考答案优先取 AI 生成结果；AI 未返回（返回空或未配置 LLM）时兜底使用 params_json 配置的标准答案
			res.ReferenceAnswer = grade.ReferenceAnswer
			if res.ReferenceAnswer == "" {
				res.ReferenceAnswer = q.ReferenceAnswer
			}
		}
	}
	return res, nil
}

// finalizeWriteThoughtsProgress 在 AI 结果回写后更新写感想进度的聚合字段（error_count / revealed / completed）。
func (s *WriteThoughtsService) finalizeWriteThoughtsProgress(ctx context.Context, classID, nodeID, userID int64, params *WriteThoughtsParams, sub *model.NodeSubmission) error {
	prog, err := s.repos.NodeProgress.GetByNodeUser(ctx, nodeID, userID)
	if err != nil {
		prog = nil
	}
	if prog == nil {
		prog = &model.NodeProgress{
			ClassID:      classID,
			NodeID:       nodeID,
			UserID:       userID,
			LastSubmitID: &sub.ID,
		}
	}
	prog.ErrorCount = s.countAllErrors(ctx, userID, nodeID)
	prog.Revealed = prog.Revealed || sub.Revealed
	if s.checkAllCompleted(ctx, params, userID, nodeID) {
		prog.Completed = true
	}
	if prog.ID == 0 {
		_, err = s.repos.NodeProgress.Create(ctx, prog)
	} else {
		err = s.repos.NodeProgress.Update(ctx, prog)
	}
	return err
}

// modeForFailCount 根据历史失败次数决定 instruction_mode。
// 次数说明（N = maxErrors，即第 N 次提供答案）：
//   - 失败次数 < N-2：方向指引（反馈 + 提示，不询问）
//   - 失败次数 == N-2：方法引导（反馈 + 提示 + 询问是否需要答案）
//   - 失败次数 >= N-1：完整示范（直接提供参考答案）
func (s *WriteThoughtsService) modeForFailCount(prevFail, maxErrors int) string {
	switch {
	case prevFail >= maxErrors-1:
		return "完整示范"
	case prevFail >= maxErrors-2:
		return "方法引导"
	default:
		return "方向指引"
	}
}

// gradeByAI 调用 AI 评估学生答案。
// 若 AI 未配置（llm 为 nil），降级为简单文本匹配批改（GradeTextSimple）。
func (s *WriteThoughtsService) gradeByAI(ctx context.Context, params *WriteThoughtsParams, q *WriteThoughtQuestionDef, userID int64, input, instructionMode string) (*grading.GradeResult, error) {
	if s.llm == nil {
		// AI 未配置，降级为简单文本匹配（仅判定是否通过；
		// 参考答案由 AI 生成，此处不返回数据库答案）
		res := grading.GradeTextSimple(input, q.ReferenceAnswer)
		res.ReferenceAnswer = ""
		return &res, nil
	}
	prompt := s.writeThoughtsPrompt()
	aiInput := s.buildAIInput(params, q, userID, input, instructionMode)
	res, err := s.callAI(ctx, prompt, aiInput)
	if err != nil {
		return nil, err
	}
	return &grading.GradeResult{
		IsPassed:        res.IsPass,
		Feedback:        res.FeedbackText,
		ReferenceAnswer: res.ReferenceAnswer,
	}, nil
}

// generateReferenceAnswer get-answer 时为学生生成个性化参考答案。
// 参考答案优先由 AI 生成；AI 未配置或未返回时兜底使用 params_json 配置的标准答案。
func (s *WriteThoughtsService) generateReferenceAnswer(ctx context.Context, params *WriteThoughtsParams, q *WriteThoughtQuestionDef, userID int64, input string) (string, error) {
	ref := ""
	if s.llm != nil {
		prompt := s.writeThoughtsPrompt()
		aiInput := s.buildAIInput(params, q, userID, input, "完整示范")
		res, err := s.callAI(ctx, prompt, aiInput)
		if err != nil {
			return "", err
		}
		ref = res.ReferenceAnswer
	}
	if ref == "" {
		ref = q.ReferenceAnswer
	}
	return ref, nil
}

// buildAIInput 组装传给 AI 的 JSON 数据。
func (s *WriteThoughtsService) buildAIInput(params *WriteThoughtsParams, q *WriteThoughtQuestionDef, userID int64, input, instructionMode string) writeThoughtAIInput {
	// 根据 questionID 获取课文标题，兜底使用题目标题
	lesson := writeThoughtsLessonMap[q.ID]
	if lesson == "" {
		lesson = q.Title
	}
	aiInput := writeThoughtAIInput{
		CurrentLesson:   lesson,
		QuestionTitle:   q.Title,
		InstructionMode: instructionMode,
		StudentInput:    input,
	}
	// 加载学生信息（性别/城市/姓名），失败时保持空值
	if u, err := s.repos.User.GetByID(context.Background(), userID); err == nil && u != nil {
		if u.RealName != nil {
			aiInput.StudentName = *u.RealName
		}
		if u.Gender != nil {
			aiInput.Gender = *u.Gender
		}
		if u.City != nil {
			aiInput.City = *u.City
		}
	}
	return aiInput
}

// writeThoughtsPrompt 返回写想法评分提示词（write-thoughts.md）。
func (s *WriteThoughtsService) writeThoughtsPrompt() string {
	if p, ok := s.prompts.Types["write-thoughts"]; ok && p != "" {
		return p
	}
	return ""
}

// callAI 调用 LLM，解析 JSON 结果。
func (s *WriteThoughtsService) callAI(ctx context.Context, prompt string, aiInput writeThoughtAIInput) (*writeThoughtAIResult, error) {
	var res writeThoughtAIResult
	if err := callAIWithJSON(ctx, s.llm, prompt, aiInput, &res); err != nil {
		return nil, fmt.Errorf("call ai: %w", err)
	}
	return &res, nil
}

// countQuestionErrors 统计某题已累计的错误次数。
func (s *WriteThoughtsService) countQuestionErrors(ctx context.Context, userID, nodeID int64, questionID string) int {
	subs, err := s.repos.NodeSubmission.ListByUserNode(ctx, userID, nodeID, 100, 0)
	if err != nil {
		return 0
	}
	count := 0
	for _, sub := range subs {
		if sub.QuestionID == nil || *sub.QuestionID != questionID {
			continue
		}
		if sub.SubmitType != "submit" {
			continue
		}
		if sub.IsPassed != nil && !*sub.IsPassed {
			count++
		}
	}
	return count
}

// countAllErrors 统计该节点该用户全部题目的错误次数。
func (s *WriteThoughtsService) countAllErrors(ctx context.Context, userID, nodeID int64) int {
	subs, err := s.repos.NodeSubmission.ListByUserNode(ctx, userID, nodeID, 100, 0)
	if err != nil {
		return 0
	}
	count := 0
	for _, sub := range subs {
		if sub.SubmitType != "submit" {
			continue
		}
		if sub.IsPassed != nil && !*sub.IsPassed {
			count++
		}
	}
	return count
}

// checkAllCompleted 检查所有题目是否都已完成（正确或达到最大错误次数或已 get-answer）。
func (s *WriteThoughtsService) checkAllCompleted(ctx context.Context, params *WriteThoughtsParams, userID, nodeID int64) bool {
	subs, err := s.repos.NodeSubmission.ListByUserNode(ctx, userID, nodeID, 100, 0)
	if err != nil {
		return false
	}
	completedByQ := make(map[string]bool, len(params.Questions))
	for _, sub := range subs {
		if sub.QuestionID == nil {
			continue
		}
		qid := *sub.QuestionID
		if sub.IsCompleted != nil && *sub.IsCompleted {
			completedByQ[qid] = true
		}
	}
	for _, q := range params.Questions {
		if !completedByQ[q.ID] {
			return false
		}
	}
	return true
}

// GetSubmission 按 submitID 查询单条提交记录。
// 防御性要求：仅允许提交记录所属用户本人查询，防止越权读取他人作答与批改结果。
func (s *WriteThoughtsService) GetSubmission(ctx context.Context, nodeID, userID int64, submitID string) (*api.WriteThoughtSubmitResult, error) {
	sub, err := s.repos.NodeSubmission.GetByCode(ctx, submitID)
	if err != nil {
		return nil, ErrQuestionNotFound
	}
	if sub.NodeID != nodeID || sub.NodeType != model.NodeTypeWriteThoughts || sub.UserID != userID {
		return nil, ErrQuestionNotFound
	}
	resp := sub.ToThoughtResp()
	return &resp, nil
}

// recordEntryTime 记录学生首次进入写感想节点的时间到 draft_json，已记录则不覆盖。
func (s *WriteThoughtsService) recordEntryTime(ctx context.Context, classID, nodeID, userID int64) {
	var d writeThoughtsDraft
	if err := loadDraftPartition(ctx, s.repos, nodeID, userID, DraftKeyWriteThoughts, &d); err == nil && d.EnteredAt != nil {
		return
	}
	now := time.Now()
	d.EnteredAt = &now
	_ = saveDraftPartition(ctx, s.repos, classID, nodeID, userID, DraftKeyWriteThoughts, &d)
}
