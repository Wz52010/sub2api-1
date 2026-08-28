package handler

import (
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
type ProviderHandler struct {
	providerSvc *service.ProviderPortalService
	adminSvc    service.AdminService
}

// NewProviderHandler 构造。
func NewProviderHandler(providerSvc *service.ProviderPortalService, adminSvc service.AdminService) *ProviderHandler {
	return &ProviderHandler{providerSvc: providerSvc, adminSvc: adminSvc}
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

// CreateAccount POST /provider/accounts —— 供货商提交账号凭据,建号并归属到自己名下、锁进指定分组。
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
	// 分组白名单:供货商只能把账号放进被授权的分组(服务端强制)。
	if !h.providerSvc.GroupAllowed(p, req.GroupID) {
		response.Forbidden(c, "该分组未授权给你")
		return
	}
	// 每日新增上限。
	if ok, err := h.providerSvc.CheckDailyAddQuota(c.Request.Context(), p); err != nil {
		response.InternalError(c, "限额校验失败")
		return
	} else if !ok {
		response.Forbidden(c, "今日新增账号已达上限")
		return
	}
	name := strings.TrimSpace(req.Name)
	input := &service.CreateAccountInput{
		Name:        fmt.Sprintf("[供%d]%s", p.ID, name),
		Platform:    strings.TrimSpace(req.Platform),
		Type:        strings.TrimSpace(req.Type),
		Credentials: req.Credentials,
		// 归属标签(隔离核心)+ 强制锁定到授权分组。
		Extra:    map[string]any{"provider_id": p.ID},
		GroupIDs: []int64{req.GroupID},
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
