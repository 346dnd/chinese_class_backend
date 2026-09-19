package model

import (
	"encoding/json"
	"time"
)

// Moment 朋友圈动态表行（对应 moments 表）。
// 取代旧 work_publishes；objs_json 存 MomentExtraObj 数组。
type Moment struct {
	ID           int64           `db:"id" json:"id"`
	ClassID      int64           `db:"class_id" json:"classId"`
	UserID       int64           `db:"user_id" json:"userId"`
	Source       *string         `db:"source" json:"source,omitempty"`
	Content      string          `db:"content" json:"content"`
	ObjsJSON     json.RawMessage `db:"objs_json" json:"objsJson"`
	LikeCount    int             `db:"like_count" json:"likeCount"`
	CommentCount int             `db:"comment_count" json:"commentCount"`
	CreatedAt    time.Time       `db:"created_at" json:"createdAt"`
	UpdatedAt    time.Time       `db:"updated_at" json:"updatedAt"`
	DeletedAt    *time.Time      `db:"deleted_at" json:"deletedAt,omitempty"`
}

// MomentImage 朋友圈图片表行（对应 moment_images 表）。
type MomentImage struct {
	ID         int64     `db:"id" json:"id"`
	MomentID   int64     `db:"moment_id" json:"momentId"`
	ResourceID int64     `db:"resource_id" json:"resourceId"`
	SortOrder  int       `db:"sort_order" json:"sortOrder"`
	CreatedAt  time.Time `db:"created_at" json:"createdAt"`
}

// MomentLike 朋友圈点赞表行（对应 moment_likes 表）。
// UNIQUE(moment_id, user_id) 防止重复点赞。
type MomentLike struct {
	ID        int64     `db:"id" json:"id"`
	MomentID  int64     `db:"moment_id" json:"momentId"`
	UserID    int64     `db:"user_id" json:"userId"`
	CreatedAt time.Time `db:"created_at" json:"createdAt"`
}

// MomentComment 朋友圈评论表行（对应 moment_comments 表）。
type MomentComment struct {
	ID        int64      `db:"id" json:"id"`
	MomentID  int64      `db:"moment_id" json:"momentId"`
	UserID    int64      `db:"user_id" json:"userId"`
	Content   string     `db:"content" json:"content"`
	CreatedAt time.Time  `db:"created_at" json:"createdAt"`
	UpdatedAt time.Time  `db:"updated_at" json:"updatedAt"`
	DeletedAt *time.Time `db:"deleted_at" json:"deletedAt,omitempty"`
}
