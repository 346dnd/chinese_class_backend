package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"zhonghuawenhua_backend/internal/api"
	"zhonghuawenhua_backend/internal/config"
	"zhonghuawenhua_backend/internal/model"
	"zhonghuawenhua_backend/pkg/ai"
	"zhonghuawenhua_backend/pkg/grading"
)

// loadInitialInsightParams 加载初步感悟节点参数。
func loadInitialInsightParams(ctx context.Context, repos *Repositories, nodeID int64) (*InitialInsightParams, error) {
	var p InitialInsightParams
	if err := loadNodeParams(ctx, repos, nodeID, &p); err != nil {
		return nil, err
	}
	if len(p.Questions) == 0 {
		return nil, ErrNodeContentNotConfigured
	}
	return &p, nil
}

// InitialInsightService 「初步感悟（填空题）」节点的状态获取、提交批改业务。
//
// 数据流：
//   - 题目定义存于 node_contents.params_json（结构见 InitialInsightParams）
//   - 每道题包含若干 blank 填空位，每个 blank 独立提交
//   - 学生每次 submit 一个 blank 的答案，插入 node_submissions 一行
//   - GetState 返回每道题的 Content（含 blank 的 state 实时计算）+ SubmitLogs
type InitialInsightService struct {
	repos   *Repositories
	llm     ai.LLMClient
	prompts config.PromptsConfig // 提示词配置（initial_comprehensive.md 综合评价）
}

// NewInitialInsightService 创建 InitialInsightService。
func NewInitialInsightService(repos *Repositories, llm ai.LLMClient, prompts config.PromptsConfig) *InitialInsightService {
	return &InitialInsightService{repos: repos, llm: llm, prompts: prompts}
}

// initialInsightLessonMap questionID 到课文标题的映射（供 AI 辅导定位语境）。
// q1 = 《赵州桥》, q2 = 《一幅名扬中外的画》
var initialInsightLessonMap = map[string]string{
	"q1": "《赵州桥》",
	"q2": "《一幅名扬中外的画》",
}

// GetParams 获取「初步感悟」节点静态参数（页面固定信息）。
// 返回标题、说明、导入视频/气泡文字，不含题目定义（题目在 GetState 中返回）。
// 同时记录学生进入节点的时间（用于 getending 首空耗时估算），首次进入后不再覆盖。
func (s *InitialInsightService) GetParams(ctx context.Context, classID, nodeID, userID int64) (*api.InitialImpressionsParams, error) {
	if err := s.repos.EnsureClassMember(ctx, classID, userID); err != nil {
		return nil, err
	}
	s.recordEntryTime(ctx, classID, nodeID, userID)
	params, err := loadInitialInsightParams(ctx, s.repos, nodeID)
	if err != nil {
		return nil, err
	}
	resp := &api.InitialImpressionsParams{
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

// recordEntryTime 记录学生首次进入节点的时间到 draft_json，已记录则不覆盖。
func (s *InitialInsightService) recordEntryTime(ctx context.Context, classID, nodeID, userID int64) {
	var d initialInsightDraft
	if err := loadDraftPartition(ctx, s.repos, nodeID, userID, DraftKeyInitialInsight, &d); err == nil && d.EnteredAt != nil {
		return
	}
	now := time.Now()
	d.EnteredAt = &now
	_ = saveDraftPartition(ctx, s.repos, classID, nodeID, userID, DraftKeyInitialInsight, &d)
}

// GetEnding 获取「初步感悟」节点结束语（AI 综合评价）。
//
// 基于学生在节点上的全部填空提交记录，按 initial_comprehensive.md 提示词生成综合评价，
// 返回其中的教师口吻总体评价（teacher_evaluation）。同步生成，isProcessing 恒为 false。
// 若 AI 未配置（llm 为空），回退到 params_json 中的静态 ending.comment。
func (s *InitialInsightService) GetEnding(ctx context.Context, classID, nodeID, userID int64) (*InitialInsightEndingResp, error) {
	if err := s.repos.EnsureClassMember(ctx, classID, userID); err != nil {
		return nil, err
	}
	params, err := loadInitialInsightParams(ctx, s.repos, nodeID)
	if err != nil {
		return nil, err
	}
	subs, err := s.repos.NodeSubmission.ListByUserNode(ctx, userID, nodeID, 500, 0)
	if err != nil {
		return nil, fmt.Errorf("list submissions: %w", err)
	}

	// 进入节点时间（首空耗时基准）
	var d initialInsightDraft
	_ = loadDraftPartition(ctx, s.repos, nodeID, userID, DraftKeyInitialInsight, &d)
	var entryTime time.Time
	if d.EnteredAt != nil {
		entryTime = *d.EnteredAt
	}

	// AI 未配置：回退静态结束语
	if s.llm == nil {
		resp := &InitialInsightEndingResp{IsProcessing: false}
		if params.Ending != nil {
			resp.Comment = params.Ending.Comment
		}
		return resp, nil
	}

	input := s.buildComprehensiveInput(ctx, params, subs, entryTime, userID)
	if len(input.Records) == 0 {
		// 尚无任何提交，无法生成评价
		return &InitialInsightEndingResp{IsProcessing: false}, nil
	}
	result, err := s.callComprehensiveAI(ctx, input)
	if err != nil {
		return nil, err
	}
	return &InitialInsightEndingResp{IsProcessing: false, Comment: result.TeacherEvaluation}, nil
}

// GetState 获取「初步感悟」节点状态。
//
// 返回每道题的题目内容（含 blank 的实时 state）+ 该题的 submitLogs。
// blank.state 计算规则：
//   - PENDING: 无提交记录
//   - CORRECT: 最近一次提交正确
//   - WRONG: 最近一次提交错误且未达最大错误次数
//   - RETRYING: 错误次数达到 maxErrors-1（再错一次就完成）
//   - UNANSWERED: 已通过 get-answer 揭示答案
func (s *InitialInsightService) GetState(ctx context.Context, classID, nodeID, userID int64) (*api.InitialInsightState, error) {
	if err := s.repos.EnsureClassMember(ctx, classID, userID); err != nil {
		return nil, err
	}
	// 1. 加载题目定义
	params, err := loadInitialInsightParams(ctx, s.repos, nodeID)
	if err != nil {
		return nil, err
	}

	// 3. 加载该用户该节点的全部提交记录
	submissions, err := s.repos.NodeSubmission.ListByUserNode(ctx, userID, nodeID, 200, 0)
	if err != nil {
		return nil, fmt.Errorf("list submissions: %w", err)
	}

	// 4. 按 (questionID, blankID) 分组统计
	type blankStat struct {
		lastCorrect   bool
		lastRevealed  bool
		errCount      int
		lastSubmitted string
		logs          []api.InitialImpressionsSubmitResult
	}
	stats := make(map[string]*blankStat) // key = questionID + ":" + blankID

	for _, sub := range submissions {
		if sub.QuestionID == nil {
			continue
		}
		// payload_json 应含 blankId 和 input
		var p map[string]string
		if len(sub.PayloadJSON) > 0 {
			_ = json.Unmarshal(sub.PayloadJSON, &p)
		}
		blankID := p["blankId"]
		if blankID == "" {
			continue
		}
		key := *sub.QuestionID + ":" + blankID
		stat, ok := stats[key]
		if !ok {
			stat = &blankStat{}
			stats[key] = stat
		}
		// 只统计 submit 类型
		if sub.SubmitType == "submit" {
			if sub.IsPassed != nil {
				if *sub.IsPassed {
					stat.lastCorrect = true
				} else {
					stat.lastCorrect = false
					stat.errCount++
				}
			}
		} else if sub.SubmitType == "get-answer" {
			stat.lastRevealed = true
		}
		stat.lastSubmitted = p["input"]
		// 构建提交日志
		log := api.InitialImpressionsSubmitResult{
			ID:            sub.SubmitIDStr(),
			IsProcessing:  sub.IsProcessing,
			BlankID:       blankID,
			SubmittedText: p["input"],
		}
		if sub.QuestionID != nil {
			log.QuestionID = *sub.QuestionID
		}
		log.IsPassed = sub.IsPassed
		log.IsCompleted = sub.IsCompleted
		log.Feedback = sub.Feedback
		log.ReferenceAnswer = sub.ReferenceAnswer
		stat.logs = append(stat.logs, log)
	}

	// 5. 组装响应
	state := &api.InitialInsightState{}
	for _, q := range params.Questions {
		q := q
		// 收集该题全部 submitLogs（合并所有 blank 的日志）
		var allLogs []interface{}
		contentItems := make([]api.InitialInsightState_Questions_Content_Item, 0, len(q.Content))
		for _, c := range q.Content {
			if c.Type == "text" && c.Text != nil {
				// 文本片段
				textContent := api.InitialInsightStateQuestionsContent0{
					Type: "text",
					Value: api.StyleOverridableText{
						Text: *c.Text,
					},
				}
				item := api.InitialInsightState_Questions_Content_Item{}
				if err := item.FromInitialInsightStateQuestionsContent0(textContent); err != nil {
					return nil, fmt.Errorf("marshal text content: %w", err)
				}
				contentItems = append(contentItems, item)
			} else if c.Type == "blank" && c.Blank != nil {
				// 填空位：根据提交记录计算 state
				key := q.ID + ":" + c.Blank.ID
				stat := stats[key]
				blankState := api.InitialInsightStateQuestionsContent1State("PENDING")
				var refAnswer *string
				if stat != nil {
					if stat.lastRevealed {
						blankState = "UNANSWERED"
						ra := c.Blank.ReferenceAnswer
						refAnswer = &ra
					} else if stat.lastCorrect {
						blankState = "CORRECT"
						ra := c.Blank.ReferenceAnswer
						refAnswer = &ra
					} else if stat.errCount > 0 {
						if stat.errCount >= getMaxErrors(c.Blank.MaxErrors) {
							blankState = "WRONG"
							ra := c.Blank.ReferenceAnswer
							refAnswer = &ra
						} else if stat.errCount >= getMaxErrors(c.Blank.MaxErrors)-1 {
							blankState = "RETRYING"
						} else {
							blankState = "WRONG"
						}
					}
					for _, l := range stat.logs {
						allLogs = append(allLogs, l)
					}
				}
				blankContent := api.InitialInsightStateQuestionsContent1{
					ID:    c.Blank.ID,
					Type:  "blank",
					State: blankState,
				}
				if c.Blank.Placeholder != "" {
					p := c.Blank.Placeholder
					blankContent.Placeholder = &p
				}
				if c.Blank.RetryPlaceholder != "" {
					p := c.Blank.RetryPlaceholder
					blankContent.RetryPlaceholder = &p
				}
				blankContent.ReferenceAnswer = refAnswer
				item := api.InitialInsightState_Questions_Content_Item{}
				if err := item.FromInitialInsightStateQuestionsContent1(blankContent); err != nil {
					return nil, fmt.Errorf("marshal blank content: %w", err)
				}
				contentItems = append(contentItems, item)
			}
		}
		// SubmitLogs 按提交时间排序（submissions 默认 DESC，所以已是最新在前）
		if allLogs == nil {
			allLogs = []interface{}{}
		}
		state.Questions = append(state.Questions, initialInsightQuestionDTO{
			ID:         q.ID,
			Title:      q.Title,
			Content:    contentItems,
			SubmitLogs: allLogs,
		})
	}
	return state, nil
}

// Submit 提交一个填空位的答案（异步 AI 批改）。
//
// 注意：新版 API 的 SubmitInitialInsightJSONBody 没有 type 字段，统一为 submit。
// get-answer 由独立接口或前端按需调用（当前未实现 get-answer 入口）。
//
// 异步设计：立即插入 is_processing=true 的提交记录并返回 submitId，
// 后台 goroutine 完成对错判定与 AI 辅导后回写，前端通过 GetSubmission 轮询 is_processing。
//
// 幂等保护：正常的作答（含答错，第 1、2 次）都会入库；
// 仅当该空已解决（已答对 或 已达错误上限、答案已自动填入）之后，重复提交不再入库，直接返回提示。
// 注意 errCount 是本次提交之前的错误次数，因此触发填入答案的那次答错仍会正常入库。
func (s *InitialInsightService) Submit(ctx context.Context, classID, nodeID, userID int64, req InitialInsightSubmitRequest) (*api.AsyncSubmitResp, error) {
	// 0. 防御性校验：确认用户是该班级成员
	if err := s.repos.EnsureClassMember(ctx, classID, userID); err != nil {
		return nil, err
	}
	// 1. 加载题目定义，查找对应 blank
	params, err := loadInitialInsightParams(ctx, s.repos, nodeID)
	if err != nil {
		return nil, err
	}
	q := params.FindQuestion(req.QuestionID)
	if q == nil {
		return nil, ErrQuestionNotFound
	}
	blank := findBlank(q, req.BlankID)
	if blank == nil {
		return nil, ErrQuestionNotFound
	}

	// 2. 幂等保护：已解决的空位不新建提交、不调用 AI
	errCount := s.countBlankErrors(ctx, userID, nodeID, req.QuestionID, req.BlankID)
	maxErrors := getMaxErrors(blank.MaxErrors)
	if s.hasPassedBlank(ctx, userID, nodeID, req.QuestionID, req.BlankID) || errCount >= maxErrors {
		tip := "该空位已填写完成"
		return &api.AsyncSubmitResp{SubmitID: "", Tip: &tip}, nil
	}

	// 3. payload 存 blankId + input
	payload, _ := json.Marshal(map[string]string{
		"blankId": req.BlankID,
		"input":   req.Input,
	})

	// 4. 通用异步提交：立即返回 submitId，后台完成批改 + AI 辅导（含按用户锁）
	submitID, err := SubmitAsync(ctx, s.repos, AsyncSubmitTask{
		ClassID:     classID,
		NodeID:      nodeID,
		UserID:      userID,
		NodeType:    model.NodeTypeInitialInsight,
		QuestionID:  req.QuestionID,
		SubmitType:  "submit",
		PayloadJSON: payload,
		Process: func(ctx context.Context) (*AsyncSubmitResult, error) {
			return s.processInitialInsightAnswer(ctx, nodeID, userID, q, blank, req)
		},
		FinalizeProgress: func(ctx context.Context, sub *model.NodeSubmission) error {
			return s.finalizeInitialInsightProgress(ctx, classID, nodeID, userID, params, sub)
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

// processInitialInsightAnswer 执行初步感悟空位的批改与 AI 辅导（在 SubmitAsync 后台 goroutine 中调用）。
//
// 判对错：答对标记完成；答错按本次是否为达上限的那次错误切换 feedback_mode
// （guidance=引导重填，explanation=结合答案讲解），达上限时揭示参考答案并标记完成。
// 错误辅导：AI 未配置或调用失败时返回空反馈，不阻断提交。
func (s *InitialInsightService) processInitialInsightAnswer(ctx context.Context, nodeID, userID int64, q *InitialInsightQuestionDef, blank *InitialInsightBlankDef, req InitialInsightSubmitRequest) (*AsyncSubmitResult, error) {
	maxErrors := getMaxErrors(blank.MaxErrors)
	// 本次提交之前的错误次数（当前提交尚未回写 IsPassed，不会被 countBlankErrors 统计）
	errCount := s.countBlankErrors(ctx, userID, nodeID, req.QuestionID, req.BlankID)

	grade := grading.GradeTextSimple(req.Input, blank.ReferenceAnswer)
	if grade.IsPassed {
		return &AsyncSubmitResult{IsPassed: true, IsCompleted: true}, nil
	}

	res := &AsyncSubmitResult{IsPassed: false}
	if errCount+1 >= maxErrors {
		// 达上限：结合答案讲解并直接填入答案，标记完成
		if fb, err := s.generateBlankFeedback(ctx, nodeID, q, blank, userID, req, "explanation"); err == nil && fb != "" {
			res.Feedback = fb
		}
		res.IsCompleted = true
		res.Revealed = true
		res.ReferenceAnswer = blank.ReferenceAnswer
	} else {
		// 未达上限：引导重填
		if fb, err := s.generateBlankFeedback(ctx, nodeID, q, blank, userID, req, "guidance"); err == nil && fb != "" {
			res.Feedback = fb
		}
	}
	return res, nil
}

// finalizeInitialInsightProgress 在批改回写后更新初步感悟进度聚合字段（error_count / completed）。
// attempt_count / last_submit_id 已在 SubmitAsync 事务内原子更新，此处不重复处理。
// completed 与写感想等其他节点对齐：所有题目的所有空位均已完成（答对或达上限揭示答案）时置 true，
// 供课堂详情页 nodeCompleted 读取（此前缺失导致已完成的初步感悟节点在课程列表中一直显示"未完成"）。
func (s *InitialInsightService) finalizeInitialInsightProgress(ctx context.Context, classID, nodeID, userID int64, params *InitialInsightParams, sub *model.NodeSubmission) error {
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
	prog.ErrorCount = s.countNodeErrors(ctx, userID, nodeID)
	if s.checkAllBlanksCompleted(ctx, params, userID, nodeID) {
		prog.Completed = true
	}
	if prog.ID == 0 {
		_, err = s.repos.NodeProgress.Create(ctx, prog)
	} else {
		err = s.repos.NodeProgress.Update(ctx, prog)
	}
	return err
}

// checkAllBlanksCompleted 检查该节点所有题目的所有空位是否都已完成（答对或达错误上限揭示答案）。
// 与 write_thoughts 的 checkAllCompleted 语义一致：按空位维度统计 is_completed 记录。
func (s *InitialInsightService) checkAllBlanksCompleted(ctx context.Context, params *InitialInsightParams, userID, nodeID int64) bool {
	subs, err := s.repos.NodeSubmission.ListByUserNode(ctx, userID, nodeID, 200, 0)
	if err != nil {
		return false
	}
	completedByBlank := make(map[string]bool)
	for _, sub := range subs {
		if sub.QuestionID == nil {
			continue
		}
		if sub.IsCompleted == nil || !*sub.IsCompleted {
			continue
		}
		var p map[string]string
		if len(sub.PayloadJSON) > 0 {
			_ = json.Unmarshal(sub.PayloadJSON, &p)
		}
		completedByBlank[*sub.QuestionID+":"+p["blankId"]] = true
	}
	for _, q := range params.Questions {
		for _, c := range q.Content {
			if c.Type == "blank" && c.Blank != nil {
				if !completedByBlank[q.ID+":"+c.Blank.ID] {
					return false
				}
			}
		}
	}
	return true
}

// countNodeErrors 统计该节点该用户全部错误提交次数（供进度聚合 error_count）。
func (s *InitialInsightService) countNodeErrors(ctx context.Context, userID, nodeID int64) int {
	subs, err := s.repos.NodeSubmission.ListByUserNode(ctx, userID, nodeID, 200, 0)
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

// countBlankErrors 统计某 blank 的错误次数。
func (s *InitialInsightService) countBlankErrors(ctx context.Context, userID, nodeID int64, questionID, blankID string) int {
	subs, err := s.repos.NodeSubmission.ListByUserNode(ctx, userID, nodeID, 200, 0)
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
		// 检查 payload_json 中的 blankId
		var p map[string]string
		if len(sub.PayloadJSON) > 0 {
			_ = json.Unmarshal(sub.PayloadJSON, &p)
		}
		if p["blankId"] != blankID {
			continue
		}
		if sub.IsPassed != nil && !*sub.IsPassed {
			count++
		}
	}
	return count
}

// hasPassedBlank 判断某 blank 是否已有答对记录。
func (s *InitialInsightService) hasPassedBlank(ctx context.Context, userID, nodeID int64, questionID, blankID string) bool {
	subs, err := s.repos.NodeSubmission.ListByUserNode(ctx, userID, nodeID, 200, 0)
	if err != nil {
		return false
	}
	for _, sub := range subs {
		if sub.QuestionID == nil || *sub.QuestionID != questionID {
			continue
		}
		if sub.SubmitType != "submit" {
			continue
		}
		var p map[string]string
		if len(sub.PayloadJSON) > 0 {
			_ = json.Unmarshal(sub.PayloadJSON, &p)
		}
		if p["blankId"] != blankID {
			continue
		}
		if sub.IsPassed != nil && *sub.IsPassed {
			return true
		}
	}
	return false
}

// boolPtr 返回 bool 指针。
func boolPtr(b bool) *bool {
	return &b
}

// GetSubmission 按 submitID 查询单条提交记录。
// 防御性要求：仅允许提交记录所属用户本人查询，防止越权读取他人作答与批改结果。
func (s *InitialInsightService) GetSubmission(ctx context.Context, nodeID, userID int64, submitID string) (*api.InitialImpressionsSubmitResult, error) {
	sub, err := s.repos.NodeSubmission.GetByCode(ctx, submitID)
	if err != nil {
		return nil, ErrQuestionNotFound
	}
	if sub.NodeID != nodeID || sub.NodeType != model.NodeTypeInitialInsight || sub.UserID != userID {
		return nil, ErrQuestionNotFound
	}
	// 从 payload 提取 blankId
	blankID := ""
	if len(sub.PayloadJSON) > 0 {
		var p map[string]string
		if json.Unmarshal(sub.PayloadJSON, &p) == nil {
			blankID = p["blankId"]
		}
	}
	resp := sub.ToInitialInsightResp(blankID)
	return &resp, nil
}

// findBlank 在题目定义中查找指定 blank。
func findBlank(q *InitialInsightQuestionDef, blankID string) *InitialInsightBlankDef {
	for i := range q.Content {
		if q.Content[i].Type == "blank" && q.Content[i].Blank != nil && q.Content[i].Blank.ID == blankID {
			return q.Content[i].Blank
		}
	}
	return nil
}

// blankIndexInQuestion 返回 blank 在题目中的序号（从 1 开始）。
func blankIndexInQuestion(q *InitialInsightQuestionDef, blankID string) int {
	idx := 0
	for _, c := range q.Content {
		if c.Type == "blank" && c.Blank != nil {
			idx++
			if c.Blank.ID == blankID {
				return idx
			}
		}
	}
	return idx
}

// generateBlankFeedback 调用 AI 生成当前空位的辅导讲解（initial_insight.md）。
// 按错误次数设置 feedback_mode：guidance=启发式引导，explanation=结合答案讲解。
// AI 未配置或调用失败时返回空串，不阻断提交。
func (s *InitialInsightService) generateBlankFeedback(ctx context.Context, nodeID int64, q *InitialInsightQuestionDef, blank *InitialInsightBlankDef, userID int64, req InitialInsightSubmitRequest, feedbackMode string) (string, error) {
	if s.llm == nil {
		return "", nil
	}
	prompt := s.blankPrompt()
	if prompt == "" {
		return "", fmt.Errorf("prompt not configured")
	}

	// 学生信息
	var name, gender string
	if u, err := s.repos.User.GetByID(ctx, userID); err == nil && u != nil {
		if u.RealName != nil {
			name = *u.RealName
		}
		if u.Gender != nil {
			gender = *u.Gender
		}
	}
	lesson := initialInsightLessonMap[req.QuestionID]
	if lesson == "" {
		lesson = q.Title
	}

	aiInput := initialInsightAIInput{
		StudentName:   name,
		Gender:        gender,
		CurrentLesson: lesson,
		QuestionID:    req.QuestionID,
		BlankIndex:    blankIndexInQuestion(q, req.BlankID),
		StudentAnswer: req.Input,
		FeedbackMode:  feedbackMode,
		CorrectAnswer: blank.ReferenceAnswer,
		History:       s.buildBlankHistory(ctx, nodeID, userID, q, req.BlankID),
	}

	var res initialInsightAIResult
	if err := callAIWithJSON(ctx, s.llm, prompt, aiInput, &res); err != nil {
		return "", fmt.Errorf("call blank ai: %w", err)
	}
	return res.FeedbackText, nil
}

// buildBlankHistory 组装当前题目内、当前空位之前答错的空位记录。
// 缺失的空位视为已答对（隐性正确），不进入 history；history 按空位顺序排列。
func (s *InitialInsightService) buildBlankHistory(ctx context.Context, nodeID, userID int64, q *InitialInsightQuestionDef, currentBlankID string) []initialInsightHistoryItem {
	subs, err := s.repos.NodeSubmission.ListByUserNode(ctx, userID, nodeID, 200, 0)
	if err != nil {
		return nil
	}
	curIdx := blankIndexInQuestion(q, currentBlankID)

	// 记录每个在前空位最新一次错误作答（ListByUserNode 默认按时间倒序，首次出现即最新）
	latestWrong := map[string]string{}
	for _, sub := range subs {
		if sub.QuestionID == nil || *sub.QuestionID != q.ID {
			continue
		}
		if sub.SubmitType != "submit" || sub.IsPassed == nil || *sub.IsPassed {
			continue
		}
		var p map[string]string
		if len(sub.PayloadJSON) > 0 {
			_ = json.Unmarshal(sub.PayloadJSON, &p)
		}
		bid := p["blankId"]
		if bid == "" {
			continue
		}
		if _, ok := latestWrong[bid]; !ok {
			latestWrong[bid] = p["input"]
		}
	}

	var history []initialInsightHistoryItem
	for _, c := range q.Content {
		if c.Type != "blank" || c.Blank == nil {
			continue
		}
		bid := c.Blank.ID
		idx := blankIndexInQuestion(q, bid)
		if idx >= curIdx {
			continue // 当前空位及其后不进入 history
		}
		if input, ok := latestWrong[bid]; ok {
			history = append(history, initialInsightHistoryItem{BlankIndex: idx, StudentAnswer: input})
		}
	}
	return history
}

// blankPrompt 返回单空辅导提示词（initial-insight.md，文件名用连字符，与 prompts 目录加载约定一致）。
func (s *InitialInsightService) blankPrompt() string {
	if p, ok := s.prompts.Types["initial-insight"]; ok && p != "" {
		return p
	}
	return ""
}

// buildComprehensiveInput 组装 AI 综合评价输入数据包。
//
// 作答时间按空分段（详见 ComputeSegmentDurations）：
//   - 每空完成时间 = 该空"答对/完成那次"的 submitted_at（答对优先，达上限揭示答案等完成那次回退）；
//   - 首空作答时间 = 首空完成时间 − 进入节点时间；后续每空作答时间 = 本空完成时间 − 上一空完成时间；
//   - 每条提交记录的 time_cost 为其所属空起始基准起的累计秒数，完成那次即等于该空作答时间。
func (s *InitialInsightService) buildComprehensiveInput(ctx context.Context, params *InitialInsightParams, subs []model.NodeSubmission, entryTime time.Time, userID int64) *initialComprehensiveInput {
	// 预计算 blank 序号、标准答案，并按空位作答顺序构造分段键（qid:blankId）
	indexByBlank := map[string]int{}
	correctByBlank := map[string]string{}
	segments := make([]string, 0, 8)
	totalBlanks := 0
	for _, q := range params.Questions {
		idx := 0
		for _, c := range q.Content {
			if c.Type == "blank" && c.Blank != nil {
				totalBlanks++
				idx++
				indexByBlank[c.Blank.ID] = idx
				correctByBlank[c.Blank.ID] = c.Blank.ReferenceAnswer
				segments = append(segments, q.ID+":"+c.Blank.ID)
			}
		}
	}

	// 提交记录按时间升序，并按空位分段计算每条耗时
	entries, totalTime := ComputeSegmentDurations(subs, entryTime, segments,
		func(sub model.NodeSubmission) string {
			if sub.QuestionID == nil {
				return ""
			}
			var p map[string]string
			if len(sub.PayloadJSON) > 0 {
				_ = json.Unmarshal(sub.PayloadJSON, &p)
			}
			return *sub.QuestionID + ":" + p["blankId"]
		},
		isKeyCompletion,
	)

	attempt := map[string]int{}
	lastCorrect := map[string]bool{}
	records := make([]initialComprehensiveRecord, 0, len(entries))
	for _, e := range entries {
		sub := e.Sub
		var p map[string]string
		if len(sub.PayloadJSON) > 0 {
			_ = json.Unmarshal(sub.PayloadJSON, &p)
		}
		blankID := p["blankId"]
		qid := ""
		if sub.QuestionID != nil {
			qid = *sub.QuestionID
		}
		key := qid + ":" + blankID
		attempt[key]++

		isCorrect := sub.IsPassed != nil && *sub.IsPassed
		lastCorrect[key] = isCorrect
		records = append(records, initialComprehensiveRecord{
			LessonTitle:   params.Title,
			QuestionID:    qid,
			BlankIndex:    indexByBlank[blankID],
			StudentAnswer: p["input"],
			CorrectAnswer: correctByBlank[blankID],
			IsCorrect:     isCorrect,
			AttemptCount:  attempt[key],
			TimeCost:      e.Duration,
		})
	}

	// 统计指标：读取 key 必须与写入一致（qid:blankId 复合键），否则正确率恒 0
	correctBlanks := 0
	for _, q := range params.Questions {
		for _, c := range q.Content {
			if c.Type == "blank" && c.Blank != nil && lastCorrect[q.ID+":"+c.Blank.ID] {
				correctBlanks++
			}
		}
	}
	accuracyRate := 0.0
	if totalBlanks > 0 {
		accuracyRate = math.Round(float64(correctBlanks)/float64(totalBlanks)*100) / 100
	}

	// 学生信息
	var name, gender, city string
	if u, err := s.repos.User.GetByID(ctx, userID); err == nil && u != nil {
		if u.RealName != nil {
			name = *u.RealName
		}
		if u.Gender != nil {
			gender = *u.Gender
		}
		if u.City != nil {
			city = *u.City
		}
	}

	return &initialComprehensiveInput{
		StudentName:   name,
		Gender:        gender,
		City:          city,
		Records:       records,
		TotalTime:     totalTime,
		TotalBlanks:   totalBlanks,
		CorrectBlanks: correctBlanks,
		TotalAttempts: len(records),
		AccuracyRate:  accuracyRate,
	}
}

// comprehensivePrompt 返回综合评价提示词（initial-comprehensive.md，文件名用连字符，与 prompts 目录加载约定一致）。
func (s *InitialInsightService) comprehensivePrompt() string {
	if p, ok := s.prompts.Types["initial-comprehensive"]; ok && p != "" {
		return p
	}
	return ""
}

// callComprehensiveAI 调用 LLM 生成综合评价并解析 JSON 结果。
func (s *InitialInsightService) callComprehensiveAI(ctx context.Context, input *initialComprehensiveInput) (*initialComprehensiveResult, error) {
	prompt := s.comprehensivePrompt()
	var res initialComprehensiveResult
	if err := callAIWithJSON(ctx, s.llm, prompt, input, &res); err != nil {
		return nil, fmt.Errorf("call comprehensive ai: %w", err)
	}
	return &res, nil
}
