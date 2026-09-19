package repository

import (
	"context"
	"fmt"

	"zhonghuawenhua_backend/internal/model"
)

// CreationTaskRepository 创作工坊任务定义数据访问。
type CreationTaskRepository struct {
	*Base
}

// NewCreationTaskRepository 创建 CreationTaskRepository。
func NewCreationTaskRepository(base *Base) *CreationTaskRepository {
	return &CreationTaskRepository{Base: base}
}

// Create 插入任务定义，返回新生成的 id。
func (r *CreationTaskRepository) Create(ctx context.Context, m *model.CreationTask) (int64, error) {
	const q = `INSERT INTO creation_tasks (code, node_id, task_type, title, config_json)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`
	var id int64
	err := r.QueryRow(ctx, &id, q, m.Code, m.NodeID, m.TaskType, m.Title, m.ConfigJSON)
	if err != nil {
		return 0, fmt.Errorf("creation_task create: %w", err)
	}
	return id, nil
}

// GetByID 按主键查询任务定义。
func (r *CreationTaskRepository) GetByID(ctx context.Context, id int64) (*model.CreationTask, error) {
	const q = `SELECT id, code, node_id, task_type, title, config_json, created_at, updated_at
		FROM creation_tasks WHERE id = $1`
	var t model.CreationTask
	if err := r.QueryRow(ctx, &t, q, id); err != nil {
		return nil, fmt.Errorf("creation_task get by id: %w", err)
	}
	return &t, nil
}

// GetByCode 按 code 查询任务定义。
func (r *CreationTaskRepository) GetByCode(ctx context.Context, code string) (*model.CreationTask, error) {
	const q = `SELECT id, code, node_id, task_type, title, config_json, created_at, updated_at
		FROM creation_tasks WHERE code = $1`
	var t model.CreationTask
	if err := r.QueryRow(ctx, &t, q, code); err != nil {
		return nil, fmt.Errorf("creation_task get by code: %w", err)
	}
	return &t, nil
}

// ListByNodeID 按节点 ID 查询任务定义列表。
func (r *CreationTaskRepository) ListByNodeID(ctx context.Context, nodeID int64) ([]model.CreationTask, error) {
	const q = `SELECT id, code, node_id, task_type, title, config_json, created_at, updated_at
		FROM creation_tasks WHERE node_id = $1 ORDER BY id ASC`
	var list []model.CreationTask
	if err := r.Query(ctx, &list, q, nodeID); err != nil {
		return nil, fmt.Errorf("creation_task list by node: %w", err)
	}
	return list, nil
}

// Update 更新任务定义可变字段。
func (r *CreationTaskRepository) Update(ctx context.Context, m *model.CreationTask) error {
	const q = `UPDATE creation_tasks SET code = $1, task_type = $2, title = $3, config_json = $4
		WHERE id = $5`
	if err := r.Exec(ctx, q, m.Code, m.TaskType, m.Title, m.ConfigJSON, m.ID); err != nil {
		return fmt.Errorf("creation_task update: %w", err)
	}
	return nil
}

// Delete 按主键删除任务定义。
func (r *CreationTaskRepository) Delete(ctx context.Context, id int64) error {
	const q = `DELETE FROM creation_tasks WHERE id = $1`
	if err := r.Exec(ctx, q, id); err != nil {
		return fmt.Errorf("creation_task delete: %w", err)
	}
	return nil
}

// CreationSubmissionRepository 创作工坊提交数据访问。
type CreationSubmissionRepository struct {
	*Base
}

// NewCreationSubmissionRepository 创建 CreationSubmissionRepository。
func NewCreationSubmissionRepository(base *Base) *CreationSubmissionRepository {
	return &CreationSubmissionRepository{Base: base}
}

// Create 插入工坊提交，返回新生成的 id。
func (r *CreationSubmissionRepository) Create(ctx context.Context, m *model.CreationSubmission) (int64, error) {
	const q = `INSERT INTO creation_submissions (code, task_id, node_id, user_id, class_id, task_type,
		text_content, content_json, ai_image_resource_id, ai_feedback, ai_revised_content,
		ai_evaluation_json, status, is_processing, is_successful, is_completed, is_published,
		duration, points_earned, submitted_at, published_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
		RETURNING id`
	var id int64
	err := r.QueryRow(ctx, &id, q,
		m.Code, m.TaskID, m.NodeID, m.UserID, m.ClassID, m.TaskType,
		m.TextContent, m.ContentJSON, m.AIImageResourceID, m.AIFeedback, m.AIRevisedContent,
		m.AIEvaluationJSON, m.Status, m.IsProcessing, m.IsSuccessful, m.IsCompleted, m.IsPublished,
		m.Duration, m.PointsEarned, m.SubmittedAt, m.PublishedAt,
	)
	if err != nil {
		return 0, fmt.Errorf("creation_submission create: %w", err)
	}
	return id, nil
}

// GetByID 按主键查询工坊提交。
func (r *CreationSubmissionRepository) GetByID(ctx context.Context, id int64) (*model.CreationSubmission, error) {
	const q = `SELECT id, code, task_id, node_id, user_id, class_id, task_type, text_content,
		content_json, ai_image_resource_id, ai_feedback, ai_revised_content, ai_evaluation_json,
		status, is_processing, is_successful, is_completed, is_published, duration, points_earned,
		submitted_at, published_at, created_at, updated_at
		FROM creation_submissions WHERE id = $1`
	var s model.CreationSubmission
	if err := r.QueryRow(ctx, &s, q, id); err != nil {
		return nil, fmt.Errorf("creation_submission get by id: %w", err)
	}
	return &s, nil
}

// GetByCode 按 code 查询工坊提交。
func (r *CreationSubmissionRepository) GetByCode(ctx context.Context, code string) (*model.CreationSubmission, error) {
	const q = `SELECT id, code, task_id, node_id, user_id, class_id, task_type, text_content,
		content_json, ai_image_resource_id, ai_feedback, ai_revised_content, ai_evaluation_json,
		status, is_processing, is_successful, is_completed, is_published, duration, points_earned,
		submitted_at, published_at, created_at, updated_at
		FROM creation_submissions WHERE code = $1`
	var s model.CreationSubmission
	if err := r.QueryRow(ctx, &s, q, code); err != nil {
		return nil, fmt.Errorf("creation_submission get by code: %w", err)
	}
	return &s, nil
}

// ListByUserNode 按用户+节点分页查询提交。
func (r *CreationSubmissionRepository) ListByUserNode(ctx context.Context, userID, nodeID int64, limit, offset int) ([]model.CreationSubmission, error) {
	const q = `SELECT id, code, task_id, node_id, user_id, class_id, task_type, text_content,
		content_json, ai_image_resource_id, ai_feedback, ai_revised_content, ai_evaluation_json,
		status, is_processing, is_successful, is_completed, is_published, duration, points_earned,
		submitted_at, published_at, created_at, updated_at
		FROM creation_submissions
		WHERE user_id = $1 AND node_id = $2
		ORDER BY submitted_at DESC LIMIT $3 OFFSET $4`
	var list []model.CreationSubmission
	if err := r.Query(ctx, &list, q, userID, nodeID, limit, offset); err != nil {
		return nil, fmt.Errorf("creation_submission list by user node: %w", err)
	}
	return list, nil
}

// ListByTaskID 按任务 ID 分页查询提交。
func (r *CreationSubmissionRepository) ListByTaskID(ctx context.Context, taskID int64, limit, offset int) ([]model.CreationSubmission, error) {
	const q = `SELECT id, code, task_id, node_id, user_id, class_id, task_type, text_content,
		content_json, ai_image_resource_id, ai_feedback, ai_revised_content, ai_evaluation_json,
		status, is_processing, is_successful, is_completed, is_published, duration, points_earned,
		submitted_at, published_at, created_at, updated_at
		FROM creation_submissions
		WHERE task_id = $1
		ORDER BY submitted_at DESC LIMIT $2 OFFSET $3`
	var list []model.CreationSubmission
	if err := r.Query(ctx, &list, q, taskID, limit, offset); err != nil {
		return nil, fmt.Errorf("creation_submission list by task: %w", err)
	}
	return list, nil
}

// Update 更新工坊提交可变字段（用于 AI 反馈回写、发布状态变更）。
func (r *CreationSubmissionRepository) Update(ctx context.Context, m *model.CreationSubmission) error {
	const q = `UPDATE creation_submissions SET text_content = $1, content_json = $2,
		ai_image_resource_id = $3, ai_feedback = $4, ai_revised_content = $5, ai_evaluation_json = $6,
		status = $7, is_processing = $8, is_successful = $9, is_completed = $10, is_published = $11,
		duration = $12, points_earned = $13, published_at = $14 WHERE id = $15`
	if err := r.Exec(ctx, q,
		m.TextContent, m.ContentJSON,
		m.AIImageResourceID, m.AIFeedback, m.AIRevisedContent, m.AIEvaluationJSON,
		m.Status, m.IsProcessing, m.IsSuccessful, m.IsCompleted, m.IsPublished,
		m.Duration, m.PointsEarned, m.PublishedAt, m.ID,
	); err != nil {
		return fmt.Errorf("creation_submission update: %w", err)
	}
	return nil
}

// Delete 按主键删除工坊提交。
func (r *CreationSubmissionRepository) Delete(ctx context.Context, id int64) error {
	const q = `DELETE FROM creation_submissions WHERE id = $1`
	if err := r.Exec(ctx, q, id); err != nil {
		return fmt.Errorf("creation_submission delete: %w", err)
	}
	return nil
}
