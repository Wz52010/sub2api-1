# 上游(官方 sub2api)更新 · 合并手册

> 目的:让"跟官方更新"从一次性大手术,变成低风险、可重复的例行操作,同时**保住我们的定制**(EasyPay 加密货币支付、出站 TLS/H2 指纹、若干计费/验证修复)。

## 0. 现状快照(2026-08-26)

- **生产基线**:官方 `fbfdcef81`(2026-08-13),已落后官方 main **≈6200 提交 / 800+ 文件**。
- **我们的定制**(必须保住):
  1. **EasyPay 加密货币支付**(与官方 payment 正交,主要在 `payment_currency*.go` 等)。
  2. **出站指纹套件**(移植自 sub_zheng):`pkg/tlsfingerprint`、`repository/http_upstream_h2_*`、指纹 profile 模型/迁移、前端"指纹与连接"页。
  3. **修复**:问题2(指纹显示)、EditAccountModal openai 保存、档0 诊断、verify 端点 h1 不误报 H2、**长上下文计费**(认证快照保留分组定价)、**Anthropic 缓存重复计费**、dompurify XSS。
- **镜像/回滚**:自建 tag `sub2api-custom:fp-YYYYMMDD[x]`,**永不覆盖** `weishaw/sub2api:latest`(终极回滚 3d1f2e0a)。compose `image:` 指向当前 tag。

## 1. 核心结论:为什么"全量合并"不可取,该怎么做

- 落后 6000+ 提交时,`git merge upstream/main` 或逐个 `git cherry-pick` 都会**大面积冲突**——单个 fix 的上下文文件(如 `openai_gateway_service.go`)已整体漂移,cherry-pick 冲突块能跨整文件。
- **两条并行策略**:
  - **(A) 例行重基线**(主线):每隔 **2~4 周**或官方每个 minor 版本,把我们的定制**重放到官方新基线**上。别再让它漂到几千提交。
  - **(B) 按需摘补丁**(应急):两次重基线之间,命中具体 bug 时,定位官方对应 fix **手工移植核心逻辑**(不走 git cherry-pick,避冲突)。本仓库的长上下文/缓存计费修复就是此法。

## 2. 一次性准备:让定制"可识别、可重放"

1. **上游 remote 常驻**:
   ```bash
   git remote add upstream https://github.com/Wei-Shaw/sub2api.git   # 只读官方
   git fetch upstream main
   ```
2. **定制标记**(降低未来冲突定位成本):所有手改处加统一注释锚点,便于 grep/合并时识别:
   ```go
   // [CUSTOM:easypay] ...      // [CUSTOM:fingerprint] ...      // [CUSTOM:billing-fix] ...
   ```
   合并后 `grep -rn "\[CUSTOM:" backend/ frontend/` 应能列全定制点。
3. **定制尽量隔离成独立文件/包**,减少与官方同文件的行级冲突:
   - EasyPay:集中在 `*_currency*.go` / 独立 handler,少改官方核心文件。
   - 指纹套件:已基本自成包(`pkg/tlsfingerprint`、`http_upstream_h2_*`),保持。
   - 不得已要改官方文件时,改动越小越好、加 `[CUSTOM:*]` 锚点。
4. **迁移号占我们自己的段**:官方用到哪就避开;我们已用 **222–229**,后续继续往上排(230+),**永不复用官方号**,避免 `schema_migrations` 冲突。
5. **启用 rerere**(记住冲突解法,重基线复用):
   ```bash
   git config rerere.enabled true
   git config rerere.autoupdate true
   ```

## 3. 例行重基线流程(策略 A)

在**独立目录/分支**做,别在生产窗口当首次编译现场。

```bash
# 0) 备份 + 记录当前生产 tag(回滚用)
docker exec sub2api-postgres pg_dump -U sub2api sub2api | gzip > backup_$(date +%F).sql.gz

# 1) 取官方新基线
git fetch upstream main
git switch -c rebase/$(date +%Y%m%d) deploy/current   # 从当前定制分支起

# 2) 把我们的定制提交重放到官方新基线(rerere 会复用旧解法)
git rebase --onto upstream/main fbfdcef81 rebase/$(date +%Y%m%d)
#   逐个解冲突 → git add → git rebase --continue
#   指纹/EasyPay 冲突优先"保我们的语义、吸收官方结构变化"

# 3) 生成物别手工合:改完 schema 跑
(cd backend && go generate ./ent && go mod tidy)

# 4) 编译 + 单测(容器内,复用缓存卷)
docker run --rm -v "$PWD/backend":/src -v sub2api_gocache:/root/.cache/go-build \
  -v sub2api_gomod:/go/pkg/mod -e GOPROXY=https://goproxy.cn,direct -e GOFLAGS=-mod=mod \
  -w /src golang:1.26.5 sh -c 'go build ./... && go test -tags unit ./internal/service/ -run "LongContext|Billing|Snapshot"'

# 5) 构建新 tag(独立,永不覆盖 weishaw/sub2api:latest)
DOCKER_BUILDKIT=1 docker build -f Dockerfile -t sub2api-custom:fp-$(date +%Y%m%d) \
  --build-arg GOPROXY=https://goproxy.cn,direct .

# 6) 冒烟(见 §5)→ 改 compose image → up -d → 复验 → 记录新基线 SHA
```

**新基线更新后,务必更新本文件 §0 的基线 SHA + 定制清单。**

## 4. 按需摘补丁流程(策略 B,应急)

```bash
git fetch upstream main
# 找官方对应修复(信息/文件双查)
git log --oneline fbfdcef81..upstream/main -- <相关文件> | grep -iE '关键词'
git show <sha>                      # 读核心 diff
# 先试 cherry-pick;冲突就 abort 改手工移植
git cherry-pick -x <sha> || git cherry-pick --abort
# 手工移植时:①确认 bug 在当前基线确实存在 ②核对目标代码与官方 pre-fix 一致
#            ③只搬核心逻辑 ④编译+(可)单测 ⑤提交注明 "移植官方 <sha>"
```
**红线**:计费/认证代码,若目标上下文与官方差异大、无法逐字核对,**宁可不摘,留到重基线**——硬港易引入更严重的钱/权限 bug。

## 5. 部署前冒烟清单(每次上线必过)

- 容器 `go build ./...` = 0;关键单测通过。
- `docker build` 成功(前端 `pnpm --frozen-lockfile` 不报 `ERR_PNPM_LOCKFILE`;改前端依赖必须同步 `pnpm-lock.yaml`)。
- 起容器 healthy;`/health`=200;公网 `aspai.top`=200 / `pay.aspai.top`=302。
- **EasyPay**:`POST /api/v1/payment/public/orders/verify` 可达(非 500/404)。
- **计费**:长上下文 >272K 请求 `long_context_billing_applied=true` 且有效单价翻倍;近 30s 无 panic/error。
- 保留上一 tag 作即时回滚;`weishaw/sub2api:latest` 作终极回滚。

## 6. 下次重基线的补丁 backlog(本次评估暂缓的官方修复)

命中你实际流量、值得在**重基线**里自然带入(硬港风险高故暂缓):

| 官方提交 | 作用 | 暂缓原因 / 备注 |
|---|---|---|
| `bc3acd6e2` | 注册邮箱别名去重(防 gmail 加点/加号刷小号绕额度) | **适用**(你 `registration_enabled=true` + gmail 白名单);但 133 行认证改动 + 迁移号 190 撞号,需专项谨慎移植或随重基线 |
| `bd52e5d77` | 流中断也记 usage(防漏计费 #5148) | 131 行请求热路径重构,漏港易坏主链路 |
| `de28eba3c` | GPT-5.6 计费/usage 加固 | 28 文件,多数逻辑基线已具备 |
| `4dd3aee5c` | /responses 用 mapped model 计费 | 基线无同名 `wsResult` 上下文,非干净移植 |
| `383f61d0e` | GPT-5.6 对齐官方价 | 基线 litellm 定价文件已正确,fallback 才用,冗余 |

已完成(无需再处理):`a2acbf553`、`0b9f40e23`、`02e50cc22`(OAuth 接管)已在基线;`bc4a9ae43`(缓存重复计费)、`4a1da2950`(dompurify XSS)、`674570ca1`(长上下文快照)已移植。

## 7. 一句话给未来的自己

**别再攒到 6000 提交。** 每月拉一次 upstream、rebase 一次、跑一遍冒烟;定制都带 `[CUSTOM:*]` 锚点、尽量独立成文件、迁移号只增不复用。这样每次合并都是"十几个冲突"而不是"八百个文件"。
