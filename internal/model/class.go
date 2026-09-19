package model

import (
	"encoding/json"
	"time"
)

// Class 班级表行（对应 classes 表）。
type Class struct {
	ID          int64      `db:"id" json:"id"`
	Code        string     `db:"code" json:"code"`
	Name        string     `db:"name" json:"name"`
	Description *string    `db:"description" json:"description,omitempty"`
	TeacherID   *int64     `db:"teacher_id" json:"teacherId,omitempty"`
	Scene       SceneType  `db:"scene" json:"scene"`
	StartAt     *time.Time `db:"start_at" json:"startAt,omitempty"`
	EndAt       *time.Time `db:"end_at" json:"endAt,omitempty"`
	Status      int16      `db:"status" json:"status"`
	CreatedAt   time.Time  `db:"created_at" json:"createdAt"`
	UpdatedAt   time.Time  `db:"updated_at" json:"updatedAt"`
	DeletedAt   *time.Time `db:"deleted_at" json:"deletedAt,omitempty"`
}

// ClassNode 课程节点表行（对应 class_nodes 表，parent_id 自引用形成树形结构）。
type ClassNode struct {
	ID          int64      `db:"id" json:"id"`
	Code        *string    `db:"code" json:"code,omitempty"`
	ClassID     int64      `db:"class_id" json:"classId"`
	ParentID    *int64     `db:"parent_id" json:"parentId,omitempty"`
	NodeType    NodeType   `db:"node_type" json:"nodeType"`
	Title       string     `db:"title" json:"title"`
	Description *string    `db:"description" json:"description,omitempty"`
	SortOrder   int        `db:"sort_order" json:"sortOrder"`
	HasChildren bool       `db:"has_children" json:"hasChildren"`
	Scene       *SceneType `db:"scene" json:"scene,omitempty"`
	CreatedAt   time.Time  `db:"created_at" json:"createdAt"`
	UpdatedAt   time.Time  `db:"updated_at" json:"updatedAt"`
	DeletedAt   *time.Time `db:"deleted_at" json:"deletedAt,omitempty"`
}

// NodeContent 节点 params 配置表行（对应 node_contents 表，与 ClassNode 1:1）。
// 新版 schema 字段名为 params_json（旧版 content_json 已废弃）。
type NodeContent struct {
	ID         int64           `db:"id" json:"id"`
	NodeID     int64           `db:"node_id" json:"nodeId"`
	Version    int             `db:"version" json:"version"`
	ParamsJSON json.RawMessage `db:"params_json" json:"paramsJson"`
	CreatedAt  time.Time       `db:"created_at" json:"createdAt"`
	UpdatedAt  time.Time       `db:"updated_at" json:"updatedAt"`
}

// NodeStatusItem 节点状态聚合视图，用于业务层组装节点列表展示。
// 该结构体不直接对应单一表，聚合自 class_nodes、node_progress 等信息。
type NodeStatusItem struct {
	NodeID      int64   `db:"node_id" json:"nodeId"`
	NodeType    NodeType `db:"node_type" json:"nodeType"`
	Title       string  `db:"title" json:"title"`
	Completed   bool    `db:"completed" json:"completed"`
	AttemptCount int    `db:"attempt_count" json:"attemptCount"`
	ErrorCount  int     `db:"error_count" json:"errorCount"`
	Revealed    bool    `db:"revealed" json:"revealed"`
}
