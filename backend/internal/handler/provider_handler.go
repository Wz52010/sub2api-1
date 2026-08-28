package handler

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// ProviderHandler 供货商二级门户 HTTP 层。仅在 config.ProviderPortal.Enabled=true 时注册路由。
// 复用 AdminService.CreateAccount 建号,强制打 Extra.provider_id 标签 + 锁定分组,实现按供货商隔离。
// OAuth 授权建号复用既有的 3 个平台 OAuthService(与 admin 面板同一套换码逻辑),
// 但换码后落库统一走本 handler 的 prepareOwnedAccount——隔离红线(provider_id/锁分组/名字前缀)一处执行。
type ProviderHandler struct {
	providerSvc *service.ProviderPortalService
	adminSvc    service.AdminService
	openaiOAuth *service.OpenAIOAuthService
	claudeOAuth *service.OAuthService
	geminiOAuth *service.GeminiOAuthService
}

// NewProviderHandler 构造。openai/claude/gemini OAuth 服务为可选(nil 时对应平台授权返回 400,不影响其余功能)。
func NewProviderHandler(
	providerSvc *service.ProviderPortalService,
	adminSvc service.AdminService,
	openaiOAuth *service.OpenAIOAuthService,
	claudeOAuth *service.OAuthService,
	geminiOAuth *service.GeminiOAuthService,
) *ProviderHandler {
	return &ProviderHandler{
		providerSvc: providerSvc,
		adminSvc:    adminSvc,
		openaiOAuth: openaiOAuth,
		claudeOAuth: claudeOAuth,
		geminiOAuth: geminiOAuth,
	}
}

const contextKeyProvider = "provider_account"

// AuthMiddleware 供货商 JWT 鉴权(typ=provider),自带 providerSvc,路由注册只需 h.Provider。
func (h *ProviderHandler) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		parts := strings.SplitN(c.GetHeader("Authorization"), " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			response.Unauthorized(c, "Authorization required")
			c.Abort()
			return
		}
		provider, err := h.providerSvc.Authenticate(c.Request.Context(), parts[1])
		if err != nil {
			response.Unauthorized(c, "invalid or expired provider session")
			c.Abort()
			return
		}
		c.Set(contextKeyProvider, provider)
		c.Next()
	}
}

func currentProvider(c *gin.Context) *service.ProviderAccount {
	if v, ok := c.Get(contextKeyProvider); ok {
		if p, ok := v.(*service.ProviderAccount); ok {
			return p
		}
	}
	return nil
}

type providerLoginRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Login POST /provider/auth/login —— 邮箱+密码登录,返回供货商 JWT。
func (h *ProviderHandler) Login(c *gin.Context) {
	var req providerLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	token, p, err := h.providerSvc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		response.Unauthorized(c, "邮箱或密码错误,或账号已停用")
		return
	}
	h.providerSvc.LogAction(c.Request.Context(), p.ID, "login", "", c.ClientIP())
	response.Success(c, gin.H{
		"token": token,
		"provider": gin.H{
			"id": p.ID, "name": p.Name, "email": p.Email, "allowed_group_ids": p.AllowedGroupIDs,
		},
	})
}

// ListAccounts GET /provider/accounts —— 只列出本供货商名下账号实况。
func (h *ProviderHandler) ListAccounts(c *gin.Context) {
	p := currentProvider(c)
	if p == nil {
		response.Unauthorized(c, "unauthorized")
		return
	}
	accts, err := h.providerSvc.ListOwnedAccounts(c.Request.Context(), p.ID)
	if err != nil {
		response.InternalError(c, "查询账号失败")
		return
	}
	response.Success(c, gin.H{"accounts": accts})
}

type providerCreateAccountRequest struct {
	Name        string         `json:"name" binding:"required"`
	Platform    string         `json:"platform" binding:"required"`
	Type        string         `json:"type" binding:"required"`
	Credentials map[string]any `json:"credentials" binding:"required"`
	GroupID     int64          `json:"group_id" binding:"required"`
}

// prepareOwnedAccount 是"强制归属本供货商"的建号输入构造器:校验分组授权 + 每日配额,
// 打 Extra.provider_id 标签、锁定授权分组、名字加 [供N] 前缀。手工建号与 OAuth 建号共用,
// 确保隔离红线在唯一一处执行。返回的 error 皆为用户可见文案。
func (h *ProviderHandler) prepareOwnedAccount(ctx context.Context, p *service.ProviderAccount, name, platform, accType string, credentials map[string]any, groupID int64) (*service.CreateAccountInput, error) {
	// 分组白名单:供货商只能把账号放进被授权的分组(服务端强制)。
	if !h.providerSvc.GroupAllowed(p, groupID) {
		return nil, errors.New("该分组未授权给你")
	}
	// 每日新增上限。
	if ok, err := h.providerSvc.CheckDailyAddQuota(ctx, p); err != nil {
		return nil, errors.New("限额校验失败")
	} else if !ok {
		return nil, errors.New("今日新增账号已达上限")
	}
	return &service.CreateAccountInput{
		Name:        fmt.Sprintf("[供%d]%s", p.ID, strings.TrimSpace(name)),
		Platform:    strings.TrimSpace(platform),
		Type:        strings.TrimSpace(accType),
		Credentials: credentials,
		// 归属标签(隔离核心)+ 强制锁定到授权分组。
		Extra:    map[string]any{"provider_id": p.ID},
		GroupIDs: []int64{groupID},
	}, nil
}

// CreateAccount POST /provider/accounts —— 供货商提交账号凭据(apikey/upstream 等手工型),建号并归属到自己名下、锁进指定分组。
// OAuth 型账号走 /provider/oauth/* 授权流,不经此端点。
func (h *ProviderHandler) CreateAccount(c *gin.Context) {
	p := currentProvider(c)
	if p == nil {
		response.Unauthorized(c, "unauthorized")
		return
	}
	var req providerCreateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	input, err := h.prepareOwnedAccount(c.Request.Context(), p, req.Name, req.Platform, req.Type, req.Credentials, req.GroupID)
	if err != nil {
		response.Forbidden(c, err.Error())
		return
	}
	acct, err := h.adminSvc.CreateAccount(c.Request.Context(), input)
	if err != nil {
		response.BadRequest(c, "建号失败:"+err.Error())
		return
	}
	h.providerSvc.LogAction(c.Request.Context(), p.ID, "account.create", fmt.Sprintf("account #%d %s/%s", acct.ID, req.Platform, req.Type), c.ClientIP())
	response.Success(c, gin.H{"id": acct.ID, "status": acct.Status})
}

// AccountUsage GET /provider/accounts/:id/usage?window=day|week|month —— 本账号 token/请求用量(不含金额)。
func (h *ProviderHandler) AccountUsage(c *gin.Context) {
	p := currentProvider(c)
	if p == nil {
		response.Unauthorized(c, "unauthorized")
		return
	}
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	owned, err := h.providerSvc.AccountOwnedBy(c.Request.Context(), p.ID, accountID)
	if err != nil || !owned {
		response.Forbidden(c, "无权访问该账号")
		return
	}
	since := time.Now().Add(-24 * time.Hour)
	switch c.Query("window") {
	case "week":
		since = time.Now().Add(-7 * 24 * time.Hour)
	case "month":
		since = time.Now().Add(-30 * 24 * time.Hour)
	}
	usage, err := h.providerSvc.AccountUsageSince(c.Request.Context(), p.ID, accountID, since)
	if err != nil {
		response.InternalError(c, "查询用量失败")
		return
	}
	response.Success(c, usage)
}

// DeleteAccount DELETE /provider/accounts/:id —— 撤下本供货商名下账号。
func (h *ProviderHandler) DeleteAccount(c *gin.Context) {
	p := currentProvider(c)
	if p == nil {
		response.Unauthorized(c, "unauthorized")
		return
	}
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	owned, err := h.providerSvc.AccountOwnedBy(c.Request.Context(), p.ID, accountID)
	if err != nil || !owned {
		response.Forbidden(c, "无权操作该账号")
		return
	}
	if err := h.adminSvc.DeleteAccount(c.Request.Context(), accountID); err != nil {
		response.InternalError(c, "删除失败:"+err.Error())
		return
	}
	h.providerSvc.LogAction(c.Request.Context(), p.ID, "account.delete", fmt.Sprintf("account #%d", accountID), c.ClientIP())
	response.Success(c, gin.H{"deleted": accountID})
}

// Dashboard GET /provider/dashboard —— 本供货商账号汇总 + 今日用量(无用户信息)。
func (h *ProviderHandler) Dashboard(c *gin.Context) {
	p := currentProvider(c)
	if p == nil {
		response.Unauthorized(c, "unauthorized")
		return
	}
	d, err := h.providerSvc.Dashboard(c.Request.Context(), p.ID)
	if err != nil {
		response.InternalError(c, "查询失败")
		return
	}
	response.Success(c, d)
}

// ListGroups GET /provider/groups —— 只读:被授权的分组(运营方所设,零用户/定价细节)。
func (h *ProviderHandler) ListGroups(c *gin.Context) {
	p := currentProvider(c)
	if p == nil {
		response.Unauthorized(c, "unauthorized")
		return
	}
	gs, err := h.providerSvc.AllowedGroups(c.Request.Context(), p)
	if err != nil {
		response.InternalError(c, "查询分组失败")
		return
	}
	response.Success(c, gin.H{"groups": gs})
}

// ListProxies GET /provider/proxies —— 只本供货商自己的代理。
func (h *ProviderHandler) ListProxies(c *gin.Context) {
	p := currentProvider(c)
	if p == nil {
		response.Unauthorized(c, "unauthorized")
		return
	}
	px, err := h.providerSvc.ListProxies(c.Request.Context(), p.ID)
	if err != nil {
		response.InternalError(c, "查询代理失败")
		return
	}
	response.Success(c, gin.H{"proxies": px})
}

type providerCreateProxyRequest struct {
	Name     string `json:"name" binding:"required"`
	Protocol string `json:"protocol" binding:"required"`
	Host     string `json:"host" binding:"required"`
	Port     int    `json:"port" binding:"required"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// CreateProxy POST /provider/proxies —— 供货商添加自己的代理(打 provider_id 标签)。
func (h *ProviderHandler) CreateProxy(c *gin.Context) {
	p := currentProvider(c)
	if p == nil {
		response.Unauthorized(c, "unauthorized")
		return
	}
	var req providerCreateProxyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	id, err := h.providerSvc.CreateProxy(c.Request.Context(), p.ID, &service.ProviderProxy{
		Name: strings.TrimSpace(req.Name), Protocol: strings.TrimSpace(req.Protocol),
		Host: strings.TrimSpace(req.Host), Port: req.Port,
	}, req.Username, req.Password)
	if err != nil {
		response.BadRequest(c, "创建代理失败:"+err.Error())
		return
	}
	h.providerSvc.LogAction(c.Request.Context(), p.ID, "proxy.create", fmt.Sprintf("proxy #%d %s", id, req.Host), c.ClientIP())
	response.Success(c, gin.H{"id": id})
}

// DeleteProxy DELETE /provider/proxies/:id —— 撤下本供货商代理。
func (h *ProviderHandler) DeleteProxy(c *gin.Context) {
	p := currentProvider(c)
	if p == nil {
		response.Unauthorized(c, "unauthorized")
		return
	}
	proxyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	if err := h.providerSvc.DeleteProxy(c.Request.Context(), p.ID, proxyID); err != nil {
		response.Forbidden(c, "删除失败:"+err.Error())
		return
	}
	h.providerSvc.LogAction(c.Request.Context(), p.ID, "proxy.delete", fmt.Sprintf("proxy #%d", proxyID), c.ClientIP())
	response.Success(c, gin.H{"deleted": proxyID})
}

// Usage GET /provider/usage?window=day|week|month —— 本供货商账号用量(token-only,无用户)。
func (h *ProviderHandler) Usage(c *gin.Context) {
	p := currentProvider(c)
	if p == nil {
		response.Unauthorized(c, "unauthorized")
		return
	}
	since := time.Now().Add(-24 * time.Hour)
	switch c.Query("window") {
	case "week":
		since = time.Now().Add(-7 * 24 * time.Hour)
	case "month":
		since = time.Now().Add(-30 * 24 * time.Hour)
	}
	rows, err := h.providerSvc.Usage(c.Request.Context(), p.ID, since)
	if err != nil {
		response.InternalError(c, "查询用量失败")
		return
	}
	response.Success(c, gin.H{"usage": rows})
}

// AuditLogs GET /provider/audit —— 只本供货商自己的操作日志。
func (h *ProviderHandler) AuditLogs(c *gin.Context) {
	p := currentProvider(c)
	if p == nil {
		response.Unauthorized(c, "unauthorized")
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	rows, err := h.providerSvc.ListAudit(c.Request.Context(), p.ID, limit)
	if err != nil {
		response.InternalError(c, "查询日志失败")
		return
	}
	response.Success(c, gin.H{"logs": rows})
}

// ==== OAuth 授权建号(不经手原始密码 / 不发 key;换码后账号直接落进主池并打 provider_id 标签)====

// providerNormalizePlatform 把前端平台别名收敛到内部 platform 常量。空串 = 不支持。
func providerNormalizePlatform(p string) string {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "openai", "chatgpt", "codex":
		return service.PlatformOpenAI
	case "anthropic", "claude":
		return service.PlatformAnthropic
	case "gemini", "google":
		return service.PlatformGemini
	}
	return ""
}

// providerOAuthRedirectURI 复刻 admin 侧从请求 Origin/Host 推导 OAuth 回调地址的逻辑(Gemini 授权需要)。
func providerOAuthRedirectURI(c *gin.Context) string {
	if origin := strings.TrimSpace(c.GetHeader("Origin")); origin != "" {
		return strings.TrimRight(origin, "/") + "/auth/callback"
	}
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	if xf := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")); xf != "" {
		scheme = strings.TrimSpace(strings.Split(xf, ",")[0])
	}
	host := strings.TrimSpace(c.Request.Host)
	if xf := strings.TrimSpace(c.GetHeader("X-Forwarded-Host")); xf != "" {
		host = strings.TrimSpace(strings.Split(xf, ",")[0])
	}
	if host == "" {
		return ""
	}
	return scheme + "://" + host + "/auth/callback"
}

type providerOAuthAuthURLRequest struct {
	Platform        string `json:"platform" binding:"required"`
	ProxyID         *int64 `json:"proxy_id"`
	GeminiOAuthType string `json:"gemini_oauth_type"`
	GeminiTierID    string `json:"gemini_tier_id"`
	GeminiProjectID string `json:"gemini_project_id"`
}

// OAuthAuthURL POST /provider/oauth/authurl —— 生成 OAuth 授权链接。
// PKCE 会话保存在各 OAuthService 的进程内存中(与 admin 面板同一套单例),换码时按 session_id 找回。
func (h *ProviderHandler) OAuthAuthURL(c *gin.Context) {
	p := currentProvider(c)
	if p == nil {
		response.Unauthorized(c, "unauthorized")
		return
	}
	var req providerOAuthAuthURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	ctx := c.Request.Context()
	switch providerNormalizePlatform(req.Platform) {
	case service.PlatformOpenAI:
		if h.openaiOAuth == nil {
			response.BadRequest(c, "OpenAI OAuth 未启用")
			return
		}
		res, err := h.openaiOAuth.GenerateAuthURL(ctx, req.ProxyID, "", service.PlatformOpenAI)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		response.Success(c, gin.H{"auth_url": res.AuthURL, "session_id": res.SessionID})
	case service.PlatformAnthropic:
		if h.claudeOAuth == nil {
			response.BadRequest(c, "Claude OAuth 未启用")
			return
		}
		res, err := h.claudeOAuth.GenerateAuthURL(ctx, req.ProxyID)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		response.Success(c, gin.H{"auth_url": res.AuthURL, "session_id": res.SessionID})
	case service.PlatformGemini:
		if h.geminiOAuth == nil {
			response.BadRequest(c, "Gemini OAuth 未启用")
			return
		}
		otype := strings.TrimSpace(req.GeminiOAuthType)
		if otype == "" {
			otype = "code_assist"
		}
		res, err := h.geminiOAuth.GenerateAuthURL(ctx, req.ProxyID, providerOAuthRedirectURI(c), strings.TrimSpace(req.GeminiProjectID), otype, strings.TrimSpace(req.GeminiTierID))
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		response.Success(c, gin.H{"auth_url": res.AuthURL, "session_id": res.SessionID, "state": res.State})
	default:
		response.BadRequest(c, "不支持的平台")
	}
}

type providerOAuthExchangeRequest struct {
	Platform        string `json:"platform" binding:"required"`
	SessionID       string `json:"session_id" binding:"required"`
	Code            string `json:"code" binding:"required"`
	State           string `json:"state"`
	ProxyID         *int64 `json:"proxy_id"`
	Name            string `json:"name" binding:"required"`
	GroupID         int64  `json:"group_id" binding:"required"`
	GeminiOAuthType string `json:"gemini_oauth_type"`
	GeminiTierID    string `json:"gemini_tier_id"`
}

// OAuthExchange POST /provider/oauth/exchange —— 换码 → 建号。
// 换码复用 admin 同款 OAuthService;落库统一经 prepareOwnedAccount 强制归属(provider_id / 锁分组 / 名字前缀)。
// 原始 token 只在服务端流转,不回传浏览器。
func (h *ProviderHandler) OAuthExchange(c *gin.Context) {
	p := currentProvider(c)
	if p == nil {
		response.Unauthorized(c, "unauthorized")
		return
	}
	var req providerOAuthExchangeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	ctx := c.Request.Context()
	platform := providerNormalizePlatform(req.Platform)

	var credentials map[string]any
	switch platform {
	case service.PlatformOpenAI:
		if h.openaiOAuth == nil {
			response.BadRequest(c, "OpenAI OAuth 未启用")
			return
		}
		if strings.TrimSpace(req.State) == "" {
			response.BadRequest(c, "state 缺失")
			return
		}
		ti, err := h.openaiOAuth.ExchangeCode(ctx, &service.OpenAIExchangeCodeInput{
			SessionID: req.SessionID, Code: req.Code, State: req.State, ProxyID: req.ProxyID,
		})
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		credentials = h.openaiOAuth.BuildAccountCredentials(ti)
	case service.PlatformAnthropic:
		if h.claudeOAuth == nil {
			response.BadRequest(c, "Claude OAuth 未启用")
			return
		}
		ti, err := h.claudeOAuth.ExchangeCode(ctx, &service.ExchangeCodeInput{
			SessionID: req.SessionID, Code: req.Code, ProxyID: req.ProxyID,
		})
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		// 复用官方 Claude 凭据构造器(与后台刷新器同一套 key/格式),再补 OAuth 身份元数据。
		credentials = service.BuildClaudeAccountCredentials(ti)
		if ti.OrgUUID != "" {
			credentials["org_uuid"] = ti.OrgUUID
		}
		if ti.AccountUUID != "" {
			credentials["account_uuid"] = ti.AccountUUID
		}
		if ti.EmailAddress != "" {
			credentials["email_address"] = ti.EmailAddress
		}
	case service.PlatformGemini:
		if h.geminiOAuth == nil {
			response.BadRequest(c, "Gemini OAuth 未启用")
			return
		}
		if strings.TrimSpace(req.State) == "" {
			response.BadRequest(c, "state 缺失")
			return
		}
		otype := strings.TrimSpace(req.GeminiOAuthType)
		if otype == "" {
			otype = "code_assist"
		}
		ti, err := h.geminiOAuth.ExchangeCode(ctx, &service.GeminiExchangeCodeInput{
			SessionID: req.SessionID, State: req.State, Code: req.Code, ProxyID: req.ProxyID,
			OAuthType: otype, TierID: strings.TrimSpace(req.GeminiTierID),
		})
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		credentials = h.geminiOAuth.BuildAccountCredentials(ti) // 已折叠 tokenInfo.Extra
	default:
		response.BadRequest(c, "不支持的平台")
		return
	}

	input, err := h.prepareOwnedAccount(ctx, p, req.Name, platform, service.AccountTypeOAuth, credentials, req.GroupID)
	if err != nil {
		response.Forbidden(c, err.Error())
		return
	}
	acct, err := h.adminSvc.CreateAccount(ctx, input)
	if err != nil {
		response.BadRequest(c, "建号失败:"+err.Error())
		return
	}
	h.providerSvc.LogAction(ctx, p.ID, "account.create", fmt.Sprintf("account #%d %s/oauth", acct.ID, platform), c.ClientIP())
	response.Success(c, gin.H{"id": acct.ID, "status": acct.Status})
}
