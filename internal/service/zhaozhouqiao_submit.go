package service

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	"zhonghuawenhua_backend/internal/api"
	"zhonghuawenhua_backend/internal/config"
	"zhonghuawenhua_backend/internal/model"
	"zhonghuawenhua_backend/pkg/ai"
	"zhonghuawenhua_backend/pkg/grading"
)

// ZhaoZhouQiaoService 赵州桥节点的提交批改业务。
//
// 数据流与写感想一致：每次 submit 插入 node_submissions 一行，进度缓存在 node_progress，
// 草稿存于 node_progress.draft_json；提交走通用异步 SubmitAsync（见 async_submit.go）。
//
// 批改规则（由问题卡区分）：
//   - 填空题（voiceOnly=false）：本地宽松判定通过/失败（允许常见程度修饰词，如“很美观”），
//     失败时调用 AI 按失败次数切换辅导策略（方向指引/方法引导/答案讲解）生成反馈；
//     达上限后揭示卡片配置的标准答案并标记完成。
//   - 朗读题（voiceOnly=true）：语音转写可能存在识别错误，不做精准匹配，转写文本达到最小
//     字数即视为完成朗读；失败时调用 AI 按失败次数切换策略（方向指引/最终讲解），
//     只提供两次朗读录音机会。
//   - get-answer：揭示卡片配置的标准答案并标记完成（答案由系统展示，不经过 AI 生成）。
type ZhaoZhouQiaoService struct {
	repos   *Repositories
	llm     ai.LLMClient
	prompts config.PromptsConfig
}

// NewZhaoZhouQiaoService 创建 ZhaoZhouQiaoService。
func NewZhaoZhouQiaoService(repos *Repositories, llm ai.LLMClient, prompts config.PromptsConfig) *ZhaoZhouQiaoService {
	return &ZhaoZhouQiaoService{repos: repos, llm: llm, prompts: prompts}
}

// ZhaoZhouQiaoSubmitRequest 赵州桥提交请求（来自 SubmitZhaoZhouQiaoJSONBody）。
type ZhaoZhouQiaoSubmitRequest struct {
	CardID  string // 问题卡 ID，为空时回退用路径上的 questionID
	Type    string // "submit" | "get-answer"
	Text    string // 填空题作答文本 / 朗读题语音转写结果
	AudioID string // 朗读录音文件 ID（可选）
}

// 朗读题最小有效字数：语音转写可能存在识别错误，不做精准匹配，
// 转写文本去除标点后达到该字数即视为完成了朗读。
const minReadingRunes = 5

// maxReadingErrors 朗读题最大提交次数（两次朗读录音机会）。
const maxReadingErrors = 2

// zhaoZhouQiaoAIInput 传给 AI 的输入数据（字段与 prompts/zhaozhouqiao.md 对齐）。
type zhaoZhouQiaoAIInput struct {
	StudentName     string `json:"student_name"`
	Gender          string `json:"gender"`
	City            string `json:"city"`
	TaskType        string `json:"task_type"` // fill_in | reading
	Strategy        string `json:"strategy"`  // direction_guide | method_guide | answer_explain | final_explain
	StudentInput    string `json:"student_input"`
	ReferenceAnswer string `json:"reference_answer"`
	Context         string `json:"context"`
}

// zhaoZhouQiaoAIResult AI 返回的反馈结果。
type zhaoZhouQiaoAIResult struct {
	IsPass       bool   `json:"is_pass"`
	FeedbackText string `json:"feedback_text"`
}

// readingPasses 朗读题宽松判定：语音转写可能含识别错误，不做精准匹配，
// 转写文本去除标点后达到最小字数即视为完成了朗读。
func readingPasses(text string) bool {
	return len([]rune(grading.NormalizeText(text))) >= minReadingRunes
}

// Submit 提交批改（异步 AI 反馈）。
//
// 与写感想一致：立即插入 is_processing=true 的提交记录并返回 submitId，
// 后台 goroutine 完成批改后回写，前端通过 GetSubmission 轮询结果。
func (s *ZhaoZhouQiaoService) Submit(ctx context.Context, classID, nodeID, userID int64, questionID string, req ZhaoZhouQiaoSubmitRequest) (*api.AsyncSubmitResp, error) {
	// 0. 防御性校验：确认用户是该班级成员
	if err := s.repos.EnsureClassMember(ctx, classID, userID); err != nil {
		return nil, err
	}
	// 0.5 提交类型枚举校验：非法类型不得绕过空输入/上限/幂等校验
	if !isValidSubmitType(req.Type) {
		return nil, ErrInvalidSubmitType
	}
	// 1. 加载卡片定义
	params, err := loadZhaoZhouQiaoParams(ctx, s.repos, nodeID)
	if err != nil {
		return nil, err
	}
	cardID := req.CardID
	if cardID == "" {
		cardID = questionID
	}
	card := params.FindCard(cardID)
	if card == nil {
		return nil, ErrQuestionNotFound
	}

	maxErrors := getMaxErrors(card.MaxErrors)
	if card.VoiceOnly && maxErrors > maxReadingErrors {
		// 朗读题只提供两次朗读录音机会
		maxErrors = maxReadingErrors
	}

	if req.Type == "submit" {
		// 强类型校验：填空题必须携带非空作答文本；朗读题为语音转写，允许为空
		if !card.VoiceOnly && strings.TrimSpace(req.Text) == "" {
			return nil, ErrEmptyInput
		}
		// 后端兜底：该卡历史失败次数已达上限时直接拒绝，不调用 AI、不写入提交记录
		if s.countCardErrors(ctx, userID, nodeID, cardID) >= maxErrors {
			return nil, ErrSubmitLimitReached
		}
	}

	// 2. 组装 payload（cardId / text / audio.id）
	audio := map[string]string{}
	if req.AudioID != "" {
		audio["id"] = req.AudioID
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"cardId": cardID,
		"text":   req.Text,
		"audio":  audio,
	})

	// 3. 通用异步提交：立即返回 submitId，后台 goroutine 执行批改并回写
	submitID, err := SubmitAsync(ctx, s.repos, AsyncSubmitTask{
		ClassID:     classID,
		NodeID:      nodeID,
		UserID:      userID,
		NodeType:    model.NodeTypeZhaoZhouQiao,
		QuestionID:  cardID,
		SubmitType:  req.Type,
		PayloadJSON: payload,
		Process: func(ctx context.Context) (*AsyncSubmitResult, error) {
			return s.processZhaoZhouQiaoAnswer(ctx, nodeID, userID, cardID, card, req)
		},
		FinalizeProgress: func(ctx context.Context, sub *model.NodeSubmission) error {
			return s.finalizeZhaoZhouQiaoProgress(ctx, classID, nodeID, userID, params, sub)
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

// processZhaoZhouQiaoAnswer 执行赵州桥特有的批改业务，返回回写结果。
// 在 SubmitAsync 启动的后台 goroutine 中调用。
func (s *ZhaoZhouQiaoService) processZhaoZhouQiaoAnswer(ctx context.Context, nodeID, userID int64, cardID string, card *ZhaoZhouQiaoCardDef, req ZhaoZhouQiaoSubmitRequest) (*AsyncSubmitResult, error) {
	maxErrors := getMaxErrors(card.MaxErrors)
	if card.VoiceOnly && maxErrors > maxReadingErrors {
		maxErrors = maxReadingErrors
	}

	if req.Type == "get-answer" {
		// get-answer：揭示卡片配置的标准答案（朗读题无标准答案，仅标记完成）
		return &AsyncSubmitResult{
			IsCompleted:     true,
			Revealed:        true,
			ReferenceAnswer: card.ReferenceAnswer,
		}, nil
	}

	prevFail := s.countCardErrors(ctx, userID, nodeID, cardID)
	if card.VoiceOnly {
		return s.processReadingAnswer(ctx, card, userID, req.Text, prevFail, maxErrors)
	}
	return s.processFillInAnswer(ctx, card, userID, req.Text, prevFail, maxErrors)
}

// processFillInAnswer 填空题批改：本地宽松判定通过/失败，失败时 AI 按策略生成反馈，
// 达上限后揭示标准答案并标记完成。
func (s *ZhaoZhouQiaoService) processFillInAnswer(ctx context.Context, card *ZhaoZhouQiaoCardDef, userID int64, input string, prevFail, maxErrors int) (*AsyncSubmitResult, error) {
	grade := grading.GradeZhaoZhouQiaoText(input, card.ReferenceAnswer)

	res := &AsyncSubmitResult{IsPassed: grade.IsPassed}
	if grade.IsPassed {
		res.IsCompleted = true
		res.Feedback = "回答正确，你真棒！"
		// 通过后仅显示标准答案，不展示学生的变体（如“很美观”）
		res.ReferenceAnswer = card.ReferenceAnswer
		return res, nil
	}

	// 失败：AI 生成个性化反馈（失败不会导致整次提交失败，故 AI 出错时优雅降级为空反馈）
	strategy := zhaoZhouQiaoStrategyForFail("fill_in", prevFail, maxErrors)
	res.Feedback = s.aiFeedback(ctx, card, userID, input, strategy, "fill_in")

	curFail := prevFail + 1
	if curFail >= maxErrors {
		res.IsCompleted = true
		res.Revealed = true
		res.ReferenceAnswer = card.ReferenceAnswer
	}
	return res, nil
}

// processReadingAnswer 朗读题批改：不做精准匹配，转写文本达到最小字数即判完成；
// 否则 AI 按失败次数切换策略（方向指引/最终讲解）生成反馈，达到上限后任务结束。
func (s *ZhaoZhouQiaoService) processReadingAnswer(ctx context.Context, card *ZhaoZhouQiaoCardDef, userID int64, input string, prevFail, maxErrors int) (*AsyncSubmitResult, error) {
	if readingPasses(input) {
		return &AsyncSubmitResult{
			IsPassed:    true,
			IsCompleted: true,
			Feedback:    "朗读流利，感情到位！",
		}, nil
	}

	strategy := zhaoZhouQiaoStrategyForFail("reading", prevFail, maxErrors)
	res := &AsyncSubmitResult{
		IsPassed: false,
		Feedback: s.aiFeedback(ctx, card, userID, input, strategy, "reading"),
	}
	if prevFail+1 >= maxErrors {
		// 两次朗读录音机会用尽，温和告知任务结束
		res.IsCompleted = true
	}
	return res, nil
}

// finalizeZhaoZhouQiaoProgress 在批改结果回写后更新赵州桥进度的聚合字段（error_count / revealed / completed）。
func (s *ZhaoZhouQiaoService) finalizeZhaoZhouQiaoProgress(ctx context.Context, classID, nodeID, userID int64, params *ZhaoZhouQiaoParams, sub *model.NodeSubmission) error {
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
	prog.ErrorCount = s.countAllCardErrors(ctx, userID, nodeID)
	prog.Revealed = prog.Revealed || sub.Revealed
	if s.checkAllCardsCompleted(ctx, params, userID, nodeID) {
		prog.Completed = true
	}
	if prog.ID == 0 {
		_, err = s.repos.NodeProgress.Create(ctx, prog)
	} else {
		err = s.repos.NodeProgress.Update(ctx, prog)
	}
	return err
}

// zhaoZhouQiaoStrategyForFail 根据任务类型与失败次数决定 AI 辅导策略。
//
//   - 填空题：方向指引（第1次）/ 方法引导（第2次）/ 答案讲解（第3次及以上，系统已展示答案）；
//   - 朗读题：方向指引（第1次不完整）/ 最终讲解（第2次仍不完整，任务结束）。
func zhaoZhouQiaoStrategyForFail(taskType string, prevFail, maxErrors int) string {
	if taskType == "reading" {
		if prevFail >= 1 {
			return "final_explain"
		}
		return "direction_guide"
	}
	switch {
	case prevFail >= maxErrors-1:
		return "answer_explain"
	case prevFail >= maxErrors-2:
		return "method_guide"
	default:
		return "direction_guide"
	}
}

// aiFeedback 调用 AI 生成个性化反馈（zhaozhouqiao.md 提示词）。
// AI 未配置或调用失败时返回空串（批改判定仍以本地为准，不阻塞提交回写）。
func (s *ZhaoZhouQiaoService) aiFeedback(ctx context.Context, card *ZhaoZhouQiaoCardDef, userID int64, input, strategy, taskType string) string {
	if s.llm == nil {
		return ""
	}
	prompt := s.zhaoZhouQiaoPrompt()
	if prompt == "" {
		return ""
	}
	aiInput := s.buildAIInput(card, userID, input, strategy, taskType)
	var res zhaoZhouQiaoAIResult
	if err := callAIWithJSON(ctx, s.llm, prompt, aiInput, &res); err != nil {
		log.Printf("zhaozhouqiao ai feedback failed: %v", err)
		return ""
	}
	return res.FeedbackText
}

// buildAIInput 组装传给 AI 的 JSON 数据（字段与 prompts/zhaozhouqiao.md 对齐）。
func (s *ZhaoZhouQiaoService) buildAIInput(card *ZhaoZhouQiaoCardDef, userID int64, input, strategy, taskType string) zhaoZhouQiaoAIInput {
	aiInput := zhaoZhouQiaoAIInput{
		TaskType:        taskType,
		Strategy:        strategy,
		StudentInput:    input,
		ReferenceAnswer: card.ReferenceAnswer,
		Context:         card.Title,
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

// zhaoZhouQiaoPrompt 返回赵州桥反馈提示词（zhaozhouqiao.md）。
func (s *ZhaoZhouQiaoService) zhaoZhouQiaoPrompt() string {
	if p, ok := s.prompts.Types["zhaozhouqiao"]; ok && p != "" {
		return p
	}
	return ""
}

// GetSubmission 按 submitID 查询单条提交记录。
// 防御性要求：仅允许提交记录所属用户本人查询，防止越权读取他人作答与批改结果。
func (s *ZhaoZhouQiaoService) GetSubmission(ctx context.Context, nodeID, userID int64, submitID string) (*api.ZhaoZhouQiaoSumbitResp, error) {
	sub, err := s.repos.NodeSubmission.GetByCode(ctx, submitID)
	if err != nil {
		return nil, ErrQuestionNotFound
	}
	if sub.NodeID != nodeID || sub.NodeType != model.NodeTypeZhaoZhouQiao || sub.UserID != userID {
		return nil, ErrQuestionNotFound
	}
	resp := sub.ToZhaoZhouQiaoResp()
	return &resp, nil
}

// countCardErrors 统计某张卡已累计的错误次数。
func (s *ZhaoZhouQiaoService) countCardErrors(ctx context.Context, userID, nodeID int64, cardID string) int {
	subs, err := s.repos.NodeSubmission.ListByUserNode(ctx, userID, nodeID, 100, 0)
	if err != nil {
		return 0
	}
	count := 0
	for _, sub := range subs {
		if sub.SubmitType != "submit" {
			continue
		}
		if sub.QuestionID == nil || *sub.QuestionID != cardID {
			continue
		}
		if sub.IsPassed != nil && !*sub.IsPassed {
			count++
		}
	}
	return count
}

// countAllCardErrors 统计该节点该用户全部卡片的错误次数。
func (s *ZhaoZhouQiaoService) countAllCardErrors(ctx context.Context, userID, nodeID int64) int {
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

// checkAllCardsCompleted 检查所有卡片是否都已完成（答对 / 达到上限 / get-answer）。
func (s *ZhaoZhouQiaoService) checkAllCardsCompleted(ctx context.Context, params *ZhaoZhouQiaoParams, userID, nodeID int64) bool {
	subs, err := s.repos.NodeSubmission.ListByUserNode(ctx, userID, nodeID, 200, 0)
	if err != nil {
		return false
	}
	completedByCard := make(map[string]bool, len(params.Cards))
	for _, sub := range subs {
		if sub.QuestionID == nil {
			continue
		}
		if sub.IsCompleted != nil && *sub.IsCompleted {
			completedByCard[*sub.QuestionID] = true
		}
	}
	for _, c := range params.Cards {
		if !completedByCard[c.ID] {
			return false
		}
	}
	return true
}
