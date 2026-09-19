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
)

// --- CulturalStyle 文化采风 params_json 结构 ---

// CulturalStyleNodeParams 文化采风节点的 params_json 结构。
type CulturalStyleNodeParams struct {
	Title      string          `json:"title"`
	IntroVideo *api.IntroVideo `json:"introVideo,omitempty"`
	Video      struct {
		URL string `json:"url"`
	} `json:"video"`
	Questions   []CulturalStyleQDef     `json:"questions"`
	Breakpoints *CulturalStyleBpDefs    `json:"breakpoints,omitempty"`
	Ending      *CulturalStyleEndingDef `json:"ending,omitempty"`
}

// CulturalStyleQDef 文化采风题目定义。
type CulturalStyleQDef struct {
	ID            string                `json:"id"`
	Type          string                `json:"type"` // "choice" | "order"
	Title         string                `json:"title"`
	Text          string                `json:"text"`
	Options       []CulturalStyleOptDef `json:"options,omitempty"`
	Items         []CulturalStyleOptDef `json:"items,omitempty"`
	CorrectAnswer string                `json:"correctAnswer,omitempty"` // choice 正确选项 ID
	CorrectOrder  []string              `json:"correctOrder,omitempty"`  // order 正确顺序
	MaxErrors     int                   `json:"maxErrors,omitempty"`
}

// CulturalStyleOptDef 选项/条目定义。
type CulturalStyleOptDef struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

// culturalStyleQuestionDTO 与 api.CulturalStyleState.Questions 元素类型（内联匿名结构体）保持一致。
type culturalStyleQuestionDTO = struct {
	Completed bool   `json:"completed"`
	ID        string `json:"id"`
	Items     *[]struct {
		Content string `json:"content"`
		ID      string `json:"id"`
	} `json:"items,omitempty"`
	Options *[]struct {
		Content string `json:"content"`
		ID      string `json:"id"`
	} `json:"options,omitempty"`
	Text  string                              `json:"text"`
	Title string                              `json:"title"`
	Type  api.CulturalStyleStateQuestionsType `json:"type"`
}

// CulturalStyleBpDefs 交互点定义集合。
type CulturalStyleBpDefs struct {
	DragSort    map[string]CulturalStyleDragSortBpDef    `json:"drag-sort,omitempty"`
	QuickSelect map[string]CulturalStyleQuickSelectBpDef `json:"quick-select,omitempty"`
}

// CulturalStyleDragSortBpDef 拖拽排序交互点定义。
type CulturalStyleDragSortBpDef struct {
	api.CSBpDragSortParams
	CorrectOrder []string `json:"correctOrder"`
	MaxErrors    int      `json:"maxErrors,omitempty"`
}

// CulturalStyleQuickSelectBpDef 快速选择交互点定义。
type CulturalStyleQuickSelectBpDef struct {
	api.CSBpQuickSelectParams
	CorrectAnswer string `json:"correctAnswer"`
	MaxErrors     int    `json:"maxErrors,omitempty"`
}

// CulturalStyleEndingDef 结束语定义。
type CulturalStyleEndingDef struct {
	Title       string     `json:"title,omitempty"`
	Description string     `json:"description,omitempty"`
	Video       *api.Video `json:"video,omitempty"`
}

// loadCulturalStyleParams 加载文化采风节点参数。
func loadCulturalStyleParams(ctx context.Context, repos *Repositories, nodeID int64) (*CulturalStyleNodeParams, error) {
	var p CulturalStyleNodeParams
	if err := loadNodeParams(ctx, repos, nodeID, &p); err != nil {
		return nil, err
	}
	if p.Title == "" {
		return nil, ErrNodeContentNotConfigured
	}
	for i := range p.Questions {
		if p.Questions[i].MaxErrors == 0 {
			p.Questions[i].MaxErrors = 3
		}
	}
	// 遍历并设置交互点默认值
	if p.Breakpoints != nil {
		for k := range p.Breakpoints.DragSort {
			bp := p.Breakpoints.DragSort[k]
			if bp.MaxErrors == 0 {
				bp.MaxErrors = 3
			}
			p.Breakpoints.DragSort[k] = bp
		}
		for k := range p.Breakpoints.QuickSelect {
			bp := p.Breakpoints.QuickSelect[k]
			if bp.MaxErrors == 0 {
				bp.MaxErrors = 3
			}
			p.Breakpoints.QuickSelect[k] = bp
		}
	}
	return &p, nil
}

// FindQuestion 按 ID 查找题目定义，未找到返回 nil。
func (p *CulturalStyleNodeParams) FindQuestion(qid string) *CulturalStyleQDef {
	for i := range p.Questions {
		if p.Questions[i].ID == qid {
			return &p.Questions[i]
		}
	}
	return nil
}

// FindDragSortBp 按 ID 查找拖拽排序交互点定义。
func (p *CulturalStyleNodeParams) FindDragSortBp(bpID string) *CulturalStyleDragSortBpDef {
	if p.Breakpoints == nil {
		return nil
	}
	bp, ok := p.Breakpoints.DragSort[bpID]
	if !ok {
		return nil
	}
	return &bp
}

// FindQuickSelectBp 按 ID 查找快速选择交互点定义。
func (p *CulturalStyleNodeParams) FindQuickSelectBp(bpID string) *CulturalStyleQuickSelectBpDef {
	if p.Breakpoints == nil {
		return nil
	}
	bp, ok := p.Breakpoints.QuickSelect[bpID]
	if !ok {
		return nil
	}
	return &bp
}

// --- CulturalStyleService 文化采风业务服务 ---

// CulturalStyleService 文化采风节点的业务服务。
type CulturalStyleService struct {
	repos   *Repositories
	llm     ai.LLMClient
	prompts config.PromptsConfig
}

// NewCulturalStyleService 创建 CulturalStyleService。
func NewCulturalStyleService(repos *Repositories, llm ai.LLMClient, prompts config.PromptsConfig) *CulturalStyleService {
	return &CulturalStyleService{repos: repos, llm: llm, prompts: prompts}
}

// GetParams 获取文化采风节点静态参数。
// 返回页面固定信息（标题、导入视频、互动视频）。
func (s *CulturalStyleService) GetParams(ctx context.Context, classID, nodeID, userID int64) (*api.CulturalStyleParams, error) {
	if err := s.repos.EnsureClassMember(ctx, classID, userID); err != nil {
		return nil, err
	}
	params, err := loadCulturalStyleParams(ctx, s.repos, nodeID)
	if err != nil {
		return nil, err
	}
	return &api.CulturalStyleParams{
		Title:      params.Title,
		IntroVideo: params.IntroVideo,
		Video:      params.Video,
	}, nil
}

// GetState 获取文化采风节点状态。
// 返回所有题目定义 + 完成状态。
func (s *CulturalStyleService) GetState(ctx context.Context, classID, nodeID, userID int64) (*api.CulturalStyleState, error) {
	if err := s.repos.EnsureClassMember(ctx, classID, userID); err != nil {
		return nil, err
	}
	params, err := loadCulturalStyleParams(ctx, s.repos, nodeID)
	if err != nil {
		return nil, err
	}

	// 拉取该用户该节点全部提交记录
	submissions, err := s.repos.NodeSubmission.ListByUserNode(ctx, userID, nodeID, 200, 0)
	if err != nil {
		return nil, fmt.Errorf("list submissions: %w", err)
	}

	// 统计各题目的完成状态
	completedByQ := make(map[string]bool, len(params.Questions))
	for _, sub := range submissions {
		if sub.QuestionID == nil || sub.NodeType != model.NodeTypeCulturalStyle {
			continue
		}
		if sub.IsCompleted != nil && *sub.IsCompleted {
			completedByQ[*sub.QuestionID] = true
		}
	}

	// 组装响应
	questions := make([]culturalStyleQuestionDTO, 0, len(params.Questions))
	allCompleted := true
	for _, q := range params.Questions {
		completed := completedByQ[q.ID]
		if !completed {
			allCompleted = false
		}
		csq := culturalStyleQuestionDTO{
			ID:        q.ID,
			Title:     q.Title,
			Text:      q.Text,
			Type:      api.CulturalStyleStateQuestionsType(q.Type),
			Completed: completed,
		}
		if len(q.Options) > 0 {
			opts := make([]struct {
				Content string `json:"content"`
				ID      string `json:"id"`
			}, len(q.Options))
			for i, o := range q.Options {
				opts[i] = struct {
					Content string `json:"content"`
					ID      string `json:"id"`
				}{Content: o.Content, ID: o.ID}
			}
			csq.Options = &opts
		}
		if len(q.Items) > 0 {
			items := make([]struct {
				Content string `json:"content"`
				ID      string `json:"id"`
			}, len(q.Items))
			for i, it := range q.Items {
				items[i] = struct {
					Content string `json:"content"`
					ID      string `json:"id"`
				}{Content: it.Content, ID: it.ID}
			}
			csq.Items = &items
		}
		questions = append(questions, csq)
	}

	state := &api.CulturalStyleState{
		Title:     params.Title,
		Video:     api.Video{Src: params.Video.URL},
		Completed: allCompleted,
		Questions: questions,
		Desc:      "",
	}
	return state, nil
}

// SyncTime 同步视频播放进度。
// 播放进度存到 draft_json 的 cultural-style 分区。
func (s *CulturalStyleService) SyncTime(ctx context.Context, classID, nodeID, userID int64, currentTime int) error {
	if err := s.repos.EnsureClassMember(ctx, classID, userID); err != nil {
		return err
	}
	d := culturalStyleDraft{PlayedTime: &currentTime}
	return saveDraftPartition(ctx, s.repos, classID, nodeID, userID, DraftKeyCulturalStyle, &d)
}

// GetEnding 获取文化采风结束语。
// 播放完毕时返回预设的结束语内容（标题、描述、视频）。
func (s *CulturalStyleService) GetEnding(ctx context.Context, classID, nodeID, userID int64) (*CulturalStyleEndingResp, error) {
	if err := s.repos.EnsureClassMember(ctx, classID, userID); err != nil {
		return nil, err
	}
	params, err := loadCulturalStyleParams(ctx, s.repos, nodeID)
	if err != nil {
		return nil, err
	}
	if params.Ending == nil {
		return &CulturalStyleEndingResp{}, nil
	}
	resp := &CulturalStyleEndingResp{
		Title:       params.Ending.Title,
		Description: params.Ending.Description,
	}
	if params.Ending.Video != nil {
		resp.Video = params.Ending.Video
	}
	return resp, nil
}

// CulturalStyleEndingResp 文化采风结束语响应。
type CulturalStyleEndingResp struct {
	Title       string     `json:"title,omitempty"`
	Description string     `json:"description,omitempty"`
	Video       *api.Video `json:"video,omitempty"`
}

// --- 拖拽排序交互点 ---

// GetDragSortParams 获取拖拽排序交互点参数。
func (s *CulturalStyleService) GetDragSortParams(ctx context.Context, nodeID int64, bpID string) (*api.CSBpDragSortParams, error) {
	params, err := loadCulturalStyleParams(ctx, s.repos, nodeID)
	if err != nil {
		return nil, err
	}
	bp := params.FindDragSortBp(bpID)
	if bp == nil {
		return nil, ErrQuestionNotFound
	}
	return &bp.CSBpDragSortParams, nil
}

// GetDragSortState 获取拖拽排序交互点状态。
// 返回该交互点该用户的所有提交记录。
func (s *CulturalStyleService) GetDragSortState(ctx context.Context, classID, nodeID, userID int64, bpID string) (*api.CSBpDragSortState, error) {
	if err := s.repos.EnsureClassMember(ctx, classID, userID); err != nil {
		return nil, err
	}
	submissions, err := s.repos.NodeSubmission.ListByUserNode(ctx, userID, nodeID, 200, 0)
	if err != nil {
		return nil, fmt.Errorf("list submissions: %w", err)
	}

	logs := make([]api.CSBpDragSortSubmitResult, 0, len(submissions))
	for _, sub := range submissions {
		if sub.QuestionID == nil || *sub.QuestionID != bpID {
			continue
		}
		if sub.SubmitType != "submit" {
			continue
		}
		logs = append(logs, sub.ToCulturalStyleDragSortResp())
	}

	return &api.CSBpDragSortState{SubmitLogs: logs}, nil
}

// SubmitDragSort 提交拖拽排序答案（异步 AI 辅导）。
//
// 异步设计：立即插入 is_processing=true 的提交记录并返回 submitId，
// 后台 goroutine 完成正误判定与 AI 辅导后回写，前端通过 GetDragSortSubmission 轮询 is_processing。
// 错误修正流程：
//   - 第 1 次排错：调用 AI 生成提示文本（Feedback），驱动数字人语音播报，不标记完成，允许重试；
//   - 第 2 次排错：显示正确答案（ReferenceAnswer），同时调用 AI 生成解释（Feedback），数字人播报解释后 3 秒自动关闭。
func (s *CulturalStyleService) SubmitDragSort(ctx context.Context, classID, nodeID, userID int64, bpID string, answer []api.CSBpDragSortEntry) (*api.AsyncSubmitResp, error) {
	if err := s.repos.EnsureClassMember(ctx, classID, userID); err != nil {
		return nil, err
	}
	params, err := loadCulturalStyleParams(ctx, s.repos, nodeID)
	if err != nil {
		return nil, err
	}
	bp := params.FindDragSortBp(bpID)
	if bp == nil {
		return nil, ErrQuestionNotFound
	}

	// 校验 answer 长度
	if len(answer) != len(bp.Blanks) {
		return nil, fmt.Errorf("answer length mismatch: expected %d, got %d", len(bp.Blanks), len(answer))
	}

	payload, _ := json.Marshal(map[string]interface{}{"answer": answer})

	// 通用异步提交：立即返回 submitId，后台 goroutine 完成正误判定 + AI 辅导（含按用户锁）
	submitID, err := SubmitAsync(ctx, s.repos, AsyncSubmitTask{
		ClassID:     classID,
		NodeID:      nodeID,
		UserID:      userID,
		NodeType:    model.NodeTypeCulturalStyleDragSort,
		QuestionID:  bpID,
		SubmitType:  "submit",
		PayloadJSON: payload,
		Process: func(ctx context.Context) (*AsyncSubmitResult, error) {
			return s.processDragSortAnswer(ctx, nodeID, userID, bpID, bp, answer)
		},
		FinalizeProgress: func(ctx context.Context, sub *model.NodeSubmission) error {
			return s.finalizeDragSortProgress(ctx, classID, nodeID, userID, sub)
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

// processDragSortAnswer 执行拖拽排序的正误判定与 AI 辅导（在 SubmitAsync 后台 goroutine 中调用）。
// 答对标记完成并返回固定成功句式；答错按本次是否为达上限的那次错误切换 feedback_mode
// （hint=方向指引，explain=答案讲解），达上限时揭示参考答案并标记完成。
func (s *CulturalStyleService) processDragSortAnswer(ctx context.Context, nodeID, userID int64, bpID string, bp *CulturalStyleDragSortBpDef, answer []api.CSBpDragSortEntry) (*AsyncSubmitResult, error) {
	// 判断正误
	if s.checkDragSortAnswer(answer, bp.CorrectOrder) {
		// 固定成功句式，驱动数字人语音播报
		return &AsyncSubmitResult{
			IsPassed:    true,
			IsCompleted: true,
			Feedback:    "你真棒，看来你得到了蔡伦的真传了！",
		}, nil
	}

	// 统计历史失败次数（不含本次：当前提交 is_passed 尚未回写，不会被 countBpErrors 统计）
	prevFail := s.countBpErrors(ctx, userID, nodeID, bpID)
	curFail := prevFail + 1

	res := &AsyncSubmitResult{IsPassed: false}
	if curFail >= bp.MaxErrors {
		// 达到最大错误次数 → 显示正确答案 + AI 解释
		res.IsCompleted = true
		res.Revealed = true
		// 参考答案：correctOrder 对应的 entries
		refs := make([]api.CSBpDragSortEntry, 0, len(bp.CorrectOrder))
		entryMap := make(map[string]string, len(bp.Entries))
		for _, e := range bp.Entries {
			entryMap[e.ID] = e.Text
		}
		for _, id := range bp.CorrectOrder {
			if text, ok := entryMap[id]; ok {
				refs = append(refs, api.CSBpDragSortEntry{ID: id, Text: text})
			}
		}
		if len(refs) > 0 {
			refBytes, _ := json.Marshal(refs)
			res.ReferenceAnswer = string(refBytes)
		}
		// AI 生成解释文本
		if explanation, err := s.dragSortAI(ctx, bp, answer, "explain"); err == nil && explanation != "" {
			res.Feedback = explanation
		}
	} else {
		// 未达到最大错误次数 → 仅返回 AI 提示，允许重试
		if hint, err := s.dragSortAI(ctx, bp, answer, "hint"); err == nil && hint != "" {
			res.Feedback = hint
		}
	}
	return res, nil
}

// finalizeDragSortProgress 在结果回写后更新拖拽排序进度聚合字段（error_count/revealed/completed）。
// attempt_count / last_submit_id 已在 SubmitAsync 事务内原子更新，此处不重复处理。
func (s *CulturalStyleService) finalizeDragSortProgress(ctx context.Context, classID, nodeID, userID int64, sub *model.NodeSubmission) error {
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
	prog.ErrorCount = s.countAllBpErrors(ctx, userID, nodeID)
	prog.Revealed = prog.Revealed || sub.Revealed
	if sub.IsCompleted != nil && *sub.IsCompleted {
		prog.Completed = true
	}
	if prog.ID == 0 {
		_, err = s.repos.NodeProgress.Create(ctx, prog)
	} else {
		err = s.repos.NodeProgress.Update(ctx, prog)
	}
	return err
}

// GetDragSortSubmission 按 submitID 查询拖拽排序提交记录。
// 防御性要求：仅允许提交记录所属用户本人查询，防止越权读取他人作答与批改结果。
func (s *CulturalStyleService) GetDragSortSubmission(ctx context.Context, nodeID, userID int64, submitID string) (*api.CSBpDragSortSubmitResult, error) {
	sub, err := s.repos.NodeSubmission.GetByCode(ctx, submitID)
	if err != nil {
		return nil, ErrQuestionNotFound
	}
	if sub.NodeID != nodeID || sub.UserID != userID {
		return nil, ErrQuestionNotFound
	}
	resp := sub.ToCulturalStyleDragSortResp()
	return &resp, nil
}

// checkDragSortAnswer 比较用户答案与正确顺序。
func (s *CulturalStyleService) checkDragSortAnswer(answer []api.CSBpDragSortEntry, correctOrder []string) bool {
	if len(answer) != len(correctOrder) {
		return false
	}
	for i, a := range answer {
		if i >= len(correctOrder) || a.ID != correctOrder[i] {
			return false
		}
	}
	return true
}

// dragSortAI 调用 LLM 生成拖拽排序的提示或解释文本。
// mode 为 "hint"（方向指引）或 "explain"（答案讲解）。
func (s *CulturalStyleService) dragSortAI(ctx context.Context, bp *CulturalStyleDragSortBpDef, answer []api.CSBpDragSortEntry, mode string) (string, error) {
	if s.llm == nil {
		return "", nil
	}

	// 获取提示词（文件名 cultural-style.md → Types["cultural-style"]）
	promptKey := "cultural-style"
	prompt := s.prompts.Types[promptKey]
	if prompt == "" {
		// 没有配置提示词，返回默认文本
		if mode == "hint" {
			return "再想想，顺序可能不太对哦，试着换个顺序看看？", nil
		}
		return "正确的顺序已经展示在上面了，记住这个顺序哦！", nil
	}

	// 构建 student_input 和 correct_sequence（文本字符串，用 "—>" 连接）
	entryMap := make(map[string]string, len(bp.Entries))
	for _, e := range bp.Entries {
		entryMap[e.ID] = e.Text
	}

	studentParts := make([]string, 0, len(answer))
	for _, a := range answer {
		if text, ok := entryMap[a.ID]; ok {
			studentParts = append(studentParts, text)
		} else {
			studentParts = append(studentParts, a.ID)
		}
	}

	correctParts := make([]string, 0, len(bp.CorrectOrder))
	for _, id := range bp.CorrectOrder {
		if text, ok := entryMap[id]; ok {
			correctParts = append(correctParts, text)
		}
	}

	instructionMode := "方向指引"
	if mode == "explain" {
		instructionMode = "答案讲解"
	}

	input := map[string]interface{}{
		"student_name":     "",
		"gender":           "",
		"instruction_mode": instructionMode,
		"student_input":    strings.Join(studentParts, "—>"),
		"correct_sequence": strings.Join(correctParts, "—>"),
	}

	resp, err := s.llm.Chat(ctx, &ai.ChatRequest{
		Messages: []ai.Message{
			{Role: "user", Content: mustJSON(input)},
		},
		MaxTokens:    256,
		Temperature:  0.7,
		SystemPrompt: prompt,
	})
	if err != nil {
		return "", fmt.Errorf("llm chat: %w", err)
	}

	// 解析 JSON 输出，提取 feedback_text
	content := strings.TrimSpace(resp.Content)
	var result struct {
		FeedbackText string `json:"feedback_text"`
	}
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		// 解析失败时直接返回原始文本作为兜底
		return content, nil
	}
	return result.FeedbackText, nil
}

// mustJSON 将 v 序列化为 JSON 字符串，失败时返回空对象。
func mustJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// --- 快速选择交互点 ---

// GetQuickSelectParams 获取快速选择交互点参数。
func (s *CulturalStyleService) GetQuickSelectParams(ctx context.Context, nodeID int64, bpID string) (*api.CSBpQuickSelectParams, error) {
	params, err := loadCulturalStyleParams(ctx, s.repos, nodeID)
	if err != nil {
		return nil, err
	}
	bp := params.FindQuickSelectBp(bpID)
	if bp == nil {
		return nil, ErrQuestionNotFound
	}
	return &bp.CSBpQuickSelectParams, nil
}

// SubmitQuickSelect 提交快速选择答案。
// 快速选择没有正确答案，仅为记录学生选择，直接标记完成。
// 保持同步处理，但同样占用按用户提交槽：同一用户同一时间只允许一个提交
// （与异步提交共享同一提交槽），槽被占用时立即拒绝（ErrSubmitInProgress）。
func (s *CulturalStyleService) SubmitQuickSelect(ctx context.Context, classID, nodeID, userID int64, bpID string, selectedID *string) (*model.CulturalStyleQuickSelectSubmitResult, error) {
	if err := s.repos.EnsureClassMember(ctx, classID, userID); err != nil {
		return nil, err
	}
	if !s.repos.SubmitLock.TryAcquire(userID) {
		return nil, ErrSubmitInProgress
	}
	defer s.repos.SubmitLock.Release(userID)

	// 只需验证交互点存在
	params, err := loadCulturalStyleParams(ctx, s.repos, nodeID)
	if err != nil {
		return nil, err
	}
	bp := params.FindQuickSelectBp(bpID)
	if bp == nil {
		return nil, ErrQuestionNotFound
	}

	// 加载当前进度
	prog, err := s.repos.NodeProgress.GetByNodeUser(ctx, nodeID, userID)
	if err != nil {
		prog = nil
	}

	now := time.Now()
	sub := &model.NodeSubmission{
		ClassID:      classID,
		NodeID:       nodeID,
		UserID:       userID,
		NodeType:     model.NodeTypeCulturalStyleQuickSelect,
		QuestionID:   &bpID,
		SubmitType:   "submit",
		SubmittedAt:  now,
		IsProcessing: false,
	}
	payload, _ := json.Marshal(map[string]interface{}{"selectedId": selectedID})
	sub.PayloadJSON = payload

	// 快速选择无正确答案，直接标记完成
	isPassed := true
	sub.IsPassed = &isPassed
	isCompleted := true
	sub.IsCompleted = &isCompleted

	// 插入提交记录
	subID, err := s.repos.NodeSubmission.Create(ctx, sub)
	if err != nil {
		return nil, fmt.Errorf("create submission: %w", err)
	}
	sub.ID = subID

	// 更新进度
	if prog == nil {
		prog = &model.NodeProgress{
			ClassID:      classID,
			NodeID:       nodeID,
			UserID:       userID,
			LastSubmitID: &subID,
		}
	} else {
		prog.LastSubmitID = &subID
	}
	prog.AttemptCount++
	prog.ErrorCount = s.countAllBpErrors(ctx, userID, nodeID)
	if isCompleted {
		prog.Completed = true
	}

	if prog.ID == 0 {
		if _, err := s.repos.NodeProgress.Create(ctx, prog); err != nil {
			return nil, fmt.Errorf("create progress: %w", err)
		}
	} else {
		if err := s.repos.NodeProgress.Update(ctx, prog); err != nil {
			return nil, fmt.Errorf("update progress: %w", err)
		}
	}

	resp := sub.ToCulturalStyleQuickSelectResp()
	return &resp, nil
}

// --- 辅助方法 ---

// culturalStyleDraft draft_json 中 cultural-style 分区的草稿结构。
type culturalStyleDraft struct {
	PlayedTime *int `json:"played_time,omitempty"`
}

// countBpErrors 统计某交互点已累计的错误次数。
func (s *CulturalStyleService) countBpErrors(ctx context.Context, userID, nodeID int64, bpID string) int {
	subs, err := s.repos.NodeSubmission.ListByUserNode(ctx, userID, nodeID, 100, 0)
	if err != nil {
		return 0
	}
	count := 0
	for _, sub := range subs {
		if sub.QuestionID == nil || *sub.QuestionID != bpID {
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

// countAllBpErrors 统计该节点该用户所有交互点的错误次数。
func (s *CulturalStyleService) countAllBpErrors(ctx context.Context, userID, nodeID int64) int {
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
