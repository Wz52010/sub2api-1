package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// 号商模式(vendor mode)管理面模块白名单。
//
// 给「账号供货商」单独部署的实例开启后,管理面只保留下列六个模块及其真实依赖;
// 其余 /api/v1/admin/* 子路径(用户管理、系统设置、数据管理、数据库备份、运维监控、
// 操作审计、订阅、卡密、优惠码、公告、风控、邀请返利、渠道监控…)一律返回 404。
//
// 采用「白名单 + 404」而不是「跳过路由注册」:
//   - 改动面小,官方更新合并时几乎不冲突;
//   - 不动 handler 装配,没有 nil 依赖风险;
//   - 404 在 handler 之前返回,安全效果等同于未注册。
//
// 注意:这里只裁「管理面」。网关 /v1/* 必须照常工作——主站正是通过它调用号商实例。
var vendorModeAllowedAdminPrefixes = []string{
	// 1) 分组管理
	"groups",
	// 2) 账号管理(含四个平台的 OAuth 授权建号)
	"accounts", "openai", "gemini", "antigravity", "grok",
	// 3) IP(代理)管理
	"proxies",
	// 4) 指纹与连接
	"tls-fingerprint-profiles",
	// 5) 使用记录(用户/密钥搜索也都在 /usage/* 之下,无需放开 users)
	"usage",
	// 6) API 密钥(号商发 key 给主站做内网对接)
	"api-keys",
	// 支撑项:分组管理页依赖渠道列表;合规确认是进入管理面的前置。
	"channels", "compliance",
	// 注意:**不要**放行 "system"。/admin/system 组里含
	//   POST /system/update    (在线更新)
	//   POST /system/rollback  (版本回滚)
	//   POST /system/restart   (重启服务)
	// 放行它等于把运营方服务器的更新/重启权交给号商。版本号对号商无用,
	// 前端也已在号商模式隐藏 VersionBadge(它就是这组接口的入口)。
}

// vendorModeAdminPathAllowed 判断 /api/v1/admin 之后的剩余路径是否在白名单内。
// rest 形如 "/groups"、"/accounts/12/test"、"" (即 /admin 本身)。
func vendorModeAdminPathAllowed(rest string) bool {
	rest = strings.TrimPrefix(rest, "/")
	if rest == "" {
		return false
	}
	// 只比对第一段,避免 "accountsx" 这种前缀误放行。
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		rest = rest[:i]
	}
	for _, allowed := range vendorModeAllowedAdminPrefixes {
		if rest == allowed {
			return true
		}
	}
	return false
}

// VendorModeAdminGuard 号商模式管理面守卫:白名单之外的管理面接口直接 404。
// 仅在 config.VendorMode.Enabled=true 时挂载;主站实例不挂载、行为完全不变。
func VendorModeAdminGuard(adminBasePath string) gin.HandlerFunc {
	return func(c *gin.Context) {
		rest := strings.TrimPrefix(c.Request.URL.Path, adminBasePath)
		if !vendorModeAdminPathAllowed(rest) {
			// 用 404 而不是 403:不泄露"该功能存在但被禁用"这一信息。
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
				"code":    http.StatusNotFound,
				"message": "not found",
			})
			return
		}
		c.Next()
	}
}
