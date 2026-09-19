// 朋友圈业务：列表、详情、发布、点赞、评论、互动统计、来源筛选。
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"zhonghuawenhua_backend/internal/api"
	"zhonghuawenhua_backend/internal/model"
)

// 以下类型别名与 api.Moment 中匿名结构体字段完全一致，便于构造切片。
type momentCommentItem = struct {
	Content string      `json:"content"`
	ID      int         `json:"id"`
	User    interface{} `json:"user"`
}

type momentLikeItem = struct {
	User api.UserBaseInfo `json:"user"`
}

type momentImageItem = struct {
	URL string `json:"url"`
}

// MomentService 朋友圈业务逻辑，编排 moment / moment_like / moment_comment / moment_image / user / resource。
type MomentService struct {
	repos *Repositories
}

// NewMomentService 创建 MomentService。
func NewMomentService(repos *Repositories) *MomentService {
	return &MomentService{repos: repos}
}

// List 获取班级朋友圈列表，按创建时间倒序，聚合作者/图片/点赞/评论/是否已赞。
func (s *MomentService) List(ctx context.Context, classID, currentUserID int64) (*api.GetMomentsResp, error) {
	if err := s.repos.EnsureClassMember(ctx, classID, currentUserID); err != nil {
		return nil, err
	}
	moments, err := s.repos.Moment.ListAllByClass(ctx, classID)
	if err != nil {
		return nil, fmt.Errorf("list moments: %w", err)
	}
	items := make([]api.Moment, 0, len(moments))
	for i := range moments {
		dto, err := s.buildMomentDTO(ctx, &moments[i], classID, currentUserID)
		if err != nil {
			return nil, err
		}
		items = append(items, *dto)
	}
	return &api.GetMomentsResp{Moments: items}, nil
}

// Get 获取单条朋友圈动态详情。
func (s *MomentService) Get(ctx context.Context, classID, momentID, currentUserID int64) (*api.Moment, error) {
	if err := s.repos.EnsureClassMember(ctx, classID, currentUserID); err != nil {
		return nil, err
	}
	m, err := s.getOwnedMoment(ctx, classID, momentID)
	if err != nil {
		return nil, err
	}
	return s.buildMomentDTO(ctx, m, classID, currentUserID)
}

// Create 发布朋友圈；图片通过业务 resource_id 关联到 moment_images。
func (s *MomentService) Create(ctx context.Context, classID, userID int64, req api.PostMomentsReq) (*api.Moment, error) {
	if err := s.repos.EnsureClassMember(ctx, classID, userID); err != nil {
		return nil, err
	}
	if req.Content == "" {
		return nil, ErrMomentContentEmpty
	}
	resourceIDs, err := s.resolveImageResourceIDs(ctx, req.Images)
	if err != nil {
		return nil, err
	}
	m := &model.Moment{
		ClassID:  classID,
		UserID:   userID,
		Source:   req.Source,
		Content:  req.Content,
		ObjsJSON: []byte("[]"),
	}
	momentID, err := s.repos.Moment.CreateWithImages(ctx, m, resourceIDs)
	if err != nil {
		return nil, fmt.Errorf("create moment: %w", err)
	}
	return s.Get(ctx, classID, momentID, userID)
}

// Like 点赞动态；已点赞返回 ErrMomentAlreadyLiked。
func (s *MomentService) Like(ctx context.Context, classID, momentID, userID int64) (*api.Moment, error) {
	if err := s.repos.EnsureClassMember(ctx, classID, userID); err != nil {
		return nil, err
	}
	if !s.repos.MomentLock.TryAcquire(userID) {
		return nil, ErrMomentOperationInProgress
	}
	defer s.repos.MomentLock.Release(userID)
	m, err := s.getOwnedMoment(ctx, classID, momentID)
	if err != nil {
		return nil, err
	}
	existing, err := s.repos.MomentLike.GetByMomentUser(ctx, m.ID, userID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("check existing like: %w", err)
	}
	if existing != nil {
		return nil, ErrMomentAlreadyLiked
	}
	if err := s.repos.MomentLike.LikeMoment(ctx, m.ID, userID); err != nil {
		return nil, fmt.Errorf("like moment: %w", err)
	}
	return s.Get(ctx, classID, momentID, userID)
}

// Unlike 取消点赞；未点赞返回 ErrMomentNotLiked。
func (s *MomentService) Unlike(ctx context.Context, classID, momentID, userID int64) (*api.Moment, error) {
	if err := s.repos.EnsureClassMember(ctx, classID, userID); err != nil {
		return nil, err
	}
	if !s.repos.MomentLock.TryAcquire(userID) {
		return nil, ErrMomentOperationInProgress
	}
	defer s.repos.MomentLock.Release(userID)
	m, err := s.getOwnedMoment(ctx, classID, momentID)
	if err != nil {
		return nil, err
	}
	existing, err := s.repos.MomentLike.GetByMomentUser(ctx, m.ID, userID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("check existing like: %w", err)
	}
	if existing == nil {
		return nil, ErrMomentNotLiked
	}
	if err := s.repos.MomentLike.UnlikeMoment(ctx, m.ID, userID); err != nil {
		return nil, fmt.Errorf("unlike moment: %w", err)
	}
	return s.Get(ctx, classID, momentID, userID)
}

// Comment 评论动态；空内容返回 ErrCommentEmpty。
func (s *MomentService) Comment(ctx context.Context, classID, momentID, userID int64, content string) (*api.Moment, error) {
	if err := s.repos.EnsureClassMember(ctx, classID, userID); err != nil {
		return nil, err
	}
	if content == "" {
		return nil, ErrCommentEmpty
	}
	if !s.repos.MomentLock.TryAcquire(userID) {
		return nil, ErrMomentOperationInProgress
	}
	defer s.repos.MomentLock.Release(userID)
	m, err := s.getOwnedMoment(ctx, classID, momentID)
	if err != nil {
		return nil, err
	}
	c := &model.MomentComment{
		MomentID: m.ID,
		UserID:   userID,
		Content:  content,
	}
	if _, err := s.repos.MomentComment.CommentMoment(ctx, c); err != nil {
		return nil, fmt.Errorf("create comment: %w", err)
	}
	return s.Get(ctx, classID, momentID, userID)
}

// DeleteComment 删除当前用户的评论；非本人或不存在返回 ErrCommentNotFound。
func (s *MomentService) DeleteComment(ctx context.Context, classID, commentID, userID int64) error {
	if err := s.repos.EnsureClassMember(ctx, classID, userID); err != nil {
		return err
	}
	if _, err := s.repos.MomentComment.DeleteCommentWithCount(ctx, commentID, userID, classID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCommentNotFound
		}
		return fmt.Errorf("delete comment: %w", err)
	}
	return nil
}

// GetStats 获取当前用户在指定班级内的互动统计。
func (s *MomentService) GetStats(ctx context.Context, classID, userID int64) (*api.MomentStats, error) {
	if err := s.repos.EnsureClassMember(ctx, classID, userID); err != nil {
		return nil, err
	}
	momentsPosted, err := s.repos.Moment.CountByUserInClass(ctx, userID, classID)
	if err != nil {
		return nil, fmt.Errorf("count moments posted: %w", err)
	}
	likesReceived, err := s.repos.Moment.SumLikeCountByUserInClass(ctx, userID, classID)
	if err != nil {
		return nil, fmt.Errorf("sum likes received: %w", err)
	}
	commentsReceived, err := s.repos.Moment.SumCommentCountByUserInClass(ctx, userID, classID)
	if err != nil {
		return nil, fmt.Errorf("sum comments received: %w", err)
	}
	likesGiven, err := s.repos.MomentLike.CountByUserInClass(ctx, userID, classID)
	if err != nil {
		return nil, fmt.Errorf("count likes given: %w", err)
	}
	commentsGiven, err := s.repos.MomentComment.CountByUserInClass(ctx, userID, classID)
	if err != nil {
		return nil, fmt.Errorf("count comments given: %w", err)
	}
	return &api.MomentStats{
		MomentsPosted:    momentsPosted,
		LikesReceived:    likesReceived,
		CommentsReceived: commentsReceived,
		LikesGiven:       likesGiven,
		CommentsGiven:    commentsGiven,
	}, nil
}

// GetSources 获取班级内可用的来源列表（去重）。
func (s *MomentService) GetSources(ctx context.Context, classID, userID int64) (*api.GetMomentSourcesResp, error) {
	if err := s.repos.EnsureClassMember(ctx, classID, userID); err != nil {
		return nil, err
	}
	sources, err := s.repos.Moment.ListSourcesByClass(ctx, classID)
	if err != nil {
		return nil, fmt.Errorf("list sources: %w", err)
	}
	if sources == nil {
		sources = []string{}
	}
	return &api.GetMomentSourcesResp{Sources: sources}, nil
}

// getOwnedMoment 查询动态并校验班级归属，不匹配返回 ErrMomentNotFound。
func (s *MomentService) getOwnedMoment(ctx context.Context, classID, momentID int64) (*model.Moment, error) {
	m, err := s.repos.Moment.GetByID(ctx, momentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrMomentNotFound
		}
		return nil, fmt.Errorf("get moment: %w", err)
	}
	if m.ClassID != classID {
		return nil, ErrMomentNotFound
	}
	return m, nil
}

// resolveImageResourceIDs 将请求中的业务 resource_id 列表解析为 resources 表主键列表。
func (s *MomentService) resolveImageResourceIDs(ctx context.Context, images *[]struct {
	PicID string `json:"picId"`
}) ([]int64, error) {
	if images == nil || len(*images) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(*images))
	for _, img := range *images {
		res, err := s.repos.Resource.GetByResourceID(ctx, img.PicID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrResourceNotFound
			}
			return nil, fmt.Errorf("get resource by id: %w", err)
		}
		ids = append(ids, res.ID)
	}
	return ids, nil
}

// buildMomentDTO 聚合单条动态的作者/图片/点赞/评论/是否已赞，组装为 api.Moment。
func (s *MomentService) buildMomentDTO(ctx context.Context, m *model.Moment, classID, currentUserID int64) (*api.Moment, error) {
	author, err := s.repos.User.GetByID(ctx, m.UserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("get author: %w", err)
	}
	imageURLs, err := s.repos.MomentImage.ListImageURLsByMomentID(ctx, m.ID)
	if err != nil {
		return nil, fmt.Errorf("list image urls: %w", err)
	}
	likes, err := s.repos.MomentLike.ListByMoment(ctx, m.ID)
	if err != nil {
		return nil, fmt.Errorf("list likes: %w", err)
	}
	likedByMe := false
	likeUserIDs := make([]int64, 0, len(likes))
	for _, l := range likes {
		if l.UserID == currentUserID {
			likedByMe = true
		}
		likeUserIDs = append(likeUserIDs, l.UserID)
	}
	likeUsers, err := s.loadUsers(ctx, likeUserIDs)
	if err != nil {
		return nil, fmt.Errorf("load like users: %w", err)
	}
	comments, err := s.repos.MomentComment.ListByMomentID(ctx, m.ID)
	if err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}
	commentUserIDs := make([]int64, 0, len(comments))
	for _, c := range comments {
		commentUserIDs = append(commentUserIDs, c.UserID)
	}
	commentUsers, err := s.loadUsers(ctx, commentUserIDs)
	if err != nil {
		return nil, fmt.Errorf("load comment users: %w", err)
	}
	return toMomentDTO(m, author, imageURLs, likes, likeUsers, comments, commentUsers, likedByMe), nil
}

// loadUsers 按用户 ID 列表逐个加载并去重，返回 id → *model.User 映射。
func (s *MomentService) loadUsers(ctx context.Context, ids []int64) (map[int64]*model.User, error) {
	result := make(map[int64]*model.User, len(ids))
	for _, id := range ids {
		if _, ok := result[id]; ok {
			continue
		}
		u, err := s.repos.User.GetByID(ctx, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			return nil, err
		}
		result[id] = u
	}
	return result, nil
}

// toMomentDTO 将聚合数据组装为 api.Moment DTO。
func toMomentDTO(m *model.Moment, author *model.User, imageURLs []string,
	likes []model.MomentLike, likeUsers map[int64]*model.User,
	comments []model.MomentComment, commentUsers map[int64]*model.User,
	likedByMe bool) *api.Moment {

	commentItems := make([]momentCommentItem, 0, len(comments))
	for _, c := range comments {
		commentItems = append(commentItems, momentCommentItem{
			Content: c.Content,
			ID:      int(c.ID),
			User:    toUserBaseInfo(commentUsers[c.UserID]),
		})
	}

	likeItems := make([]momentLikeItem, 0, len(likes))
	for _, l := range likes {
		likeItems = append(likeItems, momentLikeItem{
			User: toUserBaseInfo(likeUsers[l.UserID]),
		})
	}

	var images *[]momentImageItem
	if len(imageURLs) > 0 {
		imgs := make([]momentImageItem, 0, len(imageURLs))
		for _, u := range imageURLs {
			if u == "" {
				continue
			}
			imgs = append(imgs, momentImageItem{URL: u})
		}
		if len(imgs) > 0 {
			images = &imgs
		}
	}

	var objs *[]api.MomentExtraObj
	if len(m.ObjsJSON) > 0 && string(m.ObjsJSON) != "[]" {
		var parsed []api.MomentExtraObj
		if err := json.Unmarshal(m.ObjsJSON, &parsed); err == nil && len(parsed) > 0 {
			objs = &parsed
		}
	}

	commentCount := m.CommentCount
	likeCount := m.LikeCount
	return &api.Moment{
		Author:       toUserBaseInfo(author),
		CommentCount: &commentCount,
		Comments:     commentItems,
		Content:      m.Content,
		CreatedAt:    int(m.CreatedAt.Unix()),
		ID:           int(m.ID),
		Images:       images,
		LikeCount:    &likeCount,
		LikedByMe:    likedByMe,
		Likes:        likeItems,
		Objs:         objs,
	}
}

// toUserBaseInfo 将 model.User 转换为 api.UserBaseInfo，姓名优先取 real_name。
// 防御性要求：用户可能已删除（loadUsers 会跳过缺失用户），u 为 nil 时返回空对象避免 panic。
func toUserBaseInfo(u *model.User) api.UserBaseInfo {
	if u == nil {
		return api.UserBaseInfo{}
	}
	info := api.UserBaseInfo{
		UserID: int(u.ID),
		Name:   u.Username,
	}
	if u.RealName != nil && *u.RealName != "" {
		info.Name = *u.RealName
	}
	if u.AvatarURL != nil {
		info.Avatar = u.AvatarURL
	}
	return info
}
