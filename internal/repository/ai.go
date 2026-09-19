package repository

import (
	"context"
	"fmt"
	"time"

	"zhonghuawenhua_backend/internal/model"
)

// AISessionRepository AI 会话数据访问。
type AISessionRepository struct {
	*Base
}

// NewAISessionRepository 创建 AISessionRepository。
func NewAISessionRepository(base *Base) *AISessionRepository {
	return &AISessionRepository{Base: base}
}

// Create 插入 AI 会话，返回新生成的 id。
// code 未设置时自动生成唯一值（对外会话标识，唯一约束保证）。
func (r *AISessionRepository) Create(ctx context.Context, m *model.AISession) (int64, error) {
	if m.Code == "" {
		m.Code = fmt.Sprintf("sess%d", time.Now().UnixNano())
	}
	const q = `INSERT INTO ai_sessions (code, user_id, class_id, node_id, biz_type, title,
		prompt_template, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`
	var id int64
	err := r.QueryRow(ctx, &id, q,
		m.Code, m.UserID, m.ClassID, m.NodeID, m.BizType,
		m.Title, m.PromptTemplate, m.Status,
	)
	if err != nil {
		return 0, fmt.Errorf("ai_session create: %w", err)
	}
	return id, nil
}

// GetByID 按主键查询会话。
func (r *AISessionRepository) GetByID(ctx context.Context, id int64) (*model.AISession, error) {
	const q = `SELECT id, code, user_id, class_id, node_id, biz_type, title, prompt_template,
		status, created_at, updated_at
		FROM ai_sessions WHERE id = $1`
	var s model.AISession
	if err := r.QueryRow(ctx, &s, q, id); err != nil {
		return nil, fmt.Errorf("ai_session get by id: %w", err)
	}
	return &s, nil
}

// GetByCode 按 code 查询会话。
func (r *AISessionRepository) GetByCode(ctx context.Context, code string) (*model.AISession, error) {
	const q = `SELECT id, code, user_id, class_id, node_id, biz_type, title, prompt_template,
		status, created_at, updated_at
		FROM ai_sessions WHERE code = $1`
	var s model.AISession
	if err := r.QueryRow(ctx, &s, q, code); err != nil {
		return nil, fmt.Errorf("ai_session get by code: %w", err)
	}
	return &s, nil
}

// ListByUserID 按用户分页查询会话。
func (r *AISessionRepository) ListByUserID(ctx context.Context, userID int64, limit, offset int) ([]model.AISession, error) {
	const q = `SELECT id, code, user_id, class_id, node_id, biz_type, title, prompt_template,
		status, created_at, updated_at
		FROM ai_sessions WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	var list []model.AISession
	if err := r.Query(ctx, &list, q, userID, limit, offset); err != nil {
		return nil, fmt.Errorf("ai_session list by user: %w", err)
	}
	return list, nil
}

// ListByContext 按班级+节点上下文查询会话。
func (r *AISessionRepository) ListByContext(ctx context.Context, classID, nodeID int64) ([]model.AISession, error) {
	const q = `SELECT id, code, user_id, class_id, node_id, biz_type, title, prompt_template,
		status, created_at, updated_at
		FROM ai_sessions WHERE class_id = $1 AND node_id = $2 ORDER BY created_at DESC`
	var list []model.AISession
	if err := r.Query(ctx, &list, q, classID, nodeID); err != nil {
		return nil, fmt.Errorf("ai_session list by context: %w", err)
	}
	return list, nil
}

// Update 更新会话可变字段。
func (r *AISessionRepository) Update(ctx context.Context, m *model.AISession) error {
	const q = `UPDATE ai_sessions SET user_id = $1, class_id = $2, node_id = $3, biz_type = $4,
		title = $5, prompt_template = $6, status = $7 WHERE id = $8`
	if err := r.Exec(ctx, q,
		m.UserID, m.ClassID, m.NodeID, m.BizType,
		m.Title, m.PromptTemplate, m.Status, m.ID,
	); err != nil {
		return fmt.Errorf("ai_session update: %w", err)
	}
	return nil
}

// Close 关闭会话（status = 'closed'）。
func (r *AISessionRepository) Close(ctx context.Context, id int64) error {
	const q = `UPDATE ai_sessions SET status = 'closed' WHERE id = $1`
	if err := r.Exec(ctx, q, id); err != nil {
		return fmt.Errorf("ai_session close: %w", err)
	}
	return nil
}

// AIMessageRepository AI 消息数据访问。
type AIMessageRepository struct {
	*Base
}

// NewAIMessageRepository 创建 AIMessageRepository。
func NewAIMessageRepository(base *Base) *AIMessageRepository {
	return &AIMessageRepository{Base: base}
}

// Create 插入消息，返回新生成的 id。
func (r *AIMessageRepository) Create(ctx context.Context, m *model.AIMessage) (int64, error) {
	const q = `INSERT INTO ai_messages (session_id, role, content, finish_reason, prompt_tokens,
		completion_tokens, total_tokens, model_name, seq)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`
	var id int64
	err := r.QueryRow(ctx, &id, q,
		m.SessionID, m.Role, m.Content, m.FinishReason,
		m.PromptTokens, m.CompletionTokens, m.TotalTokens, m.ModelName, m.Seq,
	)
	if err != nil {
		return 0, fmt.Errorf("ai_message create: %w", err)
	}
	return id, nil
}

// GetByID 按主键查询消息。
func (r *AIMessageRepository) GetByID(ctx context.Context, id int64) (*model.AIMessage, error) {
	const q = `SELECT id, session_id, role, content, finish_reason, prompt_tokens,
		completion_tokens, total_tokens, model_name, seq, created_at
		FROM ai_messages WHERE id = $1`
	var m model.AIMessage
	if err := r.QueryRow(ctx, &m, q, id); err != nil {
		return nil, fmt.Errorf("ai_message get by id: %w", err)
	}
	return &m, nil
}

// ListBySessionID 按会话 ID 分页查询消息，按 seq 升序。
func (r *AIMessageRepository) ListBySessionID(ctx context.Context, sessionID int64, limit, offset int) ([]model.AIMessage, error) {
	const q = `SELECT id, session_id, role, content, finish_reason, prompt_tokens,
		completion_tokens, total_tokens, model_name, seq, created_at
		FROM ai_messages WHERE session_id = $1 ORDER BY seq ASC LIMIT $2 OFFSET $3`
	var list []model.AIMessage
	if err := r.Query(ctx, &list, q, sessionID, limit, offset); err != nil {
		return nil, fmt.Errorf("ai_message list by session: %w", err)
	}
	return list, nil
}

// ListBySessionIDWithRefs 查询会话全部消息（按 seq 升序）。
// 注意：model.AIMessage 不含 refs 字段，引用数据需调用方通过
// AISearchReferenceRepository.ListByMessageIDs 批量获取后按 messageID 关联。
func (r *AIMessageRepository) ListBySessionIDWithRefs(ctx context.Context, sessionID int64) ([]model.AIMessage, error) {
	const q = `SELECT id, session_id, role, content, finish_reason, prompt_tokens,
		completion_tokens, total_tokens, model_name, seq, created_at
		FROM ai_messages WHERE session_id = $1 ORDER BY seq ASC`
	var list []model.AIMessage
	if err := r.Query(ctx, &list, q, sessionID); err != nil {
		return nil, fmt.Errorf("ai_message list by session with refs: %w", err)
	}
	return list, nil
}

// DeleteBySessionID 按会话 ID 删除全部消息。
func (r *AIMessageRepository) DeleteBySessionID(ctx context.Context, sessionID int64) error {
	const q = `DELETE FROM ai_messages WHERE session_id = $1`
	if err := r.Exec(ctx, q, sessionID); err != nil {
		return fmt.Errorf("ai_message delete by session: %w", err)
	}
	return nil
}

// AISearchReferenceRepository AI 搜索引用数据访问。
type AISearchReferenceRepository struct {
	*Base
}

// NewAISearchReferenceRepository 创建 AISearchReferenceRepository。
func NewAISearchReferenceRepository(base *Base) *AISearchReferenceRepository {
	return &AISearchReferenceRepository{Base: base}
}

// Create 插入引用记录，返回新生成的 id。
func (r *AISearchReferenceRepository) Create(ctx context.Context, m *model.AISearchReference) (int64, error) {
	const q = `INSERT INTO ai_search_references (message_id, ref_index, title, link, snippet)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`
	var id int64
	err := r.QueryRow(ctx, &id, q, m.MessageID, m.RefIndex, m.Title, m.Link, m.Snippet)
	if err != nil {
		return 0, fmt.Errorf("ai_search_reference create: %w", err)
	}
	return id, nil
}

// ListByMessageID 按消息 ID 查询引用列表。
func (r *AISearchReferenceRepository) ListByMessageID(ctx context.Context, messageID int64) ([]model.AISearchReference, error) {
	const q = `SELECT id, message_id, ref_index, title, link, snippet, created_at
		FROM ai_search_references WHERE message_id = $1 ORDER BY ref_index ASC`
	var list []model.AISearchReference
	if err := r.Query(ctx, &list, q, messageID); err != nil {
		return nil, fmt.Errorf("ai_search_reference list by message: %w", err)
	}
	return list, nil
}

// ListByMessageIDs 批量查询多条消息的引用，按 messageID 分组返回。
// 空入参时直接返回空 map，避免生成无意义的 SQL。
func (r *AISearchReferenceRepository) ListByMessageIDs(ctx context.Context, messageIDs []int64) (map[int64][]model.AISearchReference, error) {
	result := make(map[int64][]model.AISearchReference)
	if len(messageIDs) == 0 {
		return result, nil
	}
	const q = `SELECT id, message_id, ref_index, title, link, snippet, created_at
		FROM ai_search_references WHERE message_id = ANY($1) ORDER BY message_id, ref_index`
	var list []model.AISearchReference
	if err := r.Query(ctx, &list, q, messageIDs); err != nil {
		return nil, fmt.Errorf("ai_search_reference list by message ids: %w", err)
	}
	for i := range list {
		result[list[i].MessageID] = append(result[list[i].MessageID], list[i])
	}
	return result, nil
}

// DeleteByMessageID 按消息 ID 删除全部引用。
func (r *AISearchReferenceRepository) DeleteByMessageID(ctx context.Context, messageID int64) error {
	const q = `DELETE FROM ai_search_references WHERE message_id = $1`
	if err := r.Exec(ctx, q, messageID); err != nil {
		return fmt.Errorf("ai_search_reference delete by message: %w", err)
	}
	return nil
}
