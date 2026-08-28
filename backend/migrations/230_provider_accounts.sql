-- 供货商门户:供货商身份表。独立于 users/admin,仅用于登录 /provider 二级门户并授权账号;
-- 授权进来的上游账号通过 Extra.provider_id 标签归属到本表 id,实现按供货商隔离。
-- 整套功能由 provider_portal.enabled 门控,默认关闭(休眠),本迁移为纯加法建表,不触碰任何现有表。编号 230。

CREATE TABLE IF NOT EXISTS provider_accounts (
    id                BIGSERIAL PRIMARY KEY,
    name              VARCHAR(128) NOT NULL,
    email             VARCHAR(255) NOT NULL,
    password_hash     VARCHAR(255) NOT NULL,
    -- 该供货商授权的账号只能进入这些分组(服务端强制,供货商不可自选任意分组)
    allowed_group_ids BIGINT[]     NOT NULL DEFAULT '{}',
    -- 每日新增账户上限;0 = 用 config provider_portal.default_daily_add_limit
    daily_add_limit   INTEGER      NOT NULL DEFAULT 0,
    enabled           BOOLEAN      NOT NULL DEFAULT TRUE,
    notes             TEXT,
    last_login_at     TIMESTAMPTZ,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- 邮箱大小写不敏感唯一
CREATE UNIQUE INDEX IF NOT EXISTS idx_provider_accounts_email_lower ON provider_accounts (lower(email));
