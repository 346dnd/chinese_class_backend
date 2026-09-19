package service

import (
	"time"

	"zhonghuawenhua_backend/internal/api"
)

// --- 写想法 params_json 结构 ---

// WriteThoughtsParams 写想法节点的 params_json 结构。
//
// 示例:
//
//	{
//	  "lesson": "赵州桥",
//	  "questions": [
//	    {"id": "q1", "title": "...", "placeholder": "...", "referenceAnswer": "...", "maxLength": 200},
//	    ...
//	  ]
//	}
type WriteThoughtsParams struct {
	// 以下为 getparams 返回的页面固定信息（title / subtitle / description / introVideo / introBubbleText）
	Title           string                     `json:"title,omitempty"`
	Subtitle        string                     `json:"subtitle,omitempty"`
	Description     []api.StyleOverridableText `json:"description,omitempty"`
	IntroVideo      *api.IntroVideo            `json:"introVideo,omitempty"`
	IntroBubbleText *string                    `json:"introBubbleText,omitempty"`
	Questions       []WriteThoughtQuestionDef  `json:"questions"`
}

// WriteThoughtQuestionDef 写想法单题定义。
type WriteThoughtQuestionDef struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Placeholder     string `json:"placeholder,omitempty"`
	ReferenceAnswer string `json:"referenceAnswer,omitempty"`
	MaxLength       int    `json:"maxLength,omitempty"`
	// MaxErrors 允许的最大错误次数，到达后自动揭示参考答案并标记为已完成（默认 3）。
	MaxErrors int `json:"maxErrors,omitempty"`
	// VoiceOnly 是否仅允许语音输入。
	VoiceOnly bool `json:"voiceOnly,omitempty"`
}

// FindQuestion 按 ID 查找题目定义，未找到返回 nil。
func (p *WriteThoughtsParams) FindQuestion(qid string) *WriteThoughtQuestionDef {
	for i := range p.Questions {
		if p.Questions[i].ID == qid {
			return &p.Questions[i]
		}
	}
	return nil
}

// --- 写想法提交请求 ---

// SubmitRequest 提交请求（来自 SubmitWriteThoughtsJSONBody）。
type SubmitRequest struct {
	QuestionID string `json:"questionId"`
	Input      string `json:"input"`
	Type       string `json:"type"` // "submit" | "get-answer"
}

// --- 写想法 AI 评分内部结构 ---

// writeThoughtAIInput 传给 AI 的输入数据。
type writeThoughtAIInput struct {
	StudentName     string `json:"student_name"`
	Gender          string `json:"gender"`
	City            string `json:"city"`
	CurrentLesson   string `json:"current_lesson"`
	QuestionTitle   string `json:"question_title"`
	InstructionMode string `json:"instruction_mode"`
	StudentInput    string `json:"student_input"`
}

// writeThoughtAIResult AI 返回的评分结果。
type writeThoughtAIResult struct {
	IsPass          bool   `json:"is_pass"`
	FeedbackText    string `json:"feedback_text"`
	ReferenceAnswer string `json:"reference_answer"`
}

// --- 写想法草稿结构 ---

// writeThoughtsDraft node_progress.draft_json 中 write-thoughts 分区的草稿结构。
type writeThoughtsDraft struct {
	EnteredAt *time.Time `json:"entered_at,omitempty"`
}