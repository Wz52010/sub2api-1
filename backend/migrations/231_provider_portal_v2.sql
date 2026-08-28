-- 供货商门户 v2:操作日志表 + 代理归属标签。纯加法,不触碰现有表数据。编号 231。

-- 供货商操作日志(只记供货商自己的动作,与全站 audit_logs 隔离)。
CREATE TABLE IF NOT EXISTS provider_audit_logs (
    id          BIGSERIAL PRIMARY KEY,
    provider_id BIGINT      NOT NULL,
    action      VARCHAR(64) NOT NULL,
    detail      TEXT,
    client_ip   VARCHAR(64),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_provider_audit_provider ON provider_audit_logs (provider_id, created_at DESC);

-- 代理归属:NULL = 运营方自有代理(现状不变);非 NULL = 某供货商自带代理,仅其可见/可管。
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS provider_id BIGINT;
CREATE INDEX IF NOT EXISTS idx_proxies_provider ON proxies (provider_id) WHERE provider_id IS NOT NULL;
