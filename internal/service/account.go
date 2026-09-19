package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"zhonghuawenhua_backend/internal/model"
	"zhonghuawenhua_backend/pkg/jwt"
)

// AccountUser 账号信息（对外返回，剔除密码等敏感字段）。
type AccountUser struct {
	ID         int64  `json:"id"`
	Code       string `json:"code"`
	Username   string `json:"username"`
	Role       string `json:"role"`
	RealName   string `json:"realName"`
	AvatarURL  string `json:"avatarUrl"`
	Gender     string `json:"gender"`
	City       string `json:"city"`
	GradeName  string `json:"gradeName"`
	ClassName  string `json:"className"`
}

// LoginResult 登录成功返回结构。
type LoginResult struct {
	Token string      `json:"token"`
	User  AccountUser `json:"user"`
}

// AccountStatus 登录状态查询结果。
type AccountStatus struct {
	LoggedIn bool        `json:"loggedIn"`
	User     AccountUser `json:"user"`
}

// AccountService 账号登录与状态查询业务。
type AccountService struct {
	repos          *Repositories
	jwtSecret      string
	jwtExpireHours int
}

// NewAccountService 创建 AccountService。
func NewAccountService(repos *Repositories, jwtSecret string, jwtExpireHours int) *AccountService {
	return &AccountService{
		repos:          repos,
		jwtSecret:      jwtSecret,
		jwtExpireHours: jwtExpireHours,
	}
}

// Login 校验用户名/学号 + 密码，更新最后登录时间并签发 JWT。
func (s *AccountService) Login(ctx context.Context, identifier, password string) (*LoginResult, error) {
	user, err := s.findUserByIdentifier(ctx, identifier)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("find user: %w", err)
	}
	if user.Status == 0 {
		return nil, ErrUserDisabled
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}
	if err := s.repos.User.UpdateLastLogin(ctx, user.ID); err != nil {
		return nil, fmt.Errorf("update last login: %w", err)
	}
	token, err := jwt.Generate(s.jwtSecret, int(user.ID), s.jwtExpireHours)
	if err != nil {
		return nil, fmt.Errorf("generate jwt: %w", err)
	}
	return &LoginResult{Token: token, User: toAccountUser(user)}, nil
}

// GetStatus 根据 userID 返回当前登录用户信息。
func (s *AccountService) GetStatus(ctx context.Context, userID int64) (*AccountStatus, error) {
	user, err := s.repos.User.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("get user: %w", err)
	}
	return &AccountStatus{LoggedIn: true, User: toAccountUser(user)}, nil
}

// findUserByIdentifier 先按 username 查询，未命中再按 code（学号）查询。
func (s *AccountService) findUserByIdentifier(ctx context.Context, identifier string) (*model.User, error) {
	user, err := s.repos.User.GetByUsername(ctx, identifier)
	if err == nil {
		return user, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	return s.repos.User.GetByCode(ctx, identifier)
}

// toAccountUser 将 model.User 转为对外 AccountUser，处理可空字段。
func toAccountUser(u *model.User) AccountUser {
	au := AccountUser{
		ID:       u.ID,
		Code:     u.Code,
		Username: u.Username,
		Role:     string(u.Role),
	}
	if u.RealName != nil {
		au.RealName = *u.RealName
	}
	if u.AvatarURL != nil {
		au.AvatarURL = *u.AvatarURL
	}
	if u.Gender != nil {
		au.Gender = *u.Gender
	}
	if u.City != nil {
		au.City = *u.City
	}
	if u.GradeName != nil {
		au.GradeName = *u.GradeName
	}
	if u.ClassName != nil {
		au.ClassName = *u.ClassName
	}
	return au
}
