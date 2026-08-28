package routes

import (
	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
)

// RegisterProviderRoutes 注册供货商二级门户路由。
// flag 关闭(默认)或 handler 未装配时整组不注册——对现有用户零暴露、彻底休眠。
func RegisterProviderRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	cfg *config.Config,
	panelRateLimiter *middleware.PanelRateLimiter,
) {
	if cfg == nil || !cfg.ProviderPortal.Enabled || h.Provider == nil {
		return
	}
	g := v1.Group("/provider")
	g.Use(panelRateLimiter.PublicIP())
	// 公开:登录
	g.POST("/auth/login", h.Provider.Login)
	// 需供货商 JWT 的接口
	auth := g.Group("")
	auth.Use(h.Provider.AuthMiddleware())
	{
		auth.GET("/dashboard", h.Provider.Dashboard)
		auth.GET("/accounts", h.Provider.ListAccounts)
		auth.POST("/accounts", h.Provider.CreateAccount)
		auth.GET("/accounts/:id/usage", h.Provider.AccountUsage)
		auth.DELETE("/accounts/:id", h.Provider.DeleteAccount)
		auth.GET("/groups", h.Provider.ListGroups)
		auth.GET("/proxies", h.Provider.ListProxies)
		auth.POST("/proxies", h.Provider.CreateProxy)
		auth.DELETE("/proxies/:id", h.Provider.DeleteProxy)
		auth.GET("/usage", h.Provider.Usage)
		auth.GET("/audit", h.Provider.AuditLogs)
	}
}
