-- 把 Codex Profile 的名称/描述从(遗留的)"Node.js" 文案改为真实的 Rust(reqwest)。
--
-- 背景:迁移 199/200 已把该 Profile 的 ClientHello/签名算法换成真实 rustls 形态,
-- 但名称仍是 'Codex CLI - Node.js 24.x'、描述里仍写 "Node.js 24.x runtime"。
-- 由于 BuildMetadata 用 name+description 推断 client_type,残留的 "node" 文案会把
-- 这个 Rust Profile 误显示成 "Node.js / Claude Code"(前端下拉副标题看起来没改对)。
-- 本迁移让名称诚实,并配合 BuildMetadata 的 rust/reqwest 分支正确显示为 Codex/Rust。
--
-- 幂等:若已改名(例如测试环境手工改过),WHERE 不命中即 no-op。
-- 编号:需排在 kit 迁移(合并进生产时重编号为 222-226)之后;如与你现有迁移撞号,改成更大的编号。

UPDATE tls_fingerprint_profiles
SET name = 'Codex CLI - Rust (reqwest)',
    description = 'Codex CLI transport baseline. Real Codex CLI is a Rust client (codex_cli_rs, reqwest/rustls). This profile reproduces its rustls ClientHello + per-connection extension shuffle (JA3 varies per connection, JA4 stable). Source: codex-cli 0.148.0-alpha.15 / macOS / arm64.',
    updated_at = NOW()
WHERE name = 'Codex CLI - Node.js 24.x';
