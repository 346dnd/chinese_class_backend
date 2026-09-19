package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"zhonghuawenhua_backend/internal/api"
	"zhonghuawenhua_backend/internal/config"
	"zhonghuawenhua_backend/internal/model"
	"zhonghuawenhua_backend/pkg/ai"
)

// --- heritage-cultural（讲解优秀文化）节点 params_json 结构 ---

// HeritageCulturalParamsDef 讲解优秀文化节点的 params_json 结构。
// 直接复用 api.HeritageCulturalParams（含 String/IntroVideo/IntroBubbles）。
type HeritageCulturalParamsDef struct {
	api.HeritageCulturalParams
	// MaxSubmissions 最大提交次数（达到上限后无论是否合格均完成节点，默认 3）。
	MaxSubmissions int `json:"maxSubmissions,omitempty"`
}

// heritageMaxSubmissions 讲解优秀文化节点最大提交次数的默认值（未在 params_json 配置时使用）。
const heritageMaxSubmissions = 3

// heritageIntroBubbleDTO 与 api.HeritageCulturalParams.IntroBubbles 元素类型（内联匿名结构体）保持一致。
type heritageIntroBubbleDTO = struct {
	Role api.HeritageCulturalParamsIntroBubblesRole `json:"role"`
	Text string                                     `json:"text"`
}

// loadHeritageCulturalParams 加载讲解优秀文化节点参数。
// 内容一律来自数据库 node_contents.params_json，不提供任何硬编码兜底（删除占位默认内容）。
func loadHeritageCulturalParams(ctx context.Context, repos *Repositories, nodeID int64) (*HeritageCulturalParamsDef, error) {
	var p HeritageCulturalParamsDef
	if err := loadNodeParams(ctx, repos, nodeID, &p); err != nil {
		return nil, err
	}
	if p.String == "" {
		return nil, ErrNodeContentNotConfigured
	}
	if p.MaxSubmissions <= 0 {
		p.MaxSubmissions = heritageMaxSubmissions
	}
	return &p, nil
}

// --- HeritageCulturalService ---

// HeritageCulturalService 「讲解优秀文化」节点的参数获取、提交、评测与总评业务。
//
// 数据流：
//   - 节点导入内容（标题/视频/对话气泡/最大提交次数）存于 node_contents.params_json
//   - 学生每次提交讲解文本，插入 node_submissions 一行（payload_json 存 text）
//   - 提交流程为双 AI 异步：先调 heritage-judge 判定是否合格，再调 heritage-coaching 生成辅导；
//     合格或达到最大提交次数（默认第 3 次）时节点完成，随后异步生成综合评价（heritage-comprehensive）
//   - GetState 返回该用户该节点的全部提交记录
//   - GetEnding 节点完成返回综合评价；未完成返回最后一次提交的辅导反馈
type HeritageCulturalService struct {
	repos   *Repositories
	llm     ai.LLMClient
	prompts config.PromptsConfig
}

// NewHeritageCulturalService 创建 HeritageCulturalService。
func NewHeritageCulturalService(repos *Repositories, llm ai.LLMClient, prompts config.PromptsConfig) *HeritageCulturalService {
	return &HeritageCulturalService{repos: repos, llm: llm, prompts: prompts}
}

// HeritageCulturalSubmitResp 提交回答接口的返回（对应 openapi AsyncSubmitResp）。
// api 包未生成该类型，这里本地定义。
type HeritageCulturalSubmitResp struct {
	SubmitID string `json:"submitId"`
	Tip      string `json:"tip"`
}

// GetParams 获取讲解优秀文化节点静态参数（标题、导入视频、对话气泡）。
// 纯内容端点：仅返回共享节点内容，无用户私有数据、无副作用，可不强制班级校验（防御性基线 ③）。
func (s *HeritageCulturalService) GetParams(ctx context.Context, nodeID int64) (*api.HeritageCulturalParams, error) {
	params, err := loadHeritageCulturalParams(ctx, s.repos, nodeID)
	if err != nil {
		return nil, err
	}
	return &params.HeritageCulturalParams, nil
}

// GetState 获取讲解优秀文化节点状态（该用户该节点的提交历史）。
// 防御性要求：确认用户是该班级成员，防止非本班用户访问。
func (s *HeritageCulturalService) GetState(ctx context.Context, classID, nodeID, userID int64) (*api.HeritageCulturalState, error) {
	if err := s.repos.EnsureClassMember(ctx, classID, userID); err != nil {
		return nil, err
	}
	if _, err := loadHeritageCulturalParams(ctx, s.repos, nodeID); err != nil {
		return nil, err
	}
	submissions, err := s.repos.NodeSubmission.ListByUserNode(ctx, userID, nodeID, 200, 0)
	if err != nil {
		return nil, fmt.Errorf("list submissions: %w", err)
	}
	state := &api.HeritageCulturalState{SubmitLogs: []api.HeritageCulturalSubmitResult{}}
	for _, sub := range submissions {
		state.SubmitLogs = append(state.SubmitLogs, sub.ToHeritageCulturalResp())
	}
	return state, nil
}

// Submit 提交一段讲解文本（异步双 AI 处理）。
//
// 立即插入 is_processing=true 的提交记录并返回 submitId，供前端轮询评测结果；
// 后台 goroutine 依次调用 heritage-judge（合格判定）→ heritage-coaching（辅导）：
//   - 判定不合格且未达最大提交次数：仅辅导（response_mode=仅辅导）；
//   - 判定合格或第 N 次（N = maxSubmissions）提交：辅导+范文（response_mode=辅导+范文），节点完成；
//
// 节点完成时随后异步生成综合评价（heritage-comprehensive）并缓存到 ai_evaluation_json。
func (s *HeritageCulturalService) Submit(ctx context.Context, classID, nodeID, userID int64, text string) (*HeritageCulturalSubmitResp, error) {
	// 0. 防御性校验：确认用户是该班级成员
	if err := s.repos.EnsureClassMember(ctx, classID, userID); err != nil {
		return nil, err
	}
	// 1. 加载节点配置（数据库）
	params, err := loadHeritageCulturalParams(ctx, s.repos, nodeID)
	if err != nil {
		return nil, err
	}
	// 2. 强类型校验：submit 类型必须携带非空作答内容，避免写入空 payload
	if strings.TrimSpace(text) == "" {
		return nil, ErrEmptyInput
	}
	// 3. 后端兜底：节点已完成（合格或已达最大提交次数）时直接拒绝，不调用 AI、不写入提交记录
	if s.isNodeCompleted(ctx, params, nodeID, userID) {
		return nil, ErrSubmitLimitReached
	}

	payload, _ := json.Marshal(map[string]string{"text": text})

	// 4. 通用异步提交：立即返回 submitId，后台 goroutine 执行双 AI 业务并回写
	submitID, err := SubmitAsync(ctx, s.repos, AsyncSubmitTask{
		ClassID:     classID,
		NodeID:      nodeID,
		UserID:      userID,
		NodeType:    model.NodeTypeHeritageCultural,
		SubmitType:  "submit",
		PayloadJSON: payload,
		Process: func(ctx context.Context) (*AsyncSubmitResult, error) {
			return s.processSubmit(ctx, nodeID, userID, params, text)
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

	return &HeritageCulturalSubmitResp{
		SubmitID: submitID,
		Tip:      "已收到你的讲解，正在请老师点评…",
	}, nil
}

// processSubmit 在 SubmitAsync 后台 goroutine 中执行讲解优秀文化的双 AI 业务。
// 流程：判定（heritage-judge）→ 辅导（heritage-coaching）→ 组装回写结果。
func (s *HeritageCulturalService) processSubmit(ctx context.Context, nodeID, userID int64, params *HeritageCulturalParamsDef, text string) (*AsyncSubmitResult, error) {
	// 当前提交为第几次提交。
	// 注意：SubmitAsync 会先落库当前提交（is_processing=true）再启动本 goroutine，
	// 因此 countSubmissions 已包含当前这条记录，返回的即是本次提交的轮次序号，无需再 +1。
	submissionIndex := s.countSubmissions(ctx, nodeID, userID)
	isLast := submissionIndex >= params.MaxSubmissions

	// 1. 合格判定（heritage-judge）
	qualified, err := s.judgeByAI(ctx, text)
	if err != nil {
		return nil, err
	}

	// 2. 辅导（heritage-coaching）
	// 合格或达到最大提交次数走"辅导+范文"；未合格且未达上限走"仅辅导"
	responseMode := "仅辅导"
	if qualified || isLast {
		responseMode = "辅导+范文"
	}
	coaching, err := s.coachByAI(ctx, params, nodeID, userID, text, qualified, submissionIndex, responseMode)
	if err != nil {
		// 辅导 AI 失败：降级为引导话术并保留合格判定，不阻断本次提交、不白耗提交名额
		log.Printf("heritage-cultural coaching ai failed: %v", err)
		coaching = &HeritageCoachingAIResult{Feedback: "请围绕一个意思把这段话讲清楚，再试一试。"}
	}

	res := &AsyncSubmitResult{
		IsPassed:    qualified,
		IsCompleted: qualified || isLast,
		Feedback:    coaching.Feedback,
	}
	if res.Feedback == "" {
		res.Feedback = "已收到你的讲解，请根据提示再试一试。"
	}
	if coaching.RevisedExample != "" {
		res.ReferenceAnswer = coaching.RevisedExample
	}
	res.ResultJSON = &heritageCulturalResultJSON{
		IsQualified:    qualified,
		RevisedExample: coaching.RevisedExample,
	}
	return res, nil
}

// finalizeProgress 在提交结果回写后更新讲解优秀文化进度的聚合字段（error_count / completed）。
func (s *HeritageCulturalService) finalizeProgress(ctx context.Context, classID, nodeID, userID int64, params *HeritageCulturalParamsDef, sub *model.NodeSubmission) error {
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
	prog.ErrorCount = s.countNodeErrors(ctx, nodeID, userID)
	if s.isNodeCompleted(ctx, params, nodeID, userID) {
		prog.Completed = true
	}
	if prog.ID == 0 {
		_, err = s.repos.NodeProgress.Create(ctx, prog)
	} else {
		err = s.repos.NodeProgress.Update(ctx, prog)
	}
	return err
}

// GetSubmission 按 submitId 查询单条提交记录。
// 防御性要求：仅允许提交记录所属用户本人查询，防止越权读取他人作答与批改结果。
func (s *HeritageCulturalService) GetSubmission(ctx context.Context, nodeID, userID int64, submitID string) (*api.HeritageCulturalSubmitResult, error) {
	sub, err := s.repos.NodeSubmission.GetByCode(ctx, submitID)
	if err != nil {
		return nil, ErrQuestionNotFound
	}
	if sub.NodeID != nodeID || sub.NodeType != model.NodeTypeHeritageCultural || sub.UserID != userID {
		return nil, ErrQuestionNotFound
	}
	resp := sub.ToHeritageCulturalResp()
	return &resp, nil
}

// GetEnding 结束任务，返回总评信息。
//
// 双分支：
//   - 节点已完成（合格或达到最大提交次数）：返回综合评价。优先读缓存（提交完成时已异步生成），
//     无缓存时同步生成（与报告路径共用单飞锁），生成中返回"AI 生成中"状态；
//   - 节点未完成：不触发综合评价，返回最后一次提交的辅导反馈（coaching 提示词产物）。
//
// 防御性要求：确认用户是该班级成员，防止非本班用户访问。
func (s *HeritageCulturalService) GetEnding(ctx context.Context, classID, nodeID, userID int64) (*HeritageCulturalEndingResp, error) {
	// 0. 防御性校验：确认用户是该班级成员
	if err := s.repos.EnsureClassMember(ctx, classID, userID); err != nil {
		return nil, err
	}
	params, err := loadHeritageCulturalParams(ctx, s.repos, nodeID)
	if err != nil {
		return nil, err
	}
	subs, err := s.repos.NodeSubmission.ListByUserNode(ctx, userID, nodeID, 200, 0)
	if err != nil {
		return nil, fmt.Errorf("list submissions: %w", err)
	}

	// 1. 节点未完成：返回最后一次提交的辅导反馈（不触发总评）
	if !s.isNodeCompleted(ctx, params, nodeID, userID) {
		resp := &HeritageCulturalEndingResp{
			IsProcessing: false,
			Passed:       false,
			Revealed:     false,
		}
		resp.ErrorCount = s.countNodeErrors(ctx, nodeID, userID)
		if latest := latestSubmitLog(subs); latest != nil {
			if latest.Feedback != nil {
				resp.Comment = *latest.Feedback
			}
		}
		if resp.Comment == "" {
			resp.Comment = "请围绕一个意思把这段话讲清楚，再试一试。"
		}
		return resp, nil
	}

	// 2. 节点已完成：返回综合评价（读缓存 / 同步生成）
	resp := &HeritageCulturalEndingResp{
		IsProcessing: false,
		Passed:       s.hasQualified(subs),
		Revealed:     false,
	}
	resp.ErrorCount = s.countNodeErrors(ctx, nodeID, userID)
	if resp.Passed {
		resp.CorrectAnswer = latestRevisedExample(subs)
	}

	// 2.1 AI 未配置或未提供 comprehensive 提示词：降级返回基础评价
	if s.llm == nil || comprehensivePrompt(s.prompts, model.NodeTypeHeritageCultural) == "" {
		return resp, nil
	}
	// 2.2 无作答记录（不应发生，防御性兜底）：降级返回基础评价
	if len(subs) == 0 {
		return resp, nil
	}

	nodeTitle := ""
	if n, err := s.repos.ClassNode.GetByID(ctx, nodeID); err == nil {
		nodeTitle = n.Title
	}

	// 2.3 读缓存；无缓存时加锁同步生成（与报告路径共用单飞锁）
	cached, err := loadCachedEvaluation(ctx, s.repos, nodeID, userID)
	if err != nil {
		return nil, err
	}
	if cached == nil {
		if err := ensureNodeEvaluation(ctx, s.repos, s.llm, s.prompts, model.NodeTypeHeritageCultural, nodeID, userID, nodeTitle); err != nil {
			if errors.Is(err, ErrEvaluationGenerating) {
				resp.IsProcessing = true
				resp.Comment = "AI 综合评价生成中，请稍后刷新"
				return resp, nil
			}
			return nil, err
		}
		cached, err = loadCachedEvaluation(ctx, s.repos, nodeID, userID)
		if err != nil {
			return nil, err
		}
	}
	if cached != nil {
		s.applyEndingEvaluation(resp, *cached)
	}
	return resp, nil
}

// applyEndingEvaluation 将综合评价结果映射到结束语响应。
func (s *HeritageCulturalService) applyEndingEvaluation(resp *HeritageCulturalEndingResp, res reportComprehensiveResult) {
	if res.StudentFeedback != "" {
		resp.StudentFeedback = res.StudentFeedback
		resp.Comment = res.StudentFeedback
	}
	if res.TeacherEvaluation != "" {
		resp.TeacherEvaluation = res.TeacherEvaluation
	}
	if res.TeacherSuggestion != "" {
		resp.TeacherSuggestion = res.TeacherSuggestion
	}
	if res.ScoreGrade != "" {
		resp.ScoreGrade = res.ScoreGrade
	}
	if res.ScoreValue != "" {
		resp.ScoreValue = string(res.ScoreValue)
	}
	if res.ScoreReason != "" {
		resp.ScoreReason = res.ScoreReason
	}
	if len(res.CultureKeywords) > 0 {
		resp.CultureKeywords = res.CultureKeywords
	}
	if len(res.ErrorKeywords) > 0 {
		resp.ErrorKeywords = res.ErrorKeywords
	}
}

// --- AI 调用 ---

// judgeByAI 调用 heritage-judge 提示词进行合格判定。
// AI 未配置或提示词缺失时降级为"不合格"（不阻塞提交，仍走辅导流程）；AI 调用失败返回错误。
func (s *HeritageCulturalService) judgeByAI(ctx context.Context, text string) (bool, error) {
	if s.llm == nil || s.judgePrompt() == "" {
		return false, nil
	}
	var res HeritageJudgeAIResult
	if err := callAIWithJSON(ctx, s.llm, s.judgePrompt(), HeritageJudgeAIInput{StudentInput: text}, &res); err != nil {
		return false, err
	}
	return res.IsQualified, nil
}

// coachByAI 调用 heritage-coaching 提示词生成辅导反馈与优化范文。
// AI 未配置或提示词缺失时降级返回本地占位反馈；AI 调用失败返回错误。
func (s *HeritageCulturalService) coachByAI(ctx context.Context, params *HeritageCulturalParamsDef, nodeID, userID int64, text string, qualified bool, submissionIndex int, responseMode string) (*HeritageCoachingAIResult, error) {
	if s.llm == nil || s.coachingPrompt() == "" {
		return &HeritageCoachingAIResult{Feedback: "请围绕一个意思把这段话讲清楚，再试一试。"}, nil
	}
	input := s.buildCoachingInput(ctx, params, nodeID, userID, text, qualified, submissionIndex, responseMode)
	var res HeritageCoachingAIResult
	if err := callAIWithJSON(ctx, s.llm, s.coachingPrompt(), input, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// buildCoachingInput 组装传给 AI 的辅导数据包（字段与 prompts/heritage-coaching.md 对齐）。
func (s *HeritageCulturalService) buildCoachingInput(ctx context.Context, params *HeritageCulturalParamsDef, nodeID, userID int64, text string, qualified bool, submissionIndex int, responseMode string) HeritageCoachingAIInput {
	input := HeritageCoachingAIInput{
		IsQualified:     qualified,
		SubmissionIndex: submissionIndex,
		MaxSubmissions:  params.MaxSubmissions,
		StudentInput:    text,
		ResponseMode:    responseMode,
	}
	if u, err := s.repos.User.GetByID(ctx, userID); err == nil && u != nil {
		if u.RealName != nil {
			input.StudentName = *u.RealName
		}
		if u.Gender != nil {
			input.Gender = *u.Gender
		}
		if u.City != nil {
			input.City = *u.City
		}
	}
	input.History = s.buildCoachingHistory(ctx, nodeID, userID)
	return input
}

// buildCoachingHistory 组装历史交互记录（此前已回写的提交：学生输入 + AI 反馈 + 判定），保持时间升序。
func (s *HeritageCulturalService) buildCoachingHistory(ctx context.Context, nodeID, userID int64) []HeritageCoachingHistoryItem {
	subs, err := s.repos.NodeSubmission.ListByUserNode(ctx, userID, nodeID, 200, 0)
	if err != nil {
		return nil
	}
	var history []HeritageCoachingHistoryItem
	index := 0
	// ListByUserNode 按时间倒序，倒序遍历得到升序
	for i := len(subs) - 1; i >= 0; i-- {
		sub := subs[i]
		if sub.SubmitType != "submit" || sub.IsPassed == nil {
			continue // 仅取已回写判定结果的提交
		}
		index++
		var p struct {
			Text string `json:"text"`
		}
		if len(sub.PayloadJSON) > 0 {
			_ = json.Unmarshal(sub.PayloadJSON, &p)
		}
		fb := ""
		if sub.Feedback != nil {
			fb = *sub.Feedback
		}
		history = append(history, HeritageCoachingHistoryItem{
			SubmissionIndex: index,
			StudentInput:    p.Text,
			AIFeedback:      fb,
			IsQualified:     *sub.IsPassed,
		})
	}
	return history
}

// judgePrompt 返回判定提示词（heritage-judge.md）。
func (s *HeritageCulturalService) judgePrompt() string {
	if p, ok := s.prompts.Types["heritage-judge"]; ok && p != "" {
		return p
	}
	return ""
}

// coachingPrompt 返回辅导提示词（heritage-coaching.md）。
func (s *HeritageCulturalService) coachingPrompt() string {
	if p, ok := s.prompts.Types["heritage-coaching"]; ok && p != "" {
		return p
	}
	return ""
}

// --- 进度统计 ---

// countSubmissions 统计该节点该用户的 submit 类型提交次数。
func (s *HeritageCulturalService) countSubmissions(ctx context.Context, nodeID, userID int64) int {
	subs, err := s.repos.NodeSubmission.ListByUserNode(ctx, userID, nodeID, 200, 0)
	if err != nil {
		return 0
	}
	count := 0
	for _, sub := range subs {
		if sub.SubmitType == "submit" {
			count++
		}
	}
	return count
}

// countNodeErrors 统计该节点该用户不合格（未通过判定）的提交次数（供进度聚合 error_count）。
func (s *HeritageCulturalService) countNodeErrors(ctx context.Context, nodeID, userID int64) int {
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

// hasQualified 判断是否存在合格提交记录。
func (s *HeritageCulturalService) hasQualified(subs []model.NodeSubmission) bool {
	for _, sub := range subs {
		if sub.IsPassed != nil && *sub.IsPassed {
			return true
		}
	}
	return false
}

// isNodeCompleted 判断节点是否完成：存在合格提交，或提交次数已达最大上限。
func (s *HeritageCulturalService) isNodeCompleted(ctx context.Context, params *HeritageCulturalParamsDef, nodeID, userID int64) bool {
	subs, err := s.repos.NodeSubmission.ListByUserNode(ctx, userID, nodeID, 200, 0)
	if err != nil {
		return false
	}
	if s.hasQualified(subs) {
		return true
	}
	count := 0
	for _, sub := range subs {
		if sub.SubmitType == "submit" {
			count++
		}
	}
	return count >= params.MaxSubmissions
}

// latestSubmitLog 返回最近一次（按提交时间最新）的 submit 提交记录。
func latestSubmitLog(subs []model.NodeSubmission) *model.NodeSubmission {
	var latest *model.NodeSubmission
	for i := range subs {
		sub := &subs[i]
		if sub.SubmitType != "submit" {
			continue
		}
		if latest == nil || sub.SubmittedAt.After(latest.SubmittedAt) {
			latest = sub
		}
	}
	return latest
}

// latestRevisedExample 返回最近一次完成提交的优化范文（result_json.revised_example）。
// subs 来自 ListByUserNode（submitted_at DESC，最新在前），正序遍历第一条命中即最新。
func latestRevisedExample(subs []model.NodeSubmission) string {
	for i := 0; i < len(subs); i++ {
		sub := &subs[i]
		if sub.SubmitType != "submit" || len(sub.ResultJSON) == 0 {
			continue
		}
		var rj heritageCulturalResultJSON
		if json.Unmarshal(sub.ResultJSON, &rj) == nil && rj.RevisedExample != "" {
			return rj.RevisedExample
		}
	}
	return ""
}
