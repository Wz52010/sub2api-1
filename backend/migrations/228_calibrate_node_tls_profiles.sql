-- 校准 Claude Code / Node.js TLS profile 为真实抓包指纹。
-- 来源: tls.peet.ws 抓包, macOS arm64, 2026-08-24, Node 内置 fetch(undici)。
-- 关键事实: undici 走 HTTP/1.1(ALPN 仅 http/1.1), 无 GREASE, 扩展顺序固定(不 shuffle)。
-- 两个真实指纹带:
--   Node 22/24: JA3 d67b094811e5145139d7cea5f014309f  JA4 t13d5212h1_b262b3658495_8e6e362c5eac
--   Node 18/20: JA3 1a28e69016765d92e3b381168d68922c  JA4 t13d5911h1_a33745022dd6_1f22a2ca17c4
-- H2 列保持空: undici 是 h1, 无 H2 帧指纹(H2 仅 Codex/reqwest 用)。
-- 编号 228, 排在指纹迁移 222-227 之后; 如与你迁移撞号改更大编号。

-- 现有 profile 5/7/8 都是 Node 22+, 用 {22,24} 带真值替换(此前是未校准种子, 仅 17 个 cipher)。
UPDATE tls_fingerprint_profiles SET
  cipher_suites='[4866,4867,4865,49199,49195,49200,49196,158,49191,103,49192,107,163,159,52393,52392,52394,49325,49311,49245,49249,49239,49235,162,49324,49310,49244,49248,49238,49234,49188,106,49187,64,49162,49172,57,56,49161,49171,51,50,157,49309,49233,156,49308,49232,61,60,53,47]'::jsonb,
  extensions='[65281,0,11,10,35,16,22,23,13,43,45,51]'::jsonb,
  curves='[4588,29,23,30,24,25,256,257]'::jsonb,
  point_formats='[0,1,2]'::jsonb,
  signature_algorithms='[2309,2310,2308,1027,1283,1539,2055,2056,2074,2075,2076,2057,2058,2059,2052,2053,2054,1025,1281,1537,771,769,770,1026,1282,1538]'::jsonb,
  key_share_groups='[4588,29]'::jsonb,
  supported_versions='[772,771]'::jsonb,
  psk_modes='[1]'::jsonb,
  alpn_protocols='["http/1.1"]'::jsonb,
  enable_grease=false,
  shuffle_extensions=false,
  h2_source='node 22/24 undici (tls.peet.ws capture, macOS arm64, 2026-08-24)',
  updated_at=NOW()
WHERE id IN (5, 7, 8);

-- 新增 Node 18/20 profile({18,20} 带), 提供第二个真实 JA3, 供账号多样化分散。
INSERT INTO tls_fingerprint_profiles
  (name, description, enable_grease, cipher_suites, curves, point_formats,
   signature_algorithms, alpn_protocols, supported_versions, key_share_groups,
   psk_modes, extensions, shuffle_extensions, h2_source, created_at, updated_at)
SELECT
  'Claude Code - Node.js 20.x',
  'Claude Code / Node.js 18-20 runtime (undici, HTTP/1.1). Real capture tls.peet.ws macOS arm64 2026-08-24. JA4 t13d5911h1_a33745022dd6_1f22a2ca17c4.',
  false,
  '[4866,4867,4865,49199,49195,49200,49196,158,49191,103,49192,107,163,159,52393,52392,52394,49327,49325,49315,49311,49245,49249,49239,49235,162,49326,49324,49314,49310,49244,49248,49238,49234,49188,106,49187,64,49162,49172,57,56,49161,49171,51,50,157,49313,49309,49233,156,49312,49308,49232,61,60,53,47,255]'::jsonb, '[29,23,30,25,24,256,257,258,259,260]'::jsonb, '[0,1,2]'::jsonb,
  '[1027,1283,1539,2055,2056,2057,2058,2059,2052,2053,2054,1025,1281,1537,771,769,770,1026,1282,1538]'::jsonb, '["http/1.1"]'::jsonb, '[772,771]'::jsonb,
  '[29]'::jsonb, '[1]'::jsonb, '[0,11,10,35,16,22,23,13,43,45,51]'::jsonb,
  false, 'node 18/20 undici (tls.peet.ws capture, macOS arm64, 2026-08-24)', NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM tls_fingerprint_profiles WHERE name='Claude Code - Node.js 20.x');
