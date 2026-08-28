# 供货商二级门户(Provider Portal)· 运营手册

给「账户供货商」一个自助门户:他们用独立账号登录 `/provider`,提交/授权上游账号,账号自动归属到他们名下、锁进你指定的分组,并能看到账号实况(状态/最后使用/限流/过期/用量,**不含金额**)。与你的用户/管理端完全隔离。

## 当前状态:已部署,**默认休眠**

- 生产镜像 `sub2api-custom:fp-20260827pp` 已上线;`PROVIDER_PORTAL_ENABLED=false`(默认)→ `/api/v1/provider/*` 路由**不注册**(返回 404),对现有用户零影响。
- 迁移 230 已建 `provider_accounts` 表(独立身份,未碰任何现有表)。
- 已冒烟验证(临时开 flag):登录/JWT/列表/鉴权拒绝/前端页全通过,验证后已关回休眠。

## 启用(你控制上线时机)

```bash
cd /opt/sub2api/deploy
sed -i 's/PROVIDER_PORTAL_ENABLED=false/PROVIDER_PORTAL_ENABLED=true/' .env
docker compose --env-file .env -f docker-compose.local.yml up -d sub2api
# 关闭同理:改回 false 再 up -d。可即时回滚。
```

## 创建一个供货商登录(v1 走 SQL 播种)

1) **生成 bcrypt 口令哈希**(把 `你的密码` 换掉):
```bash
cd /root/sub2api-deploy && mkdir -p backend/tmpgenhash
printf 'package main\nimport("fmt";"os";"golang.org/x/crypto/bcrypt")\nfunc main(){h,_:=bcrypt.GenerateFromPassword([]byte(os.Args[1]),bcrypt.DefaultCost);fmt.Println(string(h))}\n' > backend/tmpgenhash/main.go
docker run --rm -v "$PWD/backend":/src -v sub2api_gomod:/go/pkg/mod -e GOFLAGS=-mod=mod -w /src golang:1.26.5 go run ./tmpgenhash '你的密码'
rm -rf backend/tmpgenhash
```

2) **插入供货商**(`allowed_group_ids` = 授权他账号能进的分组;`daily_add_limit=0` 用默认 50):
```sql
INSERT INTO provider_accounts (name, email, password_hash, allowed_group_ids, daily_add_limit, enabled)
VALUES ('供货商A', 'vendorA@example.com', '<上一步的哈希>', '{10,18}', 0, true);
```

3) 把 `https://你的域名/provider/login` + 邮箱/密码给供货商。

> 停用某供货商:`UPDATE provider_accounts SET enabled=false WHERE email='...';`(其名下账号照旧,只是他登不进)。

## 接口(供货商 JWT 鉴权 · `typ=provider`,与 user/admin 互不通用)

| 方法 | 路径 | 作用 |
|---|---|---|
| POST | `/api/v1/provider/auth/login` | 邮箱+密码登录,返回 token |
| GET | `/api/v1/provider/accounts` | 只列本供货商名下账号实况 |
| POST | `/api/v1/provider/accounts` | 提交凭据建号(见下) |
| GET | `/api/v1/provider/accounts/:id/usage?window=day\|week\|month` | 该账号 token/请求用量 |
| DELETE | `/api/v1/provider/accounts/:id` | 撤下自己的账号 |
| POST | `/api/v1/provider/oauth/authurl` | **OAuth 授权**:生成授权链接(见下) |
| POST | `/api/v1/provider/oauth/exchange` | **OAuth 换码 → 建号**(见下) |

建号 body(手工型 apikey/upstream):`{"name","platform","type","credentials":{...},"group_id"}`;`credentials` 结构 = 你后台加该类账号时填的那套(apikey 的 key、upstream 的 base_url+api_key)。

## OAuth 授权建号(v3,主推:不经手原始密码、不发 key、底层直调)

供货商只需登录 → 点授权 → 在自己浏览器跑一遍平台 OAuth,账号就**直接落进你的主调度池**(打 `provider_id` 标签、锁授权分组)。**原始 token 只在服务端流转,不回传浏览器**;换码复用 admin 面板同一套 `OpenAI/OAuth(Claude)/Gemini` OAuthService。

支持平台:**ChatGPT · Codex(openai)、Claude(anthropic)、Gemini**。前端在「账户管理 → OAuth 授权」页,3 步向导:

1. 选平台 + 填名称 + 选授权分组 →「生成授权链接」(`POST /oauth/authurl` body `{"platform":"anthropic|openai|gemini", "gemini_oauth_type?":"code_assist|google_one", "gemini_tier_id?","gemini_project_id?"}` → `{auth_url, session_id, state?}`)。
2. 新标签打开链接完成授权。**OpenAI/Gemini** 授权后浏览器会跳到一个 localhost 打不开的回调页——把地址栏整条链接复制回来即可(前端自动抽 `code`+`state`);**Claude** 页面直接给授权码。
3. 粘回授权码 →「完成授权」(`POST /oauth/exchange` body `{"platform","session_id","code","state?","name","group_id","gemini_oauth_type?","gemini_tier_id?"}`)→ 服务端换码→用官方凭据构造器落库→返回 `{id,status}`。

> 隔离与手工建号同一处强制(`prepareOwnedAccount`):打 `provider_id`、锁 `allowed_group_ids`、名字 `[供N]` 前缀、每日新增配额、写审计日志。Gemini 的 `code_assist` 需 project_id;`ai_studio` 需运营方配置自有 OAuth Client(未配会由服务端明确报错)。

## 隔离与安全(服务端强制,供货商绕不过)

- 建号一律打 `Extra.provider_id=<他的id>`;查询/删除/用量一律带 `provider_id` 过滤 → 只能看/动自己名下账号。
- **分组锁定**:只能把账号放进 `allowed_group_ids` 内的分组,不能自选任意分组。
- 忽略倍率/优先级/其它特权 Extra 字段(防提权);每日新增上限;provider JWT 与用户/管理端 token 互不通用。

## v2 路线(暂未做)

- **浏览器内 OAuth 授权流**:供货商点"授权"→ 在自己浏览器完成 OAuth → 系统回调换 token,**全程不经手原始密码**(比 v1 凭据提交更安全)。已确认可复用现有 `OAuthService.GenerateAuthURL/ExchangeCode`。
- 后台管理供货商的 UI(现走 SQL);新账号"探测通过才自动启用"的严格门控;用量图表/结算导出。

## 不停机切换(蓝绿)——手动步骤,谨慎执行

主站 `aspai.top` 经宝塔 nginx 反代 `127.0.0.1:8080`。要零丢连接切换到新镜像:
1. 用新镜像起第二个容器到 `127.0.0.1:8090`(共用同一 DB/Redis/env);`curl 127.0.0.1:8090/health` 到 200。
2. 备份 `/www/server/panel/vhost/nginx/aspai.top.conf`,把 `proxy_pass 127.0.0.1:8080` 改 `:8090`;`nginx -t && nginx -s reload`(优雅重载,不断连接)。
3. 观察 :8090 正常几分钟 → 停旧容器。回滚:nginx 改回 :8080 + reload。
> 休眠部署(flag 关)无功能变化,常规 `up -d`(~2-3s 抖动)即可;蓝绿留给"开 flag 上线"或未来需要零抖动时用。
