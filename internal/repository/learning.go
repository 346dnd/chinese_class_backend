package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"zhonghuawenhua_backend/internal/model"
)

// NodeProgressRepository 学生学习进度数据访问。
type NodeProgressRepository struct {
	*Base
}

// NewNodeProgressRepository 创建 NodeProgressRepository。
func NewNodeProgressRepository(base *Base) *NodeProgressRepository {
	return &NodeProgressRepository{Base: base}
}

// Create 插入进度记录，返回新生成的 id。
func (r *NodeProgressRepository) Create(ctx context.Context, m *model.NodeProgress) (int64, error) {
	// draft_json 有 NOT NULL 约束，未设置时默认空对象
	if len(m.DraftJSON) == 0 {
		m.DraftJSON = json.RawMessage("{}")
	}
	const q = `INSERT INTO node_progress (node_id, user_id, class_id, completed, attempt_count,
		error_count, revealed, draft_json, last_submit_id, duration, ai_evaluation_json)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING id`
	var id int64
	err := r.QueryRow(ctx, &id, q,
		m.NodeID, m.UserID, m.ClassID, m.Completed, m.AttemptCount,
		m.ErrorCount, m.Revealed, m.DraftJSON, m.LastSubmitID, m.Duration, m.AIEvaluationJSON,
	)
	if err != nil {
		return 0, fmt.Errorf("node_progress create: %w", err)
	}
	return id, nil
}

// GetByID 按主键查询进度记录。
func (r *NodeProgressRepository) GetByID(ctx context.Context, id int64) (*model.NodeProgress, error) {
	const q = `SELECT id, node_id, user_id, class_id, completed, attempt_count, error_count,
		revealed, draft_json, last_submit_id, duration, ai_evaluation_json, created_at, updated_at
		FROM node_progress WHERE id = $1`
	var p model.NodeProgress
	if err := r.QueryRow(ctx, &p, q, id); err != nil {
		return nil, fmt.Errorf("node_progress get by id: %w", err)
	}
	return &p, nil
}

// GetByNodeUser 按节点 + 用户查询进度记录（新版 UNIQUE(node_id, user_id)）。
// node_id 全局唯一，class_id 仅用于归属查询，不再参与唯一约束。
func (r *NodeProgressRepository) GetByNodeUser(ctx context.Context, nodeID, userID int64) (*model.NodeProgress, error) {
	const q = `SELECT id, node_id, user_id, class_id, completed, attempt_count, error_count,
		revealed, draft_json, last_submit_id, duration, ai_evaluation_json, created_at, updated_at
		FROM node_progress WHERE node_id = $1 AND user_id = $2`
	var p model.NodeProgress
	if err := r.QueryRow(ctx, &p, q, nodeID, userID); err != nil {
		return nil, fmt.Errorf("node_progress get by node user: %w", err)
	}
	return &p, nil
}

// Upsert 按 (node_id, user_id) 冲突时更新可变字段。
func (r *NodeProgressRepository) Upsert(ctx context.Context, m *model.NodeProgress) error {
	const q = `INSERT INTO node_progress (node_id, user_id, class_id, completed, attempt_count,
		error_count, revealed, draft_json, last_submit_id, duration, ai_evaluation_json)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (node_id, user_id) DO UPDATE SET
			completed = EXCLUDED.completed,
			attempt_count = EXCLUDED.attempt_count,
			error_count = EXCLUDED.error_count,
			revealed = EXCLUDED.revealed,
			draft_json = EXCLUDED.draft_json,
			last_submit_id = EXCLUDED.last_submit_id,
			duration = EXCLUDED.duration,
			ai_evaluation_json = EXCLUDED.ai_evaluation_json`
	if err := r.Exec(ctx, q,
		m.NodeID, m.UserID, m.ClassID, m.Completed, m.AttemptCount,
		m.ErrorCount, m.Revealed, m.DraftJSON, m.LastSubmitID, m.Duration, m.AIEvaluationJSON,
	); err != nil {
		return fmt.Errorf("node_progress upsert: %w", err)
	}
	return nil
}

// Update 更新进度可变字段。
func (r *NodeProgressRepository) Update(ctx context.Context, m *model.NodeProgress) error {
	const q = `UPDATE node_progress SET completed = $1, attempt_count = $2, error_count = $3,
		revealed = $4, draft_json = $5, last_submit_id = $6, duration = $7, ai_evaluation_json = $8 WHERE id = $9`
	if err := r.Exec(ctx, q,
		m.Completed, m.AttemptCount, m.ErrorCount, m.Revealed, m.DraftJSON, m.LastSubmitID, m.Duration, m.AIEvaluationJSON, m.ID,
	); err != nil {
		return fmt.Errorf("node_progress update: %w", err)
	}
	return nil
}

// SetAIEvaluation 单独更新某节点用户的 AI 综合评价缓存（节点完成后异步写入，与提交流程解耦）。
func (r *NodeProgressRepository) SetAIEvaluation(ctx context.Context, nodeID, userID int64, evaluation json.RawMessage) error {
	const q = `UPDATE node_progress SET ai_evaluation_json = $1, updated_at = now() WHERE node_id = $2 AND user_id = $3`
	if err := r.Exec(ctx, q, evaluation, nodeID, userID); err != nil {
		return fmt.Errorf("node_progress set ai evaluation: %w", err)
	}
	return nil
}

// BumpAttemptTx 在事务内原子地对某用户某节点的提交计数 +1，并记录最后提交 ID。
// 使用 ON CONFLICT 的原子自增，替代原先的“读-改-写”，消除并发提交下的计数竞态；
// 同时处理首次提交（INSERT）与后续提交（UPDATE）两种情况。
func (r *NodeProgressRepository) BumpAttemptTx(ctx context.Context, tx pgx.Tx, classID, nodeID, userID, subID int64) error {
	const q = `INSERT INTO node_progress (node_id, user_id, class_id, completed, attempt_count,
		error_count, revealed, draft_json, last_submit_id, duration)
		VALUES ($1, $2, $3, false, 1, 0, false, '{}'::jsonb, $4, 0)
		ON CONFLICT (node_id, user_id) DO UPDATE SET
			attempt_count = node_progress.attempt_count + 1,
			last_submit_id = EXCLUDED.last_submit_id,
			updated_at = now()`
	if _, err := tx.Exec(ctx, q, nodeID, userID, classID, subID); err != nil {
		return fmt.Errorf("node_progress bump attempt: %w", err)
	}
	return nil
}

// SetDraftPartition 原子更新 node_progress.draft_json 的指定 moduleKey 分区（SQL 层 jsonb_set 合并），
// 只修改 draft_json 列，避免"读-改-写"整行回写覆盖并发更新的 attempt_count / ai_evaluation_json 等字段（丢失更新）。
// 进度不存在时自动创建；分区不存在时自动创建。
func (r *NodeProgressRepository) SetDraftPartition(ctx context.Context, classID, nodeID, userID int64, moduleKey string, partition json.RawMessage) error {
	const q = `INSERT INTO node_progress (node_id, user_id, class_id, completed, attempt_count,
		error_count, revealed, draft_json, last_submit_id, duration)
		VALUES ($1, $2, $3, false, 0, 0, false, '{}'::jsonb, NULL, 0)
		ON CONFLICT (node_id, user_id) DO UPDATE SET
			draft_json = jsonb_set(COALESCE(node_progress.draft_json, '{}'::jsonb), $4, $5::jsonb, true),
			updated_at = now()`
	if err := r.Exec(ctx, q, nodeID, userID, classID, []string{moduleKey}, partition); err != nil {
		return fmt.Errorf("node_progress set draft partition: %w", err)
	}
	return nil
}

// Delete 按主键删除进度记录。
func (r *NodeProgressRepository) Delete(ctx context.Context, id int64) error {
	const q = `DELETE FROM node_progress WHERE id = $1`
	if err := r.Exec(ctx, q, id); err != nil {
		return fmt.Errorf("node_progress delete: %w", err)
	}
	return nil
}

// NodeSubmissionRepository 节点提交记录数据访问。
type NodeSubmissionRepository struct {
	*Base
}

// NewNodeSubmissionRepository 创建 NodeSubmissionRepository。
func NewNodeSubmissionRepository(base *Base) *NodeSubmissionRepository {
	return &NodeSubmissionRepository{Base: base}
}

// Create 插入提交记录，返回新生成的 id。
// 新版 schema：submit_type / is_passed / is_completed / feedback / reference_answer 等字段。
// code 未设置时自动生成唯一值（对外 submitId，唯一约束保证）。
func (r *NodeSubmissionRepository) Create(ctx context.Context, m *model.NodeSubmission) (int64, error) {
	if m.Code == "" {
		m.Code = fmt.Sprintf("ns%d", time.Now().UnixNano())
	}
	// payload_json / result_json 为 NOT NULL，为空时默认空 JSON 对象
	if len(m.PayloadJSON) == 0 {
		m.PayloadJSON = []byte("{}")
	}
	if len(m.ResultJSON) == 0 {
		m.ResultJSON = []byte("{}")
	}
	const q = `INSERT INTO node_submissions (code, class_id, node_id, user_id, node_type, question_id,
		submit_type, payload_json, result_json, is_processing, is_passed, is_completed, error_count,
		revealed, feedback, reference_answer, audio_resource_id, duration, points_earned, submitted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
		RETURNING id`
	var id int64
	err := r.QueryRow(ctx, &id, q,
		m.Code, m.ClassID, m.NodeID, m.UserID, m.NodeType, m.QuestionID,
		m.SubmitType, m.PayloadJSON, m.ResultJSON, m.IsProcessing, m.IsPassed, m.IsCompleted,
		m.ErrorCount, m.Revealed, m.Feedback, m.ReferenceAnswer, m.AudioResourceID,
		m.Duration, m.PointsEarned, m.SubmittedAt,
	)
	if err != nil {
		return 0, fmt.Errorf("node_submission create: %w", err)
	}
	return id, nil
}

// CreateTx 在事务内插入提交记录，返回新生成的 id。
// 逻辑与 Create 一致，仅将查询绑定到传入的事务，用于与进度更新保持原子性。
func (r *NodeSubmissionRepository) CreateTx(ctx context.Context, tx pgx.Tx, m *model.NodeSubmission) (int64, error) {
	if m.Code == "" {
		m.Code = fmt.Sprintf("ns%d", time.Now().UnixNano())
	}
	if len(m.PayloadJSON) == 0 {
		m.PayloadJSON = []byte("{}")
	}
	if len(m.ResultJSON) == 0 {
		m.ResultJSON = []byte("{}")
	}
	const q = `INSERT INTO node_submissions (code, class_id, node_id, user_id, node_type, question_id,
		submit_type, payload_json, result_json, is_processing, is_passed, is_completed, error_count,
		revealed, feedback, reference_answer, audio_resource_id, duration, points_earned, submitted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
		RETURNING id`
	var id int64
	if err := tx.QueryRow(ctx, q,
		m.Code, m.ClassID, m.NodeID, m.UserID, m.NodeType, m.QuestionID,
		m.SubmitType, m.PayloadJSON, m.ResultJSON, m.IsProcessing, m.IsPassed, m.IsCompleted,
		m.ErrorCount, m.Revealed, m.Feedback, m.ReferenceAnswer, m.AudioResourceID,
		m.Duration, m.PointsEarned, m.SubmittedAt,
	).Scan(&id); err != nil {
		return 0, fmt.Errorf("node_submission create tx: %w", err)
	}
	return id, nil
}

// GetByID 按主键查询提交记录。
func (r *NodeSubmissionRepository) GetByID(ctx context.Context, id int64) (*model.NodeSubmission, error) {
	const q = `SELECT id, code, class_id, node_id, user_id, node_type, question_id, submit_type,
		payload_json, result_json, is_processing, is_passed, is_completed, error_count, revealed,
		feedback, reference_answer, audio_resource_id, duration, points_earned,
		submitted_at, created_at, updated_at
		FROM node_submissions WHERE id = $1`
	var s model.NodeSubmission
	if err := r.QueryRow(ctx, &s, q, id); err != nil {
		return nil, fmt.Errorf("node_submission get by id: %w", err)
	}
	return &s, nil
}

// GetByCode 按 code 查询提交记录。
func (r *NodeSubmissionRepository) GetByCode(ctx context.Context, code string) (*model.NodeSubmission, error) {
	const q = `SELECT id, code, class_id, node_id, user_id, node_type, question_id, submit_type,
		payload_json, result_json, is_processing, is_passed, is_completed, error_count, revealed,
		feedback, reference_answer, audio_resource_id, duration, points_earned,
		submitted_at, created_at, updated_at
		FROM node_submissions WHERE code = $1`
	var s model.NodeSubmission
	if err := r.QueryRow(ctx, &s, q, code); err != nil {
		return nil, fmt.Errorf("node_submission get by code: %w", err)
	}
	return &s, nil
}

// ListByUserNode 按用户+节点分页查询提交记录。
func (r *NodeSubmissionRepository) ListByUserNode(ctx context.Context, userID, nodeID int64, limit, offset int) ([]model.NodeSubmission, error) {
	const q = `SELECT id, code, class_id, node_id, user_id, node_type, question_id, submit_type,
		payload_json, result_json, is_processing, is_passed, is_completed, error_count, revealed,
		feedback, reference_answer, audio_resource_id, duration, points_earned,
		submitted_at, created_at, updated_at
		FROM node_submissions
		WHERE user_id = $1 AND node_id = $2
		ORDER BY submitted_at DESC LIMIT $3 OFFSET $4`
	var list []model.NodeSubmission
	if err := r.Query(ctx, &list, q, userID, nodeID, limit, offset); err != nil {
		return nil, fmt.Errorf("node_submission list by user node: %w", err)
	}
	return list, nil
}

// ListByNode 按节点分页查询提交记录。
func (r *NodeSubmissionRepository) ListByNode(ctx context.Context, nodeID int64, limit, offset int) ([]model.NodeSubmission, error) {
	const q = `SELECT id, code, class_id, node_id, user_id, node_type, question_id, submit_type,
		payload_json, result_json, is_processing, is_passed, is_completed, error_count, revealed,
		feedback, reference_answer, audio_resource_id, duration, points_earned,
		submitted_at, created_at, updated_at
		FROM node_submissions
		WHERE node_id = $1
		ORDER BY submitted_at DESC LIMIT $2 OFFSET $3`
	var list []model.NodeSubmission
	if err := r.Query(ctx, &list, q, nodeID, limit, offset); err != nil {
		return nil, fmt.Errorf("node_submission list by node: %w", err)
	}
	return list, nil
}

// LatestSubmittedAt 查询该用户该节点最近一条提交时间，用于计算本次提交耗时。
// 无历史记录时返回零值时间（nil 错误）。
func (r *NodeSubmissionRepository) LatestSubmittedAt(ctx context.Context, userID, nodeID int64) (time.Time, error) {
	const q = `SELECT submitted_at FROM node_submissions
		WHERE user_id = $1 AND node_id = $2
		ORDER BY submitted_at DESC LIMIT 1`
	var t time.Time
	if err := r.QueryRow(ctx, &t, q, userID, nodeID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return time.Time{}, nil
		}
		return time.Time{}, fmt.Errorf("node_submission latest submitted at: %w", err)
	}
	return t, nil
}

// Update 更新提交记录可变字段（主要用于异步任务完成回写）。
func (r *NodeSubmissionRepository) Update(ctx context.Context, m *model.NodeSubmission) error {
	const q = `UPDATE node_submissions SET result_json = $1, is_processing = $2, is_passed = $3,
		is_completed = $4, error_count = $5, revealed = $6, feedback = $7, reference_answer = $8,
		audio_resource_id = $9, duration = $10, points_earned = $11 WHERE id = $12`
	if err := r.Exec(ctx, q,
		m.ResultJSON, m.IsProcessing, m.IsPassed, m.IsCompleted,
		m.ErrorCount, m.Revealed, m.Feedback, m.ReferenceAnswer,
		m.AudioResourceID, m.Duration, m.PointsEarned, m.ID,
	); err != nil {
		return fmt.Errorf("node_submission update: %w", err)
	}
	return nil
}

// Delete 按主键删除提交记录。
func (r *NodeSubmissionRepository) Delete(ctx context.Context, id int64) error {
	const q = `DELETE FROM node_submissions WHERE id = $1`
	if err := r.Exec(ctx, q, id); err != nil {
		return fmt.Errorf("node_submission delete: %w", err)
	}
	return nil
}
