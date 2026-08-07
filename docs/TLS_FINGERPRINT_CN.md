# Sub2API TLS 指纹稳定性方案

## 先说结论

这套优化适合“本地号池 + 每个账号固定代理/IP + Sub2API 转发”的稳定性建设：

- 每个账号可以绑定一个固定的 TLS Profile。
- 每个账号可以绑定一个独立代理；建议使用 `socks5h://`，让 DNS 也走代理。
- 选择“按账号稳定分配”时，同一账号不会在每次请求之间随机切换 Profile。
- Profile 的 TLS 参数发生变化后，旧的上游 Transport 不会继续承载新请求。

TLS 指纹只描述出站 TLS ClientHello 的参数，不等于官方客户端、订阅资格或授权证明，也不保证上游账号策略的结果。请只用于自己有权管理的账号和合规的客户端兼容测试。

## 推荐的账号配置

1. 在“TLS 指纹 Profile”页面创建少量、用途明确的 Profile，例如：
   - `claude-node24-stable`
   - `claude-node20-legacy`
2. 在账号编辑页打开“TLS 指纹模拟”。
3. 对需要固定环境的账号直接选择一个 Profile；不要把多个账号都依赖“随机”。
4. 给每个账号绑定独立代理，代理地址使用完整格式，例如：
   - `socks5h://user:password@proxy.example:1080`
   - `http://user:password@proxy.example:8080`
5. 保存后先用“账号测试”验证，再逐步放量。

## 三种 Profile 选择方式

| 选择 | 运行方式 | 建议 |
| --- | --- | --- |
| 内置默认 | 使用代码内置 Node.js 24.x-oriented 默认值 | 想快速开始时使用 |
| 指定 Profile | 账号固定绑定数据库 Profile ID | 生产号池首选 |
| 按账号稳定分配 | 兼容旧值 `-1`，按账号 ID 稳定选择一个 Profile | 需要多 Profile 分摊时使用 |

“按账号稳定分配”不是每次请求重新抽签。Profile 列表顺序变化不会导致整体换绑；只有删除当前可选 Profile、或账号绑定关系发生变化时，结果才可能改变。

## 连接复用和修改生效

Sub2API 会把以下信息组合成 TLS 客户端缓存键：

`隔离模式 + 代理地址摘要 + 账号 ID（按隔离模式）+ 协议模式 + TLS Profile 内容摘要`

因此：

- 同一账号、同一代理、同一 Profile 可以复用连接，减少重复 TCP/TLS 握手。
- 修改 Cipher Suites、Curves、ALPN、Extensions 等字段后，会创建新的 Transport。
- Profile 名称单独改名不会改变 ClientHello，不会造成无意义的连接池重建。
- 旧 Transport 会按连接池的空闲回收策略清理，不会再被新 Profile 的请求复用。

## 验证清单

建议按下面顺序验证，不要只看“请求成功”：

1. 账号测试能正常完成，确认代理用户名、密码和协议无误。
2. 在代理服务商或目标测试端确认出口 IP 与预期账号一致。
3. 使用 TLS 指纹测试站或自建测试端检查 JA3/JA4、ALPN 和协商协议。
4. 记录修改 Profile 前后的指纹摘要和请求时间，确认修改后新连接使用新配置。
5. 观察 Sub2API 日志中的 `tls_fingerprint_creating_new_client` 和 `tls_fingerprint_reusing_client`。

## 常见误区

- TLS 指纹不是 IP 隔离：要隔离出口环境，必须同时给账号配置独立代理。
- `socks5://` 可能在本地解析域名；需要避免 DNS 泄漏时使用 `socks5h://`。
- 指纹参数、HTTP 请求头、Cookie、账号授权状态是不同层次，不能用 TLS 参数替代授权配置。
- 不要频繁修改同一个 Profile；每次修改都会产生新的 Transport，短时间内会增加握手和连接数。
- 不要把代理密码写入日志、截图或提交到 Git。

## 部署后建议

先用 1 个测试账号验证“代理出口 IP + TLS Profile + Claude 请求”三件事，再扩展到整个号池。建议每个账号保持固定的代理、Profile 和用途，变更时记录时间和原因，便于定位连接、额度或上游策略问题。
