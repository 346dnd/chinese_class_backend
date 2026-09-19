package model

import "time"

// User 用户表行（对应 users 表）。
type User struct {
	ID           int64      `db:"id" json:"id"`
	Code         string     `db:"code" json:"code"`
	Username     string     `db:"username" json:"username"`
	PasswordHash string     `db:"password_hash" json:"-"`
	Role         UserRole   `db:"role" json:"role"`
	RealName     *string    `db:"real_name" json:"realName,omitempty"`
	AvatarURL    *string    `db:"avatar_url" json:"avatarUrl,omitempty"`
	Gender       *string    `db:"gender" json:"gender,omitempty"`
	City         *string    `db:"city" json:"city,omitempty"`
	GradeName    *string    `db:"grade_name" json:"gradeName,omitempty"`
	ClassName    *string    `db:"class_name" json:"className,omitempty"`
	Status       int16      `db:"status" json:"status"`
	LastLoginAt  *time.Time `db:"last_login_at" json:"lastLoginAt,omitempty"`
	CreatedAt    time.Time  `db:"created_at" json:"createdAt"`
	UpdatedAt    time.Time  `db:"updated_at" json:"updatedAt"`
	DeletedAt    *time.Time `db:"deleted_at" json:"deletedAt,omitempty"`
}

// ClassMember 班级成员表行（对应 class_members 表）。
type ClassMember struct {
	ID        int64     `db:"id" json:"id"`
	ClassID   int64     `db:"class_id" json:"classId"`
	UserID    int64     `db:"user_id" json:"userId"`
	Role      UserRole   `db:"role" json:"role"`
	JoinedAt  time.Time `db:"joined_at" json:"joinedAt"`
	CreatedAt time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt time.Time `db:"updated_at" json:"updatedAt"`
}
