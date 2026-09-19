package service

import (
	"time"

	"zhonghuawenhua_backend/internal/api"
)

// --- 初步感悟 params_json 结构 ---

// InitialInsightParams 初步感悟节点的 params_json 结构。
//
// 示例:
//
//	{
//	  "questions": [
//	    {
//	      "id": "q1",
//	      "title": "一、在《赵州桥》的课文中",
//	      "content": [
//	        {"type": "text", "value": {"text": "赵州桥非常", "style": {}}},
//	        {"type": "blank", "id": "b1", "placeholder": "点击输入", "referenceAnswer": "雄伟"},
//	        {"type": "text", "value": {"text": "，全长...", "style": {}}}
//	      ]
//	    }
//	  ]
//	}
type InitialInsightParams struct {
	// 以下为 getparams 返回的页面固定信息（title / subtitle / description / introVideo / introBubbleText）
	Title           string                     `json:"title,omitempty"`
	Subtitle        string                     `json:"subtitle,omitempty"`
	Description     []api.StyleOverridableText `json:"description,omitempty"`
	IntroVideo      *api.IntroVideo            `json:"introVideo,omitempty"`
	IntroBubbleText *string                    `json:"introBubbleText,omitempty"`
	// Ending 结束语配置（可选），由 getending 接口返回。
	Ending    *InitialInsightEndingDef    `json:"ending,omitempty"`
	Questions []InitialInsightQuestionDef `json:"questions"`
}

// InitialInsightEndingDef 初步感悟结束语配置。
type InitialInsightEndingDef struct {
	// Comment 结束语/点评文本。
	Comment string `json:"comment,omitempty"`
}

// InitialInsightQuestionDef 初步感悟单题定义。
type InitialInsightQuestionDef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// Content 题目内容片段，按顺序渲染；blank 类型的项含填空位定义。
	Content []InitialInsightContentDef `json:"content"`
}

// InitialInsightContentDef 初步感悟题目内容片段。
//
// type=text 时 Text 字段有效；
// type=blank 时 Blank 字段有效。
type InitialInsightContentDef struct {
	Type  string                  `json:"type"`
	Text  *string                 `json:"text,omitempty"`  // type=text 时的纯文本（简化版，忽略 style）
	Blank *InitialInsightBlankDef `json:"blank,omitempty"` // type=blank 时的填空位定义
}

// InitialInsightBlankDef 初步感悟填空位定义。
type InitialInsightBlankDef struct {
	ID               string `json:"id"`
	Placeholder      string `json:"placeholder,omitempty"`
	RetryPlaceholder string `json:"retryPlaceholder,omitempty"`
	ReferenceAnswer  string `json:"referenceAnswer,omitempty"`
	// MaxErrors 允许的最大错误次数，到达后自动标记为已完成（默认 3）。
	MaxErrors int `json:"maxErrors,omitempty"`
}

// FindQuestion 按 ID 查找题目定义，未找到返回 nil。
func (p *InitialInsightParams) FindQuestion(qid string) *InitialInsightQuestionDef {
	for i := range p.Questions {
		if p.Questions[i].ID == qid {
			return &p.Questions[i]
		}
	}
	return nil
}

// initialInsightQuestionDTO 与 api.InitialInsightState.Questions 的元素类型（内联匿名结构体）保持一致。
// 类型别名确保可直接赋值给 api.InitialInsightState.Questions。
type initialInsightQuestionDTO = struct {
	Content    []api.InitialInsightState_Questions_Content_Item `json:"content"`
	ID         string                                           `json:"id"`
	SubmitLogs []interface{}                                    `json:"submitLogs"`
	Title      string                                           `json:"title"`
}

// --- 初步感悟提交请求 ---

// InitialInsightSubmitRequest 提交请求（来自 SubmitInitialInsightJSONBody）。
type InitialInsightSubmitRequest struct {
	QuestionID string `json:"questionId"`
	BlankID    string `json:"blankId"`
	Input      string `json:"input"`
}

// --- 初步感悟结束语/草稿结构 ---

// InitialInsightEndingResp 结束语接口响应数据（OpenAPI 中为 inline 对象）。
type InitialInsightEndingResp struct {
	IsProcessing bool   `json:"isProcessing"`
	Comment      string `json:"comment"`
}

// initialInsightDraft node_progress.draft_json 中 initial-insight 分区的草稿结构。
// 目前仅记录进入节点时间，供 getending 计算首空耗时。
type initialInsightDraft struct {
	EnteredAt *time.Time `json:"entered_at,omitempty"`
}

// --- AI 综合评价（getending）内部结构 ---

// initialComprehensiveRecord 传给 AI 的单条填空交互记录。
type initialComprehensiveRecord struct {
	LessonTitle   string `json:"lesson_title"`
	QuestionID    string `json:"question_id"`
	BlankIndex    int    `json:"blank_index"`
	StudentAnswer string `json:"student_answer"`
	CorrectAnswer string `json:"correct_answer"`
	IsCorrect     bool   `json:"is_correct"`
	AttemptCount  int    `json:"attempt_count"`
	TimeCost      int    `json:"time_cost"`
}

// initialComprehensiveInput 传给 AI 的综合评价输入数据包。
type initialComprehensiveInput struct {
	StudentName   string                       `json:"student_name"`
	Gender        string                       `json:"gender"`
	City          string                       `json:"city"`
	Records       []initialComprehensiveRecord `json:"records"`
	TotalTime     int                          `json:"total_time"`
	TotalBlanks   int                          `json:"total_blanks"`
	CorrectBlanks int                          `json:"correct_blanks"`
	TotalAttempts int                          `json:"total_attempts"`
	AccuracyRate  float64                      `json:"accuracy_rate"`
}

// initialComprehensiveResult AI 返回的综合评价结果。
type initialComprehensiveResult struct {
	StudentFeedback   string         `json:"student_feedback"`
	TeacherEvaluation string         `json:"teacher_evaluation"`
	TeacherSuggestion string         `json:"teacher_suggestion"`
	ErrorTags         []string       `json:"error_tags"`
	ScoreGrade        string         `json:"score_grade"`
	ScoreValue        flexibleString `json:"score_value"`
	ScoreReason       string         `json:"score_reason"`
}

// --- 初步感悟 AI 单空辅导（initial_insight.md）内部结构 ---

// initialInsightHistoryItem 当前题目内、当前空位之前答错的空位记录。
type initialInsightHistoryItem struct {
	BlankIndex    int    `json:"blank_index"`
	StudentAnswer string `json:"student_answer"`
}

// initialInsightAIInput 传给 initial_insight.md 的辅导输入数据包。
type initialInsightAIInput struct {
	StudentName   string                       `json:"student_name"`
	Gender        string                       `json:"gender"`
	CurrentLesson string                       `json:"current_lesson"`
	QuestionID    string                       `json:"question_id"`
	BlankIndex    int                          `json:"blank_index"`
	StudentAnswer string                       `json:"student_answer"`
	FeedbackMode  string                       `json:"feedback_mode"`
	CorrectAnswer string                       `json:"correct_answer"`
	History       []initialInsightHistoryItem  `json:"history"`
}

// initialInsightAIResult initial_insight.md 返回的辅导结果。
type initialInsightAIResult struct {
	FeedbackText string `json:"feedback_text"`
}
