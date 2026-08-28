# 号商(账号供货商)专用实例 · 运维手册

给合作号商单独部署一套 sub2api:他自己登录管账号,**只看得见六个模块**,数据与主站**完全隔离**,
**只走内网、不对外网暴露**。主站把他的实例绑成一个上游账号来消费。

```
你的客户 → 主 sub2api(公网 aspai.top)
            └─ 上游账号 {base_url: http://sub2api-vendor:8080, api_key: 号商发的key}
                 → 号商 sub2api(仅内网,127.0.0.1:18090)
                      → 号商自己的 LLM 账号(他自己配分组/指纹/IP)
```

## 一、保留的六个模块

| 模块 | 路径 | 后端白名单前缀 |
|---|---|---|
| 分组管理 | `/admin/groups` | `groups`(+ `channels` 依赖) |
| 账号管理 | `/admin/accounts` | `accounts`, `openai`, `gemini`, `antigravity`, `grok` |
| 指纹与连接 | `/admin/fingerprint-isolation` | `tls-fingerprint-profiles` |
| IP 管理 | `/admin/proxies` | `proxies` |
| 使用记录 | `/admin/usage` | `usage`(用户/密钥搜索也在此前缀下) |
| API 密钥 | `/keys` | 用户侧路由,不受 admin 裁剪影响 |

**其余 `/api/v1/admin/*` 一律 404**:用户管理、系统设置、数据管理、数据库备份、运维监控(Ops)、
操作审计、订阅、卡密、优惠码、公告、风控、提示词审计、邀请返利、渠道监控、仪表盘……

> 裁剪在**接口层**(`middleware.VendorModeAdminGuard`),不是只藏菜单——号商直接调 API 也进不去。
> 返回 404 而非 403,不泄露"功能存在但被禁用"。
> 网关 `/v1/*` **不受影响**(主站正是靠它调用号商实例)。

## 二、隔离清单(已逐条验证)

| 维度 | 做法 |
|---|---|
| 数据库 | 独立 database `sub2api_vendor` + 独立角色 `s2vendor`;实测该角色读主库 `accounts`/`users` 均 `permission denied` |
| Redis | 独立逻辑库 `REDIS_DB=3`(主站用 0) |
| 身份 | 独立 `JWT_SECRET` / `TOTP_ENCRYPTION_KEY` / 独立管理员账号,token 与主站互不通用 |
| 网络 | 只绑 `127.0.0.1:18090`,**外网不可达**;不配公网域名、不进 nginx 反代 |
| 数据目录 | `/opt/sub2api-vendor/data`(与主站 `/opt/sub2api/deploy/data` 分开) |

主站实例 **零改动**:号商模式默认 `false`,主站不挂载守卫、行为完全不变。

## 三、启停

```bash
cd /opt/sub2api-vendor
docker compose up -d          # 启动
docker compose logs -f        # 看日志
docker compose down           # 停止(不删数据)
```

号商登录地址:`http://127.0.0.1:18090`(仅本机)。凭据见 `/opt/sub2api-vendor/.env`
(`VENDOR_ADMIN_EMAIL` / `VENDOR_ADMIN_PASSWORD`)。

## 四、号商发 key → 主站绑上游(内网对接)

1. **号商侧**(在他的实例里):
   - 「分组管理」建一个分组,把他的账号加进去;
   - 「账号管理」添加他的 LLM 账号(可用 OAuth 授权或填凭据);
   - 「API 密钥」创建一个 key,绑定到上面那个分组;
   - 把这个 key 交给你。

2. **主站侧**(在 aspai.top 管理端):
   - 「账号管理」→ 新建账号 → 类型 `apikey`
   - 凭据填:
     ```json
     {"api_key": "<号商给的key>", "base_url": "http://sub2api-vendor:8080"}
     ```
   - 绑到你要供货的分组即可。

> `base_url` 用**容器名** `sub2api-vendor:8080`(两个容器在同一 docker 网络 `deploy_sub2api-network`),
> 走容器内网、不经公网、几乎零延迟。也可用 `http://127.0.0.1:18090`,但容器名更稳。

## 五、两条通道分离(已上线)

按「**key 只走本地、UI 走外网**」拆成两条通道:

| 通道 | 入口 | 谁用 | 状态 |
|---|---|---|---|
| **前端 UI / 登录 / OAuth 授权** | `https://vendor.aspai.top`(公网,Cloudflare 橙云 + 复用 aspai.top 证书) | 号商本人 | 开放 |
| **API Key 数据面(网关)** | `http://sub2api-vendor:8080`(docker 内网) | 只有主站 | **公网一律 404** |

实现:`deploy/vendor/nginx-vendor.aspai.top.conf`(已装到 `/www/server/panel/vhost/nginx/`)。
关键分界——**网关路径都在根级**(`/v1`、`/responses`、`/chat/completions`、`/models`…),
而**面板 API 在 `/api/v1/` 之下**,因此可以精确地"只放 UI、堵死 key":

```nginx
location ~ ^/(v1|v1beta|responses|chat/completions|messages|models|embeddings|images|videos|tts|stt|realtime|antigravity|backend-api|alpha|custom-voices)(/|$) {
    return 404;
}
```

**公网实测**:UI `/`、`/login`、`/api/v1/settings/public` 全 200;
`/v1/*`、`/responses`、`/chat/completions`、`/models`、`/images/generations`、`/embeddings` 等全 404;
**带 Bearer 从公网调 `/v1/chat/completions` 同样 404**(即使 key 泄露也无法从外网使用)。
内网侧 `sub2api → sub2api-vendor:8080` `/health` 200、`/v1/models` 401(要 key,正确)。

> ⚠️ 面板已暴露公网:请尽快改掉初始密码并开 2FA。
> ⚠️ 改 nginx 必须用**宝塔**的二进制:`/www/server/nginx/sbin/nginx -t`。
> 直接敲 `nginx -t` 测的是已 mask 的发行版配置,会误判(见历史事故)。

## 六、回滚 / 清理

```bash
cd /opt/sub2api-vendor && docker compose down          # 停实例
docker exec sub2api-postgres psql -U sub2api -d postgres \
  -c "DROP DATABASE sub2api_vendor;" -c "DROP ROLE s2vendor;"   # 彻底清数据(不可逆)
```

主站不受任何影响——号商实例是完全独立的一套容器与库。
