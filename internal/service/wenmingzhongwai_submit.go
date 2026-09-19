package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"zhonghuawenhua_backend/internal/api"
	"zhonghuawenhua_backend/internal/config"
	"zhonghuawenhua_backend/internal/model"
	"zhonghuawenhua_backend/pkg/ai"
	"zhonghuawenhua_backend/pkg/grading"
)

// WenMingZhongWaiService 文明中外（矩阵填空）节点的提交批改业务。
//
// 数据流与其他节点一致：每次 submit 插入 node_submissions 一行，进度缓存在 node_progress；
// 提交走通用异步 SubmitAsync（见 async_submit.go），批改结果（含 errors/ending 等结构化内容）
// 通过 AsyncSubmitResult.ResultJSON 回写 result_json，供前端 DTO 还原（见 ToWenMingZhongWaiResp）。
//
// 批改规则（由每题的错误次数逐级递进，达到上限后自动填入答案）：
//   - 第 1 次错误：direction_guide（方向指引），数字人讲解，提示学生重新回答；
//   - 第 2 次错误：method_guide（方法引导），弹出选项询问是否提供答案；
//   - 第 3 次错误：answer_explain（答案讲解），直接帮学生填写答案并标记该行完成；
//   - “确定意思”类短答案行（如“热闹”）允许常见程度修饰词（很/非常/十分/特别）；
//   - get-answer：一次性揭示全部行的参考答案并标记节点完成。
type WenMingZhongWaiService struct {
	repos   *Repositories
	llm     ai.LLMClient
	prompts config.PromptsConfig
}

// NewWenMingZhongWaiService 创建 WenMingZhongWaiService。
func NewWenMingZhongWaiService(repos *Repositories, llm ai.LLMClient, prompts config.PromptsConfig) *WenMingZhongWaiService {
	return &WenMingZhongWaiService{repos: repos, llm: llm, prompts: prompts}
}

// gradeRow 判定某行学生的作答是否正确。
// 对短答案行（如“热闹”，≤2 字）允许常见程度修饰词（很/非常/十分/特别），
// 如“很热闹”“非常热闹”均视为正确，与赵州桥填空题批改保持一致。
func (p *WenMingZhongWaiParams) gradeRow(rowID, input string) bool {
	ref := p.correctAnswer(rowID)
	if ref == "" {
		return false
	}
	if len([]rune(ref)) <= 2 {
		return grading.GradeZhaoZhouQiaoText(input, ref).IsPassed
	}
	return grading.GradeTextSimple(input, ref).IsPassed
}

// allReferenceAnswers 返回全部 input 行的标准答案（key = rowId）。
func (p *WenMingZhongWaiParams) allReferenceAnswers() map[string]string {
	m := make(map[string]string)
	for _, idx := range p.inputRowIndices() {
		rowID := p.Matrix.Rows[idx].RowID
		if ans := p.correctAnswer(rowID); ans != "" {
			m[rowID] = ans
		}
	}
	return m
}

// endingJSON 组装完成时的总结信息；未配置时返回 nil。
func (p *WenMingZhongWaiParams) endingJSON() *wenMingZhongWaiEndingJSON {
	if p.Ending == nil {
		return nil
	}
	e := &wenMingZhongWaiEndingJSON{Instruction: p.Ending.Instruction}
	e.Video.ID = p.Ending.VideoID
	e.Video.Src = p.Ending.VideoSrc
	return e
}

// Submit 提交批改（异步 AI 辅导）。
//
// 与写感想一致：立即插入 is_processing=true 的提交记录并返回 submitId，
// 后台 goroutine 完成批改后回写，前端通过 GetSubmission 轮询结果。
func (s *WenMingZhongWaiService) Submit(ctx context.Context, classID, nodeID, userID int64, questionID string, req WenMingZhongWaiSubmitRequest) (*api.AsyncSubmitResp, error) {
	// 0. 防御性校验：确认用户是该班级成员
	if err := s.repos.EnsureClassMember(ctx, classID, userID); err != nil {
		return nil, err
	}
	// 0.5 提交类型枚举校验：非法类型不得绕过空输入/上限/幂等校验
	if !isValidSubmitType(req.Type) {
		return nil, ErrInvalidSubmitType
	}
	// 1. 加载矩阵定义
	params, err := loadWenMingZhongWaiParams(ctx, s.repos, nodeID)
	if err != nil {
		return nil, err
	}
	if len(params.inputRowIndices()) == 0 {
		return nil, ErrQuestionNotFound
	}

	if req.Type == "submit" {
		// 强类型校验：至少携带一个非空作答
		if !hasAnyAnswer(req.Answers) {
			return nil, ErrEmptyInput
		}
		// 后端兜底：节点已全部完成时直接拒绝，不调用 AI、不写入提交记录
		if s.isNodeCompleted(ctx, params, userID, nodeID) {
			return nil, ErrSubmitLimitReached
		}
	}

	// 2. 组装 payload（questionId / answers map[rowId]answer）
	payload, _ := json.Marshal(map[string]interface{}{
		"questionId": questionID,
		"answers":    req.Answers,
	})

	// 3. 通用异步提交：立即返回 submitId，后台 goroutine 执行批改并回写
	submitID, err := SubmitAsync(ctx, s.repos, AsyncSubmitTask{
		ClassID:     classID,
		NodeID:      nodeID,
		UserID:      userID,
		NodeType:    model.NodeTypeWenMingZhongWai,
		QuestionID:  questionID,
		SubmitType:  req.Type,
		PayloadJSON: payload,
		Process: func(ctx context.Context) (*AsyncSubmitResult, error) {
			return s.processAnswer(ctx, nodeID, userID, params, req)
		},
		FinalizeProgress: func(ctx context.Context, sub *model.NodeSubmission) error {
			return s.finalizeProgress(ctx, classID, nodeID, userID, params, sub)
		},
		LLM:     s.llm,
		Prompts: s.prompts,
	})
	if err != nil {
		return nil, err
	}

	tip := "提交成功"
	if req.Type == "get-answer" {
		tip = "已查看参考答案"
	}
	return &api.AsyncSubmitResp{SubmitID: submitID, Tip: &tip}, nil
}

// GetSubmission 按 submitID 查询单条提交记录。
// 防御性要求：仅允许提交记录所属用户本人查询，防止越权读取他人作答与批改结果。
func (s *WenMingZhongWaiService) GetSubmission(ctx context.Context, nodeID, userID int64, submitID string) (*api.WenMingZhongWaiSubmitResult, error) {
	sub, err := s.repos.NodeSubmission.GetByCode(ctx, submitID)
	if err != nil {
		return nil, ErrQuestionNotFound
	}
	if sub.NodeID != nodeID || sub.NodeType != model.NodeTypeWenMingZhongWai || sub.UserID != userID {
		return nil, ErrQuestionNotFound
	}
	resp := sub.ToWenMingZhongWaiResp()
	return &resp, nil
}

// processAnswer 执行文明中外特有的批改业务（在 SubmitAsync 后台 goroutine 中调用）。
func (s *WenMingZhongWaiService) processAnswer(ctx context.Context, nodeID, userID int64, params *WenMingZhongWaiParams, req WenMingZhongWaiSubmitRequest) (*AsyncSubmitResult, error) {
	if req.Type == "get-answer" {
		// get-answer：一次性揭示全部行的参考答案并标记完成
		states := s.rowStates(ctx, params, userID, nodeID)
		rowStatesOut := make(map[string]string, len(states))
		for rowID := range states {
			rowStatesOut[rowID] = "completed"
		}
		return &AsyncSubmitResult{
			IsCompleted: true,
			Revealed:    true,
			ResultJSON: &wenMingZhongWaiResultJSON{
				Errors:          []wenMingZhongWaiErrorItem{},
				ReferenceAnswer: params.allReferenceAnswers(),
				Ending:          params.endingJSON(),
				RowStates:       rowStatesOut,
			},
		}, nil
	}
	return s.gradeSubmission(ctx, nodeID, userID, params, req)
}

// gradeSubmission 矩阵逐行批改：答对标记完成；答错按该行累计错误次数切换辅导策略，
// 达上限（第 3 次错误）自动填入答案并标记该行完成；全部行完成后节点完成。
func (s *WenMingZhongWaiService) gradeSubmission(ctx context.Context, nodeID, userID int64, params *WenMingZhongWaiParams, req WenMingZhongWaiSubmitRequest) (*AsyncSubmitResult, error) {
	// 历史行状态（不含本次尚未回写的记录，IsPassed=nil 的进行中记录被跳过）
	states := s.rowStates(ctx, params, userID, nodeID)

	var errors []wenMingZhongWaiErrorItem
	var feedbacks []string
	referenceAnswer := map[string]string{}
	rowStatesOut := make(map[string]string, len(states))
	revealedAny := false

	// 校验提交的填空位：未知 key 不能被静默跳过，否则会误判为"全部正确"。
	for rowID, answer := range req.Answers {
		if strings.TrimSpace(answer) == "" {
			continue
		}
		if !params.isInputRow(rowID) {
			fb := "请按表格左侧提示在对应空格中作答。"
			if label := params.rowLabel(rowID); label != "" {
				fb = fmt.Sprintf("%s：请在对应空格中作答。", label)
			}
			fbPtr := fb
			errors = append(errors, wenMingZhongWaiErrorItem{RowID: rowID, BlankID: rowID, Msg: &fbPtr})
			feedbacks = append(feedbacks, fb)
			rowStatesOut[rowID] = "failed"
		}
	}

	for _, idx := range params.inputRowIndices() {
		rowID := params.Matrix.Rows[idx].RowID
		answer, ok := req.Answers[rowID]
		if !ok || strings.TrimSpace(answer) == "" {
			continue // 本次未提交该行
		}
		st := states[rowID]
		if st == nil {
			st = &wenMingZhongWaiRowState{}
			states[rowID] = st
		}
		if st.Completed {
			// 该行已解决（此前答对或已自动填入），仅返回其参考答案，不重复批改
			rowStatesOut[rowID] = "completed"
			referenceAnswer[rowID] = params.correctAnswer(rowID)
			continue
		}

		if params.gradeRow(rowID, answer) {
			st.Passed = true
			st.Completed = true
			rowStatesOut[rowID] = "passed"
			continue
		}

		// 答错：按累计错误次数切换辅导策略生成 AI 反馈
		st.Errors++
		rowStatesOut[rowID] = "failed"
		strategy := wenMingZhongWaiStrategyForFail(st.Errors, params.MaxErrors)
		fb := s.aiFeedback(ctx, params, nodeID, userID, rowID, answer, strategy, req.Answers)
		if fb == "" {
			fb = fmt.Sprintf("%s 不正确，请再想想。", params.rowLabel(rowID))
		}
		fbPtr := fb
		errors = append(errors, wenMingZhongWaiErrorItem{RowID: rowID, BlankID: rowID, Msg: &fbPtr})
		feedbacks = append(feedbacks, fb)

		if st.Errors >= params.MaxErrors {
			// 达上限（第 3 次错误）：自动填入答案并标记该行完成
			st.Completed = true
			revealedAny = true
			rowStatesOut[rowID] = "completed"
			referenceAnswer[rowID] = params.correctAnswer(rowID)
		}
	}

	res := &AsyncSubmitResult{
		IsPassed: len(errors) == 0,
		Feedback: strings.Join(feedbacks, "\n"),
	}
	if res.IsPassed {
		// 本次提交全部正确：返回全部参考答案
		referenceAnswer = params.allReferenceAnswers()
		for rowID := range states {
			rowStatesOut[rowID] = "passed"
		}
		if res.Feedback == "" {
			res.Feedback = "回答错误！"
		}
	}

	// 节点完成判定：所有 input 行均已解决（答对或自动填入）。
	// 直接用已含本次提交作答的本地 states 判定，避免 isNodeCompleted 从 DB 重读
	// （当前提交尚未落库，会把"本次刚好达到错误上限自动填答案"的完成判定漏掉）。
	allDone := true
	for _, idx := range params.inputRowIndices() {
		rowID := params.Matrix.Rows[idx].RowID
		if st := states[rowID]; st == nil || !st.Completed {
			allDone = false
			break
		}
	}
	if allDone {
		res.IsCompleted = true
		revealedAny = true
	}
	if revealedAny {
		res.Revealed = true
	}
	if len(referenceAnswer) == 0 {
		referenceAnswer = nil
	}
	res.ResultJSON = &wenMingZhongWaiResultJSON{
		Errors:          errors,
		ReferenceAnswer: referenceAnswer,
		Ending:          params.endingJSON(),
		RowStates:       rowStatesOut,
	}
	return res, nil
}

// finalizeProgress 在批改结果回写后更新文明中外进度的聚合字段（error_count / revealed / completed）。
func (s *WenMingZhongWaiService) finalizeProgress(ctx context.Context, classID, nodeID, userID int64, params *WenMingZhongWaiParams, sub *model.NodeSubmission) error {
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
	prog.Revealed = prog.Revealed || sub.Revealed
	if s.isNodeCompleted(ctx, params, userID, nodeID) {
		prog.Completed = true
	}
	if prog.ID == 0 {
		_, err = s.repos.NodeProgress.Create(ctx, prog)
	} else {
		err = s.repos.NodeProgress.Update(ctx, prog)
	}
	return err
}

// countNodeErrors 统计该节点该用户全部错误提交次数（供进度聚合 error_count）。
func (s *WenMingZhongWaiService) countNodeErrors(ctx context.Context, userID, nodeID int64) int {
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

// wenMingZhongWaiRowState 文明中外某一行的完成/错误状态。
type wenMingZhongWaiRowState struct {
	Passed    bool // 是否曾有答对记录
	Completed bool // 是否已解决（答对 / 自动填入 / get-answer 揭示）
	Errors    int  // 累计错误次数
}

// rowStates 汇总该节点该用户全部已回写提交，计算每行状态（不含处理中的本次记录）。
// get-answer 揭示后全部行视为完成；整次提交通过（IsPassed=true）则其作答行全部答对。
func (s *WenMingZhongWaiService) rowStates(ctx context.Context, params *WenMingZhongWaiParams, userID, nodeID int64) map[string]*wenMingZhongWaiRowState {
	subs, err := s.repos.NodeSubmission.ListByUserNode(ctx, userID, nodeID, 200, 0)
	if err != nil {
		// 防御性要求：出错返回非 nil 空 map，避免调用方对 nil map 写入触发 panic
		return map[string]*wenMingZhongWaiRowState{}
	}
	states := make(map[string]*wenMingZhongWaiRowState, len(params.inputRowIndices()))
	for _, idx := range params.inputRowIndices() {
		rowID := params.Matrix.Rows[idx].RowID
		states[rowID] = &wenMingZhongWaiRowState{}
	}
	// ListByUserNode 按时间倒序，倒序遍历得到升序
	for i := len(subs) - 1; i >= 0; i-- {
		sub := subs[i]
		if sub.SubmitType == "get-answer" {
			for _, st := range states {
				st.Completed = true
			}
			continue
		}
		if sub.IsPassed == nil {
			continue // 尚未回写批改结果（处理中 / 失败）不纳入统计
		}
		var p struct {
			Answers map[string]string `json:"answers"`
		}
		if len(sub.PayloadJSON) > 0 {
			_ = json.Unmarshal(sub.PayloadJSON, &p)
		}
		for _, idx := range params.inputRowIndices() {
			rowID := params.Matrix.Rows[idx].RowID
			st := states[rowID]
			answer, ok := p.Answers[rowID]
			if !ok {
				continue
			}
			if *sub.IsPassed {
				// 整次提交通过 = 其作答行全部答对
				st.Passed = true
				st.Completed = true
				continue
			}
			if params.gradeRow(rowID, answer) {
				st.Passed = true
				st.Completed = true
			} else {
				st.Errors++
			}
		}
	}
	// 已达错误上限的行视为完成（答案已自动填入）
	for _, st := range states {
		if !st.Completed && st.Errors >= params.MaxErrors {
			st.Completed = true
		}
	}
	return states
}

// isNodeCompleted 判断所有 input 行是否均已解决（答对 / 自动填入 / 揭示答案）。
func (s *WenMingZhongWaiService) isNodeCompleted(ctx context.Context, params *WenMingZhongWaiParams, userID, nodeID int64) bool {
	states := s.rowStates(ctx, params, userID, nodeID)
	for _, idx := range params.inputRowIndices() {
		rowID := params.Matrix.Rows[idx].RowID
		if st := states[rowID]; st == nil || !st.Completed {
			return false
		}
	}
	return true
}

// hasAnyAnswer 判断 answers 中是否存在非空作答。
func hasAnyAnswer(answers map[string]string) bool {
	for _, v := range answers {
		if strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}

// wenMingZhongWaiStrategyForFail 根据该行累计错误次数决定 AI 辅导策略：
// 第 1 次 = 方向指引；第 2 次 = 方法引导；第 3 次（达上限） = 答案讲解。
func wenMingZhongWaiStrategyForFail(curErrors, maxErrors int) string {
	switch {
	case curErrors >= maxErrors:
		return "answer_explain"
	case curErrors >= maxErrors-1:
		return "method_guide"
	default:
		return "direction_guide"
	}
}

// aiFeedback 调用 AI 生成当前出错行的个性化辅导（wenmingzhongwai.md 提示词）。
// AI 未配置或调用失败时返回空串，批改判定仍以本地为准，不阻塞提交回写。
func (s *WenMingZhongWaiService) aiFeedback(ctx context.Context, params *WenMingZhongWaiParams, nodeID, userID int64, rowID, input, strategy string, answers map[string]string) string {
	if s.llm == nil {
		return ""
	}
	prompt := s.feedbackPrompt()
	if prompt == "" {
		return ""
	}
	aiInput := s.buildAIInput(ctx, params, nodeID, userID, rowID, input, strategy, answers)
	var res wenMingZhongWaiAIResult
	if err := callAIWithJSON(ctx, s.llm, prompt, aiInput, &res); err != nil {
		log.Printf("wenmingzhongwai ai feedback failed: %v", err)
		return ""
	}
	return res.FeedbackText
}

// buildAIInput 组装传给 AI 的 JSON 数据包（字段与 prompts/wenmingzhongwai.md 对齐）。
func (s *WenMingZhongWaiService) buildAIInput(ctx context.Context, params *WenMingZhongWaiParams, nodeID, userID int64, rowID, input, strategy string, answers map[string]string) wenMingZhongWaiAIInput {
	aiInput := wenMingZhongWaiAIInput{
		ErrorSlot:       rowID,
		StudentInput:    answers,
		ReferenceAnswer: params.allReferenceAnswers(),
		Context:         wenMingZhongWaiParagraph,
		Strategy:        strategy,
	}
	for _, idx := range params.inputRowIndices() {
		r := params.Matrix.Rows[idx]
		aiInput.TaskStructure.Slots = append(aiInput.TaskStructure.Slots, wenMingZhongWaiTaskSlot{
			SlotID:      r.RowID,
			Label:       params.rowLabel(r.RowID),
			Description: params.rowLabel(r.RowID),
		})
	}
	if u, err := s.repos.User.GetByID(ctx, userID); err == nil && u != nil {
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
	aiInput.History = s.buildAIHistory(ctx, nodeID, userID)
	return aiInput
}

// buildAIHistory 组装历史对话（之前答错的提交：学生答案 + AI 反馈），保持时间升序。
func (s *WenMingZhongWaiService) buildAIHistory(ctx context.Context, nodeID, userID int64) []wenMingZhongWaiAIHistoryItem {
	subs, err := s.repos.NodeSubmission.ListByUserNode(ctx, userID, nodeID, 200, 0)
	if err != nil {
		return nil
	}
	var history []wenMingZhongWaiAIHistoryItem
	for i := len(subs) - 1; i >= 0; i-- {
		sub := subs[i]
		if sub.SubmitType != "submit" || sub.IsPassed == nil || *sub.IsPassed {
			continue // 仅取已回写的答错记录
		}
		var p struct {
			Answers map[string]string `json:"answers"`
		}
		if len(sub.PayloadJSON) > 0 {
			_ = json.Unmarshal(sub.PayloadJSON, &p)
		}
		fb := ""
		if sub.Feedback != nil {
			fb = *sub.Feedback
		}
		history = append(history, wenMingZhongWaiAIHistoryItem{StudentInput: p.Answers, AIFeedback: fb})
	}
	return history
}

// feedbackPrompt 返回文明中外辅导提示词（wenmingzhongwai.md）。
func (s *WenMingZhongWaiService) feedbackPrompt() string {
	if p, ok := s.prompts.Types["wenmingzhongwai"]; ok && p != "" {
		return p
	}
	return ""
}
