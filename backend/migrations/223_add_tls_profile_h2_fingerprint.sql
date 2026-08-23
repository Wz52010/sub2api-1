-- 档2: per-Profile HTTP/2 帧级指纹字段。
--
-- 让 H2 SETTINGS/WINDOW_UPDATE/伪头序/header 序成为 Profile 的一部分(DB 存储、后台可编辑),
-- 使"跟随 Claude/GPT 客户端更新指纹"变成改数据而非改代码。列为空/NULL 时,运行时回退到按
-- 客户端类型选择的内置默认 spec,行为向后兼容。
--
-- 类型与 ent schema 对齐:jsonb 承载有序数组;h2_connection_flow 用 bigint 容纳 uint32;
-- 文本列可空。幂等(IF NOT EXISTS);种子回填仅在列为空时执行,不覆盖用户后续编辑。

ALTER TABLE tls_fingerprint_profiles ADD COLUMN IF NOT EXISTS h2_settings jsonb;
ALTER TABLE tls_fingerprint_profiles ADD COLUMN IF NOT EXISTS h2_connection_flow bigint NOT NULL DEFAULT 0;
ALTER TABLE tls_fingerprint_profiles ADD COLUMN IF NOT EXISTS h2_pseudo_header_order jsonb;
ALTER TABLE tls_fingerprint_profiles ADD COLUMN IF NOT EXISTS h2_header_order jsonb;
ALTER TABLE tls_fingerprint_profiles ADD COLUMN IF NOT EXISTS h2_akamai_expected text;
ALTER TABLE tls_fingerprint_profiles ADD COLUMN IF NOT EXISTS h2_source text;

-- undici (Node.js / Claude Code 链路) 种子。注意:种子值为合理默认,非真实抓包确证,
-- 上生产前应以真实客户端在 tls.peet.ws 的 akamai_fingerprint 校准。
UPDATE tls_fingerprint_profiles SET
    h2_settings = '[[1,65536],[2,0],[4,6291456],[6,262144]]'::jsonb,
    h2_connection_flow = 15663105,
    h2_pseudo_header_order = '[":method",":authority",":scheme",":path"]'::jsonb,
    h2_akamai_expected = '1:65536;2:0;4:6291456;6:262144|15663105|0|m,a,s,p',
    h2_source = 'seed undici (uncalibrated)'
WHERE name IN (
    'Claude Code - Node.js 24.x',
    'Claude/Codex shared - Node.js 22.17.1 Linux x64',
    'Claude/Codex shared - Node.js 24.3.0 macOS arm64'
) AND h2_settings IS NULL;

-- reqwest (真实 Codex CLI = Rust) 种子。同样为合理默认,待真实 codex_cli_rs 抓包校准。
UPDATE tls_fingerprint_profiles SET
    h2_settings = '[[2,0],[4,2097152],[5,16384]]'::jsonb,
    h2_connection_flow = 1048576,
    h2_pseudo_header_order = '[":method",":path",":authority",":scheme"]'::jsonb,
    h2_akamai_expected = '2:0;4:2097152;5:16384|1048576|0|m,p,a,s',
    h2_source = 'seed reqwest (uncalibrated)'
WHERE name = 'Codex CLI - Node.js 24.x' AND h2_settings IS NULL;
