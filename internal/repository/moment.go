package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"zhonghuawenhua_backend/internal/model"
)

// MomentRepository 朋友圈动态数据访问。
type MomentRepository struct {
	*Base
}

// NewMomentRepository 创建 MomentRepository。
func NewMomentRepository(base *Base) *MomentRepository {
	return &MomentRepository{Base: base}
}

// Create 插入朋友圈动态，返回新生成的 id。
func (r *MomentRepository) Create(ctx context.Context, m *model.Moment) (int64, error) {
	const q = `INSERT INTO moments (class_id, user_id, source, content, objs_json, like_count, comment_count)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`
	var id int64
	err := r.QueryRow(ctx, &id, q,
		m.ClassID, m.UserID, m.Source, m.Content, m.ObjsJSON, m.LikeCount, m.CommentCount,
	)
	if err != nil {
		return 0, fmt.Errorf("moment create: %w", err)
	}
	return id, nil
}

// GetByID 按主键查询未删除动态。
func (r *MomentRepository) GetByID(ctx context.Context, id int64) (*model.Moment, error) {
	const q = `SELECT id, class_id, user_id, source, content, objs_json, like_count, comment_count,
		created_at, updated_at, deleted_at
		FROM moments WHERE id = $1 AND deleted_at IS NULL`
	var m model.Moment
	if err := r.QueryRow(ctx, &m, q, id); err != nil {
		return nil, fmt.Errorf("moment get by id: %w", err)
	}
	return &m, nil
}

// ListByClass 按班级查询未删除动态列表，按创建时间倒序。
func (r *MomentRepository) ListByClass(ctx context.Context, classID int64, limit, offset int) ([]model.Moment, error) {
	const q = `SELECT id, class_id, user_id, source, content, objs_json, like_count, comment_count,
		created_at, updated_at, deleted_at
		FROM moments
		WHERE class_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	var list []model.Moment
	if err := r.Query(ctx, &list, q, classID, limit, offset); err != nil {
		return nil, fmt.Errorf("moment list by class: %w", err)
	}
	return list, nil
}

// ListByUser 按用户查询未删除动态列表。
func (r *MomentRepository) ListByUser(ctx context.Context, userID int64, limit, offset int) ([]model.Moment, error) {
	const q = `SELECT id, class_id, user_id, source, content, objs_json, like_count, comment_count,
		created_at, updated_at, deleted_at
		FROM moments
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	var list []model.Moment
	if err := r.Query(ctx, &list, q, userID, limit, offset); err != nil {
		return nil, fmt.Errorf("moment list by user: %w", err)
	}
	return list, nil
}

// ListAllByClass 查询班级内全部未删除动态，按创建时间倒序（不分页）。
func (r *MomentRepository) ListAllByClass(ctx context.Context, classID int64) ([]model.Moment, error) {
	const q = `SELECT id, class_id, user_id, source, content, objs_json, like_count, comment_count,
		created_at, updated_at, deleted_at
		FROM moments
		WHERE class_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC`
	var list []model.Moment
	if err := r.Query(ctx, &list, q, classID); err != nil {
		return nil, fmt.Errorf("moment list all by class: %w", err)
	}
	return list, nil
}

// IncrLikeCount 点赞数 +1。
func (r *MomentRepository) IncrLikeCount(ctx context.Context, id int64) error {
	const q = `UPDATE moments SET like_count = like_count + 1 WHERE id = $1 AND deleted_at IS NULL`
	if err := r.Exec(ctx, q, id); err != nil {
		return fmt.Errorf("moment incr like count: %w", err)
	}
	return nil
}

// DecrLikeCount 点赞数 -1（不低于 0）。
func (r *MomentRepository) DecrLikeCount(ctx context.Context, id int64) error {
	const q = `UPDATE moments SET like_count = GREATEST(like_count - 1, 0) WHERE id = $1 AND deleted_at IS NULL`
	if err := r.Exec(ctx, q, id); err != nil {
		return fmt.Errorf("moment decr like count: %w", err)
	}
	return nil
}

// IncrCommentCount 评论数 +1。
func (r *MomentRepository) IncrCommentCount(ctx context.Context, id int64) error {
	const q = `UPDATE moments SET comment_count = comment_count + 1 WHERE id = $1 AND deleted_at IS NULL`
	if err := r.Exec(ctx, q, id); err != nil {
		return fmt.Errorf("moment incr comment count: %w", err)
	}
	return nil
}

// DecrCommentCount 评论数 -1（不低于 0）。
func (r *MomentRepository) DecrCommentCount(ctx context.Context, id int64) error {
	const q = `UPDATE moments SET comment_count = GREATEST(comment_count - 1, 0) WHERE id = $1 AND deleted_at IS NULL`
	if err := r.Exec(ctx, q, id); err != nil {
		return fmt.Errorf("moment decr comment count: %w", err)
	}
	return nil
}

// Delete 软删除动态。
func (r *MomentRepository) Delete(ctx context.Context, id int64) error {
	const q = `UPDATE moments SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`
	if err := r.Exec(ctx, q, id); err != nil {
		return fmt.Errorf("moment delete: %w", err)
	}
	return nil
}

// CountByUserInClass 统计用户在指定班级内发布的动态数量。
func (r *MomentRepository) CountByUserInClass(ctx context.Context, userID, classID int64) (int, error) {
	const q = `SELECT COUNT(*) FROM moments WHERE user_id = $1 AND class_id = $2 AND deleted_at IS NULL`
	var count int
	if err := r.QueryRow(ctx, &count, q, userID, classID); err != nil {
		return 0, fmt.Errorf("moment count by user in class: %w", err)
	}
	return count, nil
}

// SumLikeCountByUserInClass 统计用户在指定班级内全部动态收获的点赞总数（冗余计数求和）。
func (r *MomentRepository) SumLikeCountByUserInClass(ctx context.Context, userID, classID int64) (int, error) {
	const q = `SELECT COALESCE(SUM(like_count), 0) FROM moments WHERE user_id = $1 AND class_id = $2 AND deleted_at IS NULL`
	var count int
	if err := r.QueryRow(ctx, &count, q, userID, classID); err != nil {
		return 0, fmt.Errorf("moment sum like count by user in class: %w", err)
	}
	return count, nil
}

// SumCommentCountByUserInClass 统计用户在指定班级内全部动态收获的评论总数（冗余计数求和）。
func (r *MomentRepository) SumCommentCountByUserInClass(ctx context.Context, userID, classID int64) (int, error) {
	const q = `SELECT COALESCE(SUM(comment_count), 0) FROM moments WHERE user_id = $1 AND class_id = $2 AND deleted_at IS NULL`
	var count int
	if err := r.QueryRow(ctx, &count, q, userID, classID); err != nil {
		return 0, fmt.Errorf("moment sum comment count by user in class: %w", err)
	}
	return count, nil
}

// ListSourcesByClass 查询班级内全部非空 source 的去重列表，按首次出现顺序倒序。
func (r *MomentRepository) ListSourcesByClass(ctx context.Context, classID int64) ([]string, error) {
	const q = `SELECT DISTINCT source FROM moments WHERE class_id = $1 AND deleted_at IS NULL AND source IS NOT NULL AND source <> '' ORDER BY source DESC`
	var list []string
	if err := r.Query(ctx, &list, q, classID); err != nil {
		return nil, fmt.Errorf("moment list sources by class: %w", err)
	}
	return list, nil
}

// CreateWithImages 事务：插入动态并关联图片资源，返回动态 ID。
// resourceIDs 为已解析的 resources 表主键列表，按切片顺序写入 sort_order。
func (r *MomentRepository) CreateWithImages(ctx context.Context, m *model.Moment, resourceIDs []int64) (int64, error) {
	var momentID int64
	err := r.RunInTx(ctx, func(tx pgx.Tx) error {
		const insertMoment = `INSERT INTO moments (class_id, user_id, source, content, objs_json, like_count, comment_count)
			VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`
		if err := tx.QueryRow(ctx, insertMoment,
			m.ClassID, m.UserID, m.Source, m.Content, m.ObjsJSON, m.LikeCount, m.CommentCount,
		).Scan(&momentID); err != nil {
			return fmt.Errorf("moment tx create: %w", err)
		}
		for i, rid := range resourceIDs {
			const insertImg = `INSERT INTO moment_images (moment_id, resource_id, sort_order) VALUES ($1, $2, $3)`
			if _, err := tx.Exec(ctx, insertImg, momentID, rid, i); err != nil {
				return fmt.Errorf("moment_image tx create: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return momentID, nil
}

// MomentImageRepository 朋友圈图片数据访问。
type MomentImageRepository struct {
	*Base
}

// NewMomentImageRepository 创建 MomentImageRepository。
func NewMomentImageRepository(base *Base) *MomentImageRepository {
	return &MomentImageRepository{Base: base}
}

// Create 插入图片关联，返回新生成的 id。
func (r *MomentImageRepository) Create(ctx context.Context, m *model.MomentImage) (int64, error) {
	const q = `INSERT INTO moment_images (moment_id, resource_id, sort_order)
		VALUES ($1, $2, $3) RETURNING id`
	var id int64
	err := r.QueryRow(ctx, &id, q, m.MomentID, m.ResourceID, m.SortOrder)
	if err != nil {
		return 0, fmt.Errorf("moment_image create: %w", err)
	}
	return id, nil
}

// ListByMomentID 按动态 ID 查询图片列表，按 sort_order 排序。
func (r *MomentImageRepository) ListByMomentID(ctx context.Context, momentID int64) ([]model.MomentImage, error) {
	const q = `SELECT id, moment_id, resource_id, sort_order, created_at
		FROM moment_images WHERE moment_id = $1 ORDER BY sort_order ASC, id ASC`
	var list []model.MomentImage
	if err := r.Query(ctx, &list, q, momentID); err != nil {
		return nil, fmt.Errorf("moment_image list by moment: %w", err)
	}
	return list, nil
}

// ListImageURLsByMomentID 按动态 ID 查询图片 URL 列表（JOIN resources），按 sort_order 排序。
func (r *MomentImageRepository) ListImageURLsByMomentID(ctx context.Context, momentID int64) ([]string, error) {
	const q = `SELECT COALESCE(r.file_url, '')
		FROM moment_images mi
		JOIN resources r ON r.id = mi.resource_id AND r.deleted_at IS NULL
		WHERE mi.moment_id = $1
		ORDER BY mi.sort_order ASC, mi.id ASC`
	var list []string
	if err := r.Query(ctx, &list, q, momentID); err != nil {
		return nil, fmt.Errorf("moment_image list image urls by moment: %w", err)
	}
	return list, nil
}

// DeleteByMomentID 按动态 ID 删除全部图片。
func (r *MomentImageRepository) DeleteByMomentID(ctx context.Context, momentID int64) error {
	const q = `DELETE FROM moment_images WHERE moment_id = $1`
	if err := r.Exec(ctx, q, momentID); err != nil {
		return fmt.Errorf("moment_image delete by moment: %w", err)
	}
	return nil
}

// MomentLikeRepository 朋友圈点赞数据访问。
type MomentLikeRepository struct {
	*Base
}

// NewMomentLikeRepository 创建 MomentLikeRepository。
func NewMomentLikeRepository(base *Base) *MomentLikeRepository {
	return &MomentLikeRepository{Base: base}
}

// Create 插入点赞记录。UNIQUE(moment_id, user_id) 防止重复点赞。
func (r *MomentLikeRepository) Create(ctx context.Context, m *model.MomentLike) (int64, error) {
	const q = `INSERT INTO moment_likes (moment_id, user_id) VALUES ($1, $2) RETURNING id`
	var id int64
	err := r.QueryRow(ctx, &id, q, m.MomentID, m.UserID)
	if err != nil {
		return 0, fmt.Errorf("moment_like create: %w", err)
	}
	return id, nil
}

// GetByMomentUser 按动态 + 用户查询点赞记录（用于判断是否已点赞）。
func (r *MomentLikeRepository) GetByMomentUser(ctx context.Context, momentID, userID int64) (*model.MomentLike, error) {
	const q = `SELECT id, moment_id, user_id, created_at
		FROM moment_likes WHERE moment_id = $1 AND user_id = $2`
	var l model.MomentLike
	if err := r.QueryRow(ctx, &l, q, momentID, userID); err != nil {
		return nil, fmt.Errorf("moment_like get by moment user: %w", err)
	}
	return &l, nil
}

// Delete 按主键删除点赞（取消点赞）。
func (r *MomentLikeRepository) Delete(ctx context.Context, id int64) error {
	const q = `DELETE FROM moment_likes WHERE id = $1`
	if err := r.Exec(ctx, q, id); err != nil {
		return fmt.Errorf("moment_like delete: %w", err)
	}
	return nil
}

// DeleteByMomentUser 按动态 + 用户删除点赞（取消点赞）。
func (r *MomentLikeRepository) DeleteByMomentUser(ctx context.Context, momentID, userID int64) error {
	const q = `DELETE FROM moment_likes WHERE moment_id = $1 AND user_id = $2`
	if err := r.Exec(ctx, q, momentID, userID); err != nil {
		return fmt.Errorf("moment_like delete by moment user: %w", err)
	}
	return nil
}

// CountByMoment 按动态统计点赞数。
func (r *MomentLikeRepository) CountByMoment(ctx context.Context, momentID int64) (int, error) {
	const q = `SELECT COUNT(*) FROM moment_likes WHERE moment_id = $1`
	var count int
	if err := r.QueryRow(ctx, &count, q, momentID); err != nil {
		return 0, fmt.Errorf("moment_like count by moment: %w", err)
	}
	return count, nil
}

// ListByMoment 按动态 ID 查询点赞列表，按创建时间正序。
func (r *MomentLikeRepository) ListByMoment(ctx context.Context, momentID int64) ([]model.MomentLike, error) {
	const q = `SELECT id, moment_id, user_id, created_at
		FROM moment_likes WHERE moment_id = $1 ORDER BY created_at ASC`
	var list []model.MomentLike
	if err := r.Query(ctx, &list, q, momentID); err != nil {
		return nil, fmt.Errorf("moment_like list by moment: %w", err)
	}
	return list, nil
}

// CountByUserInClass 统计用户在指定班级内给出的点赞数（JOIN moments 限定班级）。
func (r *MomentLikeRepository) CountByUserInClass(ctx context.Context, userID, classID int64) (int, error) {
	const q = `SELECT COUNT(*) FROM moment_likes ml
		JOIN moments m ON m.id = ml.moment_id
		WHERE ml.user_id = $1 AND m.class_id = $2 AND m.deleted_at IS NULL`
	var count int
	if err := r.QueryRow(ctx, &count, q, userID, classID); err != nil {
		return 0, fmt.Errorf("moment_like count by user in class: %w", err)
	}
	return count, nil
}

// LikeMoment 事务：插入点赞记录并将动态点赞数 +1。
// 调用方应先通过 GetByMomentUser 预检查避免 UNIQUE 冲突。
func (r *MomentLikeRepository) LikeMoment(ctx context.Context, momentID, userID int64) error {
	return r.RunInTx(ctx, func(tx pgx.Tx) error {
		const insertQ = `INSERT INTO moment_likes (moment_id, user_id) VALUES ($1, $2)`
		if _, err := tx.Exec(ctx, insertQ, momentID, userID); err != nil {
			return fmt.Errorf("moment_like tx insert: %w", err)
		}
		const updateQ = `UPDATE moments SET like_count = like_count + 1 WHERE id = $1 AND deleted_at IS NULL`
		if _, err := tx.Exec(ctx, updateQ, momentID); err != nil {
			return fmt.Errorf("moment_like tx incr count: %w", err)
		}
		return nil
	})
}

// UnlikeMoment 事务：删除点赞记录并将动态点赞数 -1（不低于 0）。
// 调用方应先通过 GetByMomentUser 预检查确保记录存在。
func (r *MomentLikeRepository) UnlikeMoment(ctx context.Context, momentID, userID int64) error {
	return r.RunInTx(ctx, func(tx pgx.Tx) error {
		const deleteQ = `DELETE FROM moment_likes WHERE moment_id = $1 AND user_id = $2`
		if _, err := tx.Exec(ctx, deleteQ, momentID, userID); err != nil {
			return fmt.Errorf("moment_like tx delete: %w", err)
		}
		const updateQ = `UPDATE moments SET like_count = GREATEST(like_count - 1, 0) WHERE id = $1 AND deleted_at IS NULL`
		if _, err := tx.Exec(ctx, updateQ, momentID); err != nil {
			return fmt.Errorf("moment_like tx decr count: %w", err)
		}
		return nil
	})
}

// MomentCommentRepository 朋友圈评论数据访问。
type MomentCommentRepository struct {
	*Base
}

// NewMomentCommentRepository 创建 MomentCommentRepository。
func NewMomentCommentRepository(base *Base) *MomentCommentRepository {
	return &MomentCommentRepository{Base: base}
}

// Create 插入评论，返回新生成的 id。
func (r *MomentCommentRepository) Create(ctx context.Context, m *model.MomentComment) (int64, error) {
	const q = `INSERT INTO moment_comments (moment_id, user_id, content)
		VALUES ($1, $2, $3) RETURNING id`
	var id int64
	err := r.QueryRow(ctx, &id, q, m.MomentID, m.UserID, m.Content)
	if err != nil {
		return 0, fmt.Errorf("moment_comment create: %w", err)
	}
	return id, nil
}

// GetByID 按主键查询未删除评论。
func (r *MomentCommentRepository) GetByID(ctx context.Context, id int64) (*model.MomentComment, error) {
	const q = `SELECT id, moment_id, user_id, content, created_at, updated_at, deleted_at
		FROM moment_comments WHERE id = $1 AND deleted_at IS NULL`
	var c model.MomentComment
	if err := r.QueryRow(ctx, &c, q, id); err != nil {
		return nil, fmt.Errorf("moment_comment get by id: %w", err)
	}
	return &c, nil
}

// ListByMomentID 按动态 ID 查询未删除评论，按创建时间正序。
func (r *MomentCommentRepository) ListByMomentID(ctx context.Context, momentID int64) ([]model.MomentComment, error) {
	const q = `SELECT id, moment_id, user_id, content, created_at, updated_at, deleted_at
		FROM moment_comments WHERE moment_id = $1 AND deleted_at IS NULL
		ORDER BY created_at ASC`
	var list []model.MomentComment
	if err := r.Query(ctx, &list, q, momentID); err != nil {
		return nil, fmt.Errorf("moment_comment list by moment: %w", err)
	}
	return list, nil
}

// Delete 软删除评论。
func (r *MomentCommentRepository) Delete(ctx context.Context, id int64) error {
	const q = `UPDATE moment_comments SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`
	if err := r.Exec(ctx, q, id); err != nil {
		return fmt.Errorf("moment_comment delete: %w", err)
	}
	return nil
}

// CountByUserInClass 统计用户在指定班级内给出的评论数（JOIN moments 限定班级）。
func (r *MomentCommentRepository) CountByUserInClass(ctx context.Context, userID, classID int64) (int, error) {
	const q = `SELECT COUNT(*) FROM moment_comments mc
		JOIN moments m ON m.id = mc.moment_id
		WHERE mc.user_id = $1 AND m.class_id = $2 AND m.deleted_at IS NULL AND mc.deleted_at IS NULL`
	var count int
	if err := r.QueryRow(ctx, &count, q, userID, classID); err != nil {
		return 0, fmt.Errorf("moment_comment count by user in class: %w", err)
	}
	return count, nil
}

// CommentMoment 事务：插入评论并将动态评论数 +1，返回新评论 ID。
func (r *MomentCommentRepository) CommentMoment(ctx context.Context, m *model.MomentComment) (int64, error) {
	var commentID int64
	err := r.RunInTx(ctx, func(tx pgx.Tx) error {
		const insertQ = `INSERT INTO moment_comments (moment_id, user_id, content) VALUES ($1, $2, $3) RETURNING id`
		if err := tx.QueryRow(ctx, insertQ, m.MomentID, m.UserID, m.Content).Scan(&commentID); err != nil {
			return fmt.Errorf("moment_comment tx insert: %w", err)
		}
		const updateQ = `UPDATE moments SET comment_count = comment_count + 1 WHERE id = $1 AND deleted_at IS NULL`
		if _, err := tx.Exec(ctx, updateQ, m.MomentID); err != nil {
			return fmt.Errorf("moment_comment tx incr count: %w", err)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return commentID, nil
}

// DeleteCommentWithCount 事务：软删除评论并将对应动态评论数 -1（不低于 0）。
// 通过 JOIN moments 校验评论所属动态在指定班级内且归属于提交用户，返回 momentID。
// 评论不存在、非本人或班级不匹配时返回 pgx.ErrNoRows。
func (r *MomentCommentRepository) DeleteCommentWithCount(ctx context.Context, commentID, userID, classID int64) (int64, error) {
	var momentID int64
	err := r.RunInTx(ctx, func(tx pgx.Tx) error {
		const selectQ = `SELECT mc.moment_id FROM moment_comments mc
			JOIN moments m ON m.id = mc.moment_id
			WHERE mc.id = $1 AND mc.user_id = $2 AND m.class_id = $3
			AND mc.deleted_at IS NULL AND m.deleted_at IS NULL`
		if err := tx.QueryRow(ctx, selectQ, commentID, userID, classID).Scan(&momentID); err != nil {
			return fmt.Errorf("moment_comment tx select: %w", err)
		}
		const deleteQ = `UPDATE moment_comments SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`
		if _, err := tx.Exec(ctx, deleteQ, commentID); err != nil {
			return fmt.Errorf("moment_comment tx delete: %w", err)
		}
		const updateQ = `UPDATE moments SET comment_count = GREATEST(comment_count - 1, 0) WHERE id = $1 AND deleted_at IS NULL`
		if _, err := tx.Exec(ctx, updateQ, momentID); err != nil {
			return fmt.Errorf("moment_comment tx decr count: %w", err)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return momentID, nil
}
