package repository

import (
	"context"
	"fmt"

	"zhonghuawenhua_backend/internal/model"
)

// UserRepository 用户表数据访问。
type UserRepository struct {
	*Base
}

// NewUserRepository 创建 UserRepository。
func NewUserRepository(base *Base) *UserRepository {
	return &UserRepository{Base: base}
}

// Create 插入用户，返回新生成的 id。
func (r *UserRepository) Create(ctx context.Context, m *model.User) (int64, error) {
	const q = `INSERT INTO users (code, username, password_hash, role, real_name, avatar_url, gender, city,
		grade_name, class_name, status, last_login_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING id`
	var id int64
	err := r.QueryRow(ctx, &id, q,
		m.Code, m.Username, m.PasswordHash, m.Role,
		m.RealName, m.AvatarURL, m.Gender, m.City,
		m.GradeName, m.ClassName, m.Status, m.LastLoginAt,
	)
	if err != nil {
		return 0, fmt.Errorf("user create: %w", err)
	}
	return id, nil
}

// GetByID 按主键查询未删除用户。
func (r *UserRepository) GetByID(ctx context.Context, id int64) (*model.User, error) {
	const q = `SELECT id, code, username, password_hash, role, real_name, avatar_url, gender, city,
		grade_name, class_name, status, last_login_at, created_at, updated_at, deleted_at
		FROM users WHERE id = $1 AND deleted_at IS NULL`
	var u model.User
	if err := r.QueryRow(ctx, &u, q, id); err != nil {
		return nil, fmt.Errorf("user get by id: %w", err)
	}
	return &u, nil
}

// GetByCode 按 code 查询未删除用户。
func (r *UserRepository) GetByCode(ctx context.Context, code string) (*model.User, error) {
	const q = `SELECT id, code, username, password_hash, role, real_name, avatar_url, gender, city,
		grade_name, class_name, status, last_login_at, created_at, updated_at, deleted_at
		FROM users WHERE code = $1 AND deleted_at IS NULL`
	var u model.User
	if err := r.QueryRow(ctx, &u, q, code); err != nil {
		return nil, fmt.Errorf("user get by code: %w", err)
	}
	return &u, nil
}

// GetByUsername 按 username 查询未删除用户。
func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	const q = `SELECT id, code, username, password_hash, role, real_name, avatar_url, gender, city,
		grade_name, class_name, status, last_login_at, created_at, updated_at, deleted_at
		FROM users WHERE username = $1 AND deleted_at IS NULL`
	var u model.User
	if err := r.QueryRow(ctx, &u, q, username); err != nil {
		return nil, fmt.Errorf("user get by username: %w", err)
	}
	return &u, nil
}

// Update 更新用户可变字段。
func (r *UserRepository) Update(ctx context.Context, m *model.User) error {
	const q = `UPDATE users SET real_name = $1, avatar_url = $2, gender = $3, city = $4,
		grade_name = $5, class_name = $6, status = $7, last_login_at = $8, password_hash = $9
		WHERE id = $10 AND deleted_at IS NULL`
	if err := r.Exec(ctx, q,
		m.RealName, m.AvatarURL, m.Gender, m.City,
		m.GradeName, m.ClassName, m.Status, m.LastLoginAt, m.PasswordHash, m.ID,
	); err != nil {
		return fmt.Errorf("user update: %w", err)
	}
	return nil
}

// UpdateLastLogin 仅更新最后登录时间。
func (r *UserRepository) UpdateLastLogin(ctx context.Context, id int64) error {
	const q = `UPDATE users SET last_login_at = now() WHERE id = $1 AND deleted_at IS NULL`
	if err := r.Exec(ctx, q, id); err != nil {
		return fmt.Errorf("user update last login: %w", err)
	}
	return nil
}

// Delete 软删除用户。
func (r *UserRepository) Delete(ctx context.Context, id int64) error {
	const q = `UPDATE users SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`
	if err := r.Exec(ctx, q, id); err != nil {
		return fmt.Errorf("user delete: %w", err)
	}
	return nil
}

// ListByRole 按角色分页查询未删除用户。
func (r *UserRepository) ListByRole(ctx context.Context, role model.UserRole, limit, offset int) ([]model.User, error) {
	const q = `SELECT id, code, username, password_hash, role, real_name, avatar_url, gender, city, status,
		last_login_at, created_at, updated_at, deleted_at
		FROM users WHERE role = $1 AND deleted_at IS NULL
		ORDER BY id DESC LIMIT $2 OFFSET $3`
	var list []model.User
	if err := r.Query(ctx, &list, q, role, limit, offset); err != nil {
		return nil, fmt.Errorf("user list by role: %w", err)
	}
	return list, nil
}

// ClassMemberRepository 班级成员表数据访问。
type ClassMemberRepository struct {
	*Base
}

// NewClassMemberRepository 创建 ClassMemberRepository。
func NewClassMemberRepository(base *Base) *ClassMemberRepository {
	return &ClassMemberRepository{Base: base}
}

// Create 插入班级成员，返回新生成的 id。
func (r *ClassMemberRepository) Create(ctx context.Context, m *model.ClassMember) (int64, error) {
	const q = `INSERT INTO class_members (class_id, user_id, role) VALUES ($1, $2, $3) RETURNING id`
	var id int64
	err := r.QueryRow(ctx, &id, q, m.ClassID, m.UserID, m.Role)
	if err != nil {
		return 0, fmt.Errorf("class_member create: %w", err)
	}
	return id, nil
}

// GetByID 按主键查询班级成员。
func (r *ClassMemberRepository) GetByID(ctx context.Context, id int64) (*model.ClassMember, error) {
	const q = `SELECT id, class_id, user_id, role, joined_at, created_at, updated_at
		FROM class_members WHERE id = $1`
	var m model.ClassMember
	if err := r.QueryRow(ctx, &m, q, id); err != nil {
		return nil, fmt.Errorf("class_member get by id: %w", err)
	}
	return &m, nil
}

// GetByClassAndUser 按班级 ID 和用户 ID 查询成员记录。
func (r *ClassMemberRepository) GetByClassAndUser(ctx context.Context, classID, userID int64) (*model.ClassMember, error) {
	const q = `SELECT id, class_id, user_id, role, joined_at, created_at, updated_at
		FROM class_members WHERE class_id = $1 AND user_id = $2`
	var m model.ClassMember
	if err := r.QueryRow(ctx, &m, q, classID, userID); err != nil {
		return nil, fmt.Errorf("class_member get by class and user: %w", err)
	}
	return &m, nil
}

// ListByClassID 查询指定班级的全部成员。
func (r *ClassMemberRepository) ListByClassID(ctx context.Context, classID int64) ([]model.ClassMember, error) {
	const q = `SELECT id, class_id, user_id, role, joined_at, created_at, updated_at
		FROM class_members WHERE class_id = $1 ORDER BY id ASC`
	var list []model.ClassMember
	if err := r.Query(ctx, &list, q, classID); err != nil {
		return nil, fmt.Errorf("class_member list by class: %w", err)
	}
	return list, nil
}

// ListByUserID 查询用户加入的所有班级成员记录。
func (r *ClassMemberRepository) ListByUserID(ctx context.Context, userID int64) ([]model.ClassMember, error) {
	const q = `SELECT id, class_id, user_id, role, joined_at, created_at, updated_at
		FROM class_members WHERE user_id = $1 ORDER BY id ASC`
	var list []model.ClassMember
	if err := r.Query(ctx, &list, q, userID); err != nil {
		return nil, fmt.Errorf("class_member list by user: %w", err)
	}
	return list, nil
}

// Delete 按主键硬删班级成员。
func (r *ClassMemberRepository) Delete(ctx context.Context, id int64) error {
	const q = `DELETE FROM class_members WHERE id = $1`
	if err := r.Exec(ctx, q, id); err != nil {
		return fmt.Errorf("class_member delete: %w", err)
	}
	return nil
}

// DeleteByClassAndUser 按班级 ID 和用户 ID 硬删成员记录。
func (r *ClassMemberRepository) DeleteByClassAndUser(ctx context.Context, classID, userID int64) error {
	const q = `DELETE FROM class_members WHERE class_id = $1 AND user_id = $2`
	if err := r.Exec(ctx, q, classID, userID); err != nil {
		return fmt.Errorf("class_member delete by class and user: %w", err)
	}
	return nil
}
