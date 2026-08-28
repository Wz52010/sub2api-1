package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// 供货商门户(Provider Portal):独立于 users/admin 的最小身份子系统。
// 供货商登录 /provider 二级门户、授权上游账号;授权进来的账号通过 Extra.provider_id
// 归属到此身份。整套由 config.ProviderPortal.Enabled 门控,默认关闭休眠。

var (
	ErrProviderNotFound      = errors.New("provider account not found")
	ErrProviderDisabled      = errors.New("provider account disabled")
	ErrProviderBadCredential = errors.New("invalid provider credentials")
	ErrProviderEmailExists   = errors.New("provider email already exists")
)

// ProviderAccount 供货商身份。
type ProviderAccount struct {
	ID              int64
	Name            string
	Email           string
	PasswordHash    string
	AllowedGroupIDs []int64
	DailyAddLimit   int // 0 = 用 config 默认
	Enabled         bool
	Notes           string
	LastLoginAt     *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// ProviderRepository 供货商身份持久化(裸 SQL 实现,避开 ent codegen)。
type ProviderRepository interface {
	Create(ctx context.Context, p *ProviderAccount) (int64, error)
	GetByID(ctx context.Context, id int64) (*ProviderAccount, error)
	GetByEmail(ctx context.Context, email string) (*ProviderAccount, error)
	List(ctx context.Context) ([]*ProviderAccount, error)
	SetEnabled(ctx context.Context, id int64, enabled bool) error
	UpdateLastLogin(ctx context.Context, id int64, at time.Time) error
	Delete(ctx context.Context, id int64) error
	// CountAccountsAddedSince 统计该供货商名下、created_at 晚于 since 的上游账号数(每日新增限流用)。
	CountAccountsAddedSince(ctx context.Context, providerID int64, since time.Time) (int, error)
}

// providerClaims 供货商登录 JWT。typ=provider 使其无法被 user/admin 路由复用,反之亦然。
type providerClaims struct {
	ProviderID int64  `json:"pid"`
	Typ        string `json:"typ"`
	jwt.RegisteredClaims
}

const providerTokenType = "provider"

// ProviderPortalService 供货商身份服务:登录校验、JWT 签发/解析、CRUD、限流判定。
// 不做 OAuth/建号编排(那在 handler 层复用既有 OAuthService + adminService)。
type ProviderPortalService struct {
	repo         ProviderRepository
	jwtSecret    string
	sessionHours int
	dailyLimit   int
}

func NewProviderPortalService(repo ProviderRepository, jwtSecret string, sessionHours, dailyLimit int) *ProviderPortalService {
	if sessionHours <= 0 {
		sessionHours = 12
	}
	return &ProviderPortalService{repo: repo, jwtSecret: jwtSecret, sessionHours: sessionHours, dailyLimit: dailyLimit}
}

func normalizeProviderEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// CreateProvider 后台创建供货商(仅管理员路径调用)。
func (s *ProviderPortalService) CreateProvider(ctx context.Context, name, email, password string, allowedGroupIDs []int64, dailyAddLimit int, notes string) (*ProviderAccount, error) {
	email = normalizeProviderEmail(email)
	if email == "" || password == "" || strings.TrimSpace(name) == "" {
		return nil, errors.New("name/email/password required")
	}
	if existing, _ := s.repo.GetByEmail(ctx, email); existing != nil {
		return nil, ErrProviderEmailExists
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	p := &ProviderAccount{
		Name: strings.TrimSpace(name), Email: email, PasswordHash: string(hash),
		AllowedGroupIDs: allowedGroupIDs, DailyAddLimit: dailyAddLimit, Enabled: true, Notes: notes,
	}
	id, err := s.repo.Create(ctx, p)
	if err != nil {
		return nil, err
	}
	p.ID = id
	return p, nil
}

// Login 校验邮箱+密码,成功返回 JWT + 供货商。
func (s *ProviderPortalService) Login(ctx context.Context, email, password string) (string, *ProviderAccount, error) {
	p, err := s.repo.GetByEmail(ctx, normalizeProviderEmail(email))
	if err != nil || p == nil {
		return "", nil, ErrProviderBadCredential
	}
	if !p.Enabled {
		return "", nil, ErrProviderDisabled
	}
	if bcrypt.CompareHashAndPassword([]byte(p.PasswordHash), []byte(password)) != nil {
		return "", nil, ErrProviderBadCredential
	}
	token, err := s.issueToken(p.ID)
	if err != nil {
		return "", nil, err
	}
	_ = s.repo.UpdateLastLogin(ctx, p.ID, time.Now())
	return token, p, nil
}

func (s *ProviderPortalService) issueToken(providerID int64) (string, error) {
	now := time.Now()
	claims := providerClaims{
		ProviderID: providerID,
		Typ:        providerTokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(s.sessionHours) * time.Hour)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.jwtSecret))
}

// ParseToken 解析并校验供货商 JWT,返回 providerID;typ 必须为 provider。
func (s *ProviderPortalService) ParseToken(tokenString string) (int64, error) {
	var claims providerClaims
	token, err := jwt.ParseWithClaims(tokenString, &claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(s.jwtSecret), nil
	})
	if err != nil || !token.Valid || claims.Typ != providerTokenType || claims.ProviderID <= 0 {
		return 0, ErrProviderBadCredential
	}
	return claims.ProviderID, nil
}

// Authenticate 解析 token 并加载启用中的供货商(中间件用)。
func (s *ProviderPortalService) Authenticate(ctx context.Context, tokenString string) (*ProviderAccount, error) {
	pid, err := s.ParseToken(tokenString)
	if err != nil {
		return nil, err
	}
	p, err := s.repo.GetByID(ctx, pid)
	if err != nil || p == nil {
		return nil, ErrProviderNotFound
	}
	if !p.Enabled {
		return nil, ErrProviderDisabled
	}
	return p, nil
}

func (s *ProviderPortalService) List(ctx context.Context) ([]*ProviderAccount, error) {
	return s.repo.List(ctx)
}

func (s *ProviderPortalService) SetEnabled(ctx context.Context, id int64, enabled bool) error {
	return s.repo.SetEnabled(ctx, id, enabled)
}

func (s *ProviderPortalService) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// EffectiveDailyLimit 供货商自身上限优先,否则用 config 默认。
func (s *ProviderPortalService) EffectiveDailyLimit(p *ProviderAccount) int {
	if p != nil && p.DailyAddLimit > 0 {
		return p.DailyAddLimit
	}
	return s.dailyLimit
}

// CheckDailyAddQuota 返回是否还能新增(true=允许)。limit<=0 表示不限。
func (s *ProviderPortalService) CheckDailyAddQuota(ctx context.Context, p *ProviderAccount) (bool, error) {
	limit := s.EffectiveDailyLimit(p)
	if limit <= 0 {
		return true, nil
	}
	since := time.Now().Truncate(24 * time.Hour)
	n, err := s.repo.CountAccountsAddedSince(ctx, p.ID, since)
	if err != nil {
		return false, err
	}
	return n < limit, nil
}

// GroupAllowed 校验目标分组是否在供货商允许集合内(空集合=不允许任何,必须显式配)。
func (s *ProviderPortalService) GroupAllowed(p *ProviderAccount, groupID int64) bool {
	if p == nil {
		return false
	}
	for _, g := range p.AllowedGroupIDs {
		if g == groupID {
			return true
		}
	}
	return false
}
