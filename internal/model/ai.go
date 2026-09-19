package model

import (
	"time"
)

// AISession AI 会话表行（对应 ai_sessions 表，Code 使用 sess_ 前缀）。
type AISession struct {
	ID             int64     `db:"id" json:"id"`
	Code           string    `db:"code" json:"code"`
	UserID         *int64    `db:"user_id" json:"userId,omitempty"`
	ClassID        *int64    `db:"class_id" json:"classId,omitempty"`
	NodeID         *int64    `db:"node_id" json:"nodeId,omitempty"`
	BizType        *string   `db:"biz_type" json:"bizType,omitempty"`
	Title          *string   `db:"title" json:"title,omitempty"`
	PromptTemplate *string   `db:"prompt_template" json:"promptTemplate,omitempty"`
	Status         string    `db:"status" json:"status"`
	CreatedAt      time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt      time.Time `db:"updated_at" json:"updatedAt"`
}

// AIMessage AI 消息表行（对应 ai_messages 表）。
type AIMessage struct {
	ID               int64             `db:"id" json:"id"`
	SessionID        int64             `db:"session_id" json:"sessionId"`
	Role             AIMessageRole     `db:"role" json:"role"`
	Content          string            `db:"content" json:"content"`
	FinishReason     *ChatFinishReason `db:"finish_reason" json:"finishReason,omitempty"`
	PromptTokens     *int              `db:"prompt_tokens" json:"promptTokens,omitempty"`
	CompletionTokens *int              `db:"completion_tokens" json:"completionTokens,omitempty"`
	TotalTokens      *int              `db:"total_tokens" json:"totalTokens,omitempty"`
	ModelName        *string           `db:"model_name" json:"modelName,omitempty"`
	Seq              int               `db:"seq" json:"seq"`
	CreatedAt        time.Time         `db:"created_at" json:"createdAt"`
}

// AISearchReference AI 搜索引用表行（对应 ai_search_references 表）。
type AISearchReference struct {
	ID        int64     `db:"id" json:"id"`
	MessageID int64     `db:"message_id" json:"messageId"`
	RefIndex  int       `db:"ref_index" json:"refIndex"`
	Title     string    `db:"title" json:"title"`
	Link      string    `db:"link" json:"link"`
	Snippet   *string   `db:"snippet" json:"snippet,omitempty"`
	CreatedAt time.Time `db:"created_at" json:"createdAt"`
}
