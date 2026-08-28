package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// ProviderAuthMiddleware 供货商门户鉴权(独立于 user/admin)。
type ProviderAuthMiddleware gin.HandlerFunc

const contextKeyProvider = "provider_account"

// NewProviderAuthMiddleware 校验供货商 JWT(typ=provider),加载启用中的供货商放入 context。
func NewProviderAuthMiddleware(svc *service.ProviderPortalService) ProviderAuthMiddleware {
	return ProviderAuthMiddleware(func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			AbortWithError(c, 401, "UNAUTHORIZED", "Authorization header is required")
			return
		}
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			AbortWithError(c, 401, "UNAUTHORIZED", "invalid authorization scheme")
			return
		}
		provider, err := svc.Authenticate(c.Request.Context(), parts[1])
		if err != nil {
			AbortWithError(c, 401, "UNAUTHORIZED", "invalid or expired provider session")
			return
		}
		c.Set(contextKeyProvider, provider)
		c.Next()
	})
}

// ProviderFromContext 取出当前请求的供货商(经 ProviderAuthMiddleware 放入)。
func ProviderFromContext(c *gin.Context) *service.ProviderAccount {
	if v, ok := c.Get(contextKeyProvider); ok {
		if p, ok := v.(*service.ProviderAccount); ok {
			return p
		}
	}
	return nil
}
