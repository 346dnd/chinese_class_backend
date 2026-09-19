package repository

import (
	"context"
	"fmt"

	"zhonghuawenhua_backend/internal/model"
)

// ClassRepository 班级表数据访问。
type ClassRepository struct {
	*Base
}

// NewClassRepository 创建 ClassRepository。
func NewClassRepository(base *Base) *ClassRepository {
	return &ClassRepository{Base: base}
}

// Create 插入班级，返回新生成的 id。
func (r *ClassRepository) Create(ctx context.Context, m *model.Class) (int64, error) {
	const q = `INSERT INTO classes (code, name, description, teacher_id, scene, start_at, end_at, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`
	var id int64
	err := r.QueryRow(ctx, &id, q,
		m.Code, m.Name, m.Description, m.TeacherID, m.Scene, m.StartAt, m.EndAt, m.Status,
	)
	if err != nil {
		return 0, fmt.Errorf("class create: %w", err)
	}
	return id, nil
}

// GetByID 按主键查询未删除班级。
func (r *ClassRepository) GetByID(ctx context.Context, id int64) (*model.Class, error) {
	const q = `SELECT id, code, name, description, teacher_id, scene, start_at, end_at, status,
		created_at, updated_at, deleted_at
		FROM classes WHERE id = $1 AND deleted_at IS NULL`
	var c model.Class
	if err := r.QueryRow(ctx, &c, q, id); err != nil {
		return nil, fmt.Errorf("class get by id: %w", err)
	}
	return &c, nil
}

// GetByCode 按 code 查询未删除班级。
func (r *ClassRepository) GetByCode(ctx context.Context, code string) (*model.Class, error) {
	const q = `SELECT id, code, name, description, teacher_id, scene, start_at, end_at, status,
		created_at, updated_at, deleted_at
		FROM classes WHERE code = $1 AND deleted_at IS NULL`
	var c model.Class
	if err := r.QueryRow(ctx, &c, q, code); err != nil {
		return nil, fmt.Errorf("class get by code: %w", err)
	}
	return &c, nil
}

// Update 更新班级可变字段。
func (r *ClassRepository) Update(ctx context.Context, m *model.Class) error {
	const q = `UPDATE classes SET name = $1, description = $2, teacher_id = $3,
		scene = $4, start_at = $5, end_at = $6, status = $7
		WHERE id = $8 AND deleted_at IS NULL`
	if err := r.Exec(ctx, q,
		m.Name, m.Description, m.TeacherID, m.Scene, m.StartAt, m.EndAt, m.Status, m.ID,
	); err != nil {
		return fmt.Errorf("class update: %w", err)
	}
	return nil
}

// Delete 软删除班级。
func (r *ClassRepository) Delete(ctx context.Context, id int64) error {
	const q = `UPDATE classes SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`
	if err := r.Exec(ctx, q, id); err != nil {
		return fmt.Errorf("class delete: %w", err)
	}
	return nil
}

// ListByTeacherID 查询教师创建的班级列表。
func (r *ClassRepository) ListByTeacherID(ctx context.Context, teacherID int64) ([]model.Class, error) {
	const q = `SELECT id, code, name, description, teacher_id, scene, start_at, end_at, status,
		created_at, updated_at, deleted_at
		FROM classes WHERE teacher_id = $1 AND deleted_at IS NULL ORDER BY id DESC`
	var list []model.Class
	if err := r.Query(ctx, &list, q, teacherID); err != nil {
		return nil, fmt.Errorf("class list by teacher: %w", err)
	}
	return list, nil
}

// ClassNodeRepository 课程节点表数据访问。
type ClassNodeRepository struct {
	*Base
}

// NewClassNodeRepository 创建 ClassNodeRepository。
func NewClassNodeRepository(base *Base) *ClassNodeRepository {
	return &ClassNodeRepository{Base: base}
}

// Create 插入节点，返回新生成的 id。
func (r *ClassNodeRepository) Create(ctx context.Context, m *model.ClassNode) (int64, error) {
	const q = `INSERT INTO class_nodes (code, class_id, parent_id, node_type, title, description,
		sort_order, has_children, scene)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`
	var id int64
	err := r.QueryRow(ctx, &id, q,
		m.Code, m.ClassID, m.ParentID, m.NodeType, m.Title, m.Description,
		m.SortOrder, m.HasChildren, m.Scene,
	)
	if err != nil {
		return 0, fmt.Errorf("class_node create: %w", err)
	}
	return id, nil
}

// GetByID 按主键查询未删除节点。
func (r *ClassNodeRepository) GetByID(ctx context.Context, id int64) (*model.ClassNode, error) {
	const q = `SELECT id, code, class_id, parent_id, node_type, title, description,
		sort_order, has_children, scene, created_at, updated_at, deleted_at
		FROM class_nodes WHERE id = $1 AND deleted_at IS NULL`
	var n model.ClassNode
	if err := r.QueryRow(ctx, &n, q, id); err != nil {
		return nil, fmt.Errorf("class_node get by id: %w", err)
	}
	return &n, nil
}

// GetByCode 按 code 查询未删除节点。
func (r *ClassNodeRepository) GetByCode(ctx context.Context, code string) (*model.ClassNode, error) {
	const q = `SELECT id, code, class_id, parent_id, node_type, title, description,
		sort_order, has_children, scene, created_at, updated_at, deleted_at
		FROM class_nodes WHERE code = $1 AND deleted_at IS NULL`
	var n model.ClassNode
	if err := r.QueryRow(ctx, &n, q, code); err != nil {
		return nil, fmt.Errorf("class_node get by code: %w", err)
	}
	return &n, nil
}

// ListByClassID 查询指定班级下的全部节点。
func (r *ClassNodeRepository) ListByClassID(ctx context.Context, classID int64) ([]model.ClassNode, error) {
	const q = `SELECT id, code, class_id, parent_id, node_type, title, description,
		sort_order, has_children, scene, created_at, updated_at, deleted_at
		FROM class_nodes WHERE class_id = $1 AND deleted_at IS NULL
		ORDER BY parent_id NULLS FIRST, sort_order ASC`
	var list []model.ClassNode
	if err := r.Query(ctx, &list, q, classID); err != nil {
		return nil, fmt.Errorf("class_node list by class: %w", err)
	}
	return list, nil
}

// ListByParentID 查询指定父节点下的子节点。
func (r *ClassNodeRepository) ListByParentID(ctx context.Context, parentID int64) ([]model.ClassNode, error) {
	const q = `SELECT id, code, class_id, parent_id, node_type, title, description,
		sort_order, has_children, scene, created_at, updated_at, deleted_at
		FROM class_nodes WHERE parent_id = $1 AND deleted_at IS NULL
		ORDER BY sort_order ASC`
	var list []model.ClassNode
	if err := r.Query(ctx, &list, q, parentID); err != nil {
		return nil, fmt.Errorf("class_node list by parent: %w", err)
	}
	return list, nil
}

// Update 更新节点可变字段。
func (r *ClassNodeRepository) Update(ctx context.Context, m *model.ClassNode) error {
	const q = `UPDATE class_nodes SET code = $1, parent_id = $2, node_type = $3, title = $4,
		description = $5, sort_order = $6, has_children = $7, scene = $8
		WHERE id = $9 AND deleted_at IS NULL`
	if err := r.Exec(ctx, q,
		m.Code, m.ParentID, m.NodeType, m.Title, m.Description,
		m.SortOrder, m.HasChildren, m.Scene, m.ID,
	); err != nil {
		return fmt.Errorf("class_node update: %w", err)
	}
	return nil
}

// Delete 软删除节点。
func (r *ClassNodeRepository) Delete(ctx context.Context, id int64) error {
	const q = `UPDATE class_nodes SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`
	if err := r.Exec(ctx, q, id); err != nil {
		return fmt.Errorf("class_node delete: %w", err)
	}
	return nil
}

// NodeContentRepository 节点内容表数据访问（1:1 关联 ClassNode）。
type NodeContentRepository struct {
	*Base
}

// NewNodeContentRepository 创建 NodeContentRepository。
func NewNodeContentRepository(base *Base) *NodeContentRepository {
	return &NodeContentRepository{Base: base}
}

// Create 插入节点内容，返回新生成的 id。
func (r *NodeContentRepository) Create(ctx context.Context, m *model.NodeContent) (int64, error) {
	const q = `INSERT INTO node_contents (node_id, version, params_json)
		VALUES ($1, $2, $3) RETURNING id`
	var id int64
	err := r.QueryRow(ctx, &id, q, m.NodeID, m.Version, m.ParamsJSON)
	if err != nil {
		return 0, fmt.Errorf("node_content create: %w", err)
	}
	return id, nil
}

// GetByNodeID 按节点 ID 查询内容。
func (r *NodeContentRepository) GetByNodeID(ctx context.Context, nodeID int64) (*model.NodeContent, error) {
	const q = `SELECT id, node_id, version, params_json, created_at, updated_at
		FROM node_contents WHERE node_id = $1`
	var c model.NodeContent
	if err := r.QueryRow(ctx, &c, q, nodeID); err != nil {
		return nil, fmt.Errorf("node_content get by node: %w", err)
	}
	return &c, nil
}

// Update 更新节点内容与版本号。
func (r *NodeContentRepository) Update(ctx context.Context, m *model.NodeContent) error {
	const q = `UPDATE node_contents SET params_json = $1, version = $2 WHERE node_id = $3`
	if err := r.Exec(ctx, q, m.ParamsJSON, m.Version, m.NodeID); err != nil {
		return fmt.Errorf("node_content update: %w", err)
	}
	return nil
}

// DeleteByNodeID 按节点 ID 删除内容。
func (r *NodeContentRepository) DeleteByNodeID(ctx context.Context, nodeID int64) error {
	const q = `DELETE FROM node_contents WHERE node_id = $1`
	if err := r.Exec(ctx, q, nodeID); err != nil {
		return fmt.Errorf("node_content delete by node: %w", err)
	}
	return nil
}
