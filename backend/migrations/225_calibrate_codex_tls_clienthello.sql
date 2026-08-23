-- Codex(reqwest/rustls) TLS ClientHello 校准 + 每连接扩展随机化。
--
-- 1) 新增 shuffle_extensions 列(默认 false;仅 Codex 置 true 以模拟 rustls 的 JA3 每连接变化)。
-- 2) 把 "Codex CLI" Profile 的 ClientHello 从(错误的)Node 种子换成真实 rustls 形态。
--
-- 来源: codex-cli 0.148.0-alpha.15 / macOS 15.5 / arm64,tls.peet.ws 抓包。
-- 已字节级复现的 JA3(含 SCSV=0x00ff=255 与 X25519MLKEM768=4588):
--   771,4866-4865-4867-49196-49195-52393-49200-49199-52392-255,45-11-13-5-23-16-0-35-10-51-43,4588-29-23-24,0
-- JA4 目标 t13d1011h2_61a7ad8aa9b6_f9531d972513 的 cipher/扩展段已匹配。
--
-- 注意: signature_algorithms 为待定占位(JA3 不编码之;JA4 第三段哈希尚未匹配),待真实 ja4_r 精修;
--       扩展顺序 shuffle=true => JA3 每连接变、JA4 稳定(与 rustls 一致)。
-- 迁移仅执行一次(schema_migrations 记录),不会重复覆盖后续人工/API 编辑。

ALTER TABLE tls_fingerprint_profiles ADD COLUMN IF NOT EXISTS shuffle_extensions boolean NOT NULL DEFAULT false;

UPDATE tls_fingerprint_profiles SET
    cipher_suites        = '[4866,4865,4867,49196,49195,52393,49200,49199,52392,255]'::jsonb,
    curves               = '[4588,29,23,24]'::jsonb,
    point_formats        = '[0]'::jsonb,
    signature_algorithms = '[1027,1283,1539,2055,2052,2053,2054,1025,1281,1537]'::jsonb,
    alpn_protocols       = '["h2","http/1.1"]'::jsonb,
    supported_versions   = '[772,771]'::jsonb,
    key_share_groups     = '[4588,29]'::jsonb,
    psk_modes            = '[1]'::jsonb,
    extensions           = '[45,11,13,5,23,16,0,35,10,51,43]'::jsonb,
    enable_grease        = false,
    shuffle_extensions   = true
WHERE name = 'Codex CLI - Node.js 24.x';
