package model

import (
	"encoding/json"
	"time"
)

// CreationTask 创作工坊任务定义表行（对应 creation_tasks 表）。
// code 字段对应对外 taskId；task_type 为 4 种子任务之一。
type CreationTask struct {
	ID         int64             `db:"id" json:"id"`
	Code       string            `db:"code" json:"code"`
	NodeID     int64             `db:"node_id" json:"nodeId"`
	TaskType   CreationTaskType  `db:"task_type" json:"taskType"`
	Title      *string           `db:"title" json:"title,omitempty"`
	ConfigJSON json.RawMessage   `db:"config_json" json:"configJson,omitempty"`
	CreatedAt  time.Time         `db:"created_at" json:"createdAt"`
	UpdatedAt  time.Time         `db:"updated_at" json:"updatedAt"`
}

// CreationSubmission 创作工坊提交表行（对应 creation_submissions 表）。
// code 字段对应对外 submitId；status 表示处理状态（pending/processing/successful/published）。
type CreationSubmission struct {
	ID                 int64            `db:"id" json:"id"`
	Code               string           `db:"code" json:"code"`
	TaskID             int64            `db:"task_id" json:"taskId"`
	NodeID             int64            `db:"node_id" json:"nodeId"`
	UserID             int64            `db:"user_id" json:"userId"`
	ClassID            int64            `db:"class_id" json:"classId"`
	TaskType           CreationTaskType `db:"task_type" json:"taskType"`
	TextContent        *string          `db:"text_content" json:"textContent,omitempty"`
	ContentJSON        json.RawMessage  `db:"content_json" json:"contentJson"`
	AIImageResourceID  *int64           `db:"ai_image_resource_id" json:"aiImageResourceId,omitempty"`
	AIFeedback         *string          `db:"ai_feedback" json:"aiFeedback,omitempty"`
	AIRevisedContent   *string          `db:"ai_revised_content" json:"aiRevisedContent,omitempty"`
	AIEvaluationJSON   json.RawMessage  `db:"ai_evaluation_json" json:"aiEvaluationJson,omitempty"`
	Status             CreationStatus   `db:"status" json:"status"`
	IsProcessing       bool             `db:"is_processing" json:"isProcessing"`
	IsSuccessful       bool             `db:"is_successful" json:"isSuccessful"`
	IsCompleted        bool             `db:"is_completed" json:"isCompleted"`
	IsPublished        bool             `db:"is_published" json:"isPublished"`
	Duration           int              `db:"duration" json:"duration"`
	PointsEarned       int              `db:"points_earned" json:"pointsEarned"`
	SubmittedAt        time.Time        `db:"submitted_at" json:"submittedAt"`
	PublishedAt        *time.Time       `db:"published_at" json:"publishedAt,omitempty"`
	CreatedAt          time.Time        `db:"created_at" json:"createdAt"`
	UpdatedAt          time.Time        `db:"updated_at" json:"updatedAt"`
}
