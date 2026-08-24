-- 新增 OS 标签的 Node profile,支撑"Mac / Windows 用户"多样化。
-- 复用已校准的真实 TLS 字节(Node 指纹与 OS 无关),仅改名称——名称里的 OS 由
-- identity_os_sync 解析成出站 X-Stainless-OS/Arch(需 gateway.tls_fingerprint.identity_os_sync=true)。
-- 现有已覆盖:Node22/24 的 Linux x64(id7)、macOS arm64(id8);此处补 Windows,及 Node18/20 的 Win/Mac。
-- 幂等:NOT EXISTS 守卫。编号 229,排在 228 之后。

-- Node 22/24 带 → Windows x64(复制 id=5 的已校准字节)
INSERT INTO tls_fingerprint_profiles
  (name, description, enable_grease, cipher_suites, curves, point_formats, signature_algorithms,
   alpn_protocols, supported_versions, key_share_groups, psk_modes, extensions, shuffle_extensions,
   h2_source, created_at, updated_at)
SELECT
  'Claude Code - Node.js 24.x Windows x64',
  'Claude Code / Node.js 22-24 (undici, HTTP/1.1) presenting as Windows x64. TLS bytes = real Node22/24 capture; OS via identity_os_sync.',
  enable_grease, cipher_suites, curves, point_formats, signature_algorithms,
  alpn_protocols, supported_versions, key_share_groups, psk_modes, extensions, shuffle_extensions,
  'node 22/24 undici (tls.peet.ws capture); OS label=Windows x64', NOW(), NOW()
FROM tls_fingerprint_profiles WHERE id = 5
  AND NOT EXISTS (SELECT 1 FROM tls_fingerprint_profiles WHERE name = 'Claude Code - Node.js 24.x Windows x64');

-- Node 18/20 带 → Windows x64(复制 Node18/20 profile 的字节)
INSERT INTO tls_fingerprint_profiles
  (name, description, enable_grease, cipher_suites, curves, point_formats, signature_algorithms,
   alpn_protocols, supported_versions, key_share_groups, psk_modes, extensions, shuffle_extensions,
   h2_source, created_at, updated_at)
SELECT
  'Claude Code - Node.js 20.x Windows x64',
  'Claude Code / Node.js 18-20 (undici, HTTP/1.1) presenting as Windows x64. TLS bytes = real Node18/20 capture; OS via identity_os_sync.',
  enable_grease, cipher_suites, curves, point_formats, signature_algorithms,
  alpn_protocols, supported_versions, key_share_groups, psk_modes, extensions, shuffle_extensions,
  'node 18/20 undici (tls.peet.ws capture); OS label=Windows x64', NOW(), NOW()
FROM tls_fingerprint_profiles WHERE name = 'Claude Code - Node.js 20.x'
  AND NOT EXISTS (SELECT 1 FROM tls_fingerprint_profiles WHERE name = 'Claude Code - Node.js 20.x Windows x64');

-- Node 18/20 带 → macOS arm64
INSERT INTO tls_fingerprint_profiles
  (name, description, enable_grease, cipher_suites, curves, point_formats, signature_algorithms,
   alpn_protocols, supported_versions, key_share_groups, psk_modes, extensions, shuffle_extensions,
   h2_source, created_at, updated_at)
SELECT
  'Claude Code - Node.js 20.x macOS arm64',
  'Claude Code / Node.js 18-20 (undici, HTTP/1.1) presenting as macOS arm64. TLS bytes = real Node18/20 capture; OS via identity_os_sync.',
  enable_grease, cipher_suites, curves, point_formats, signature_algorithms,
  alpn_protocols, supported_versions, key_share_groups, psk_modes, extensions, shuffle_extensions,
  'node 18/20 undici (tls.peet.ws capture); OS label=macOS arm64', NOW(), NOW()
FROM tls_fingerprint_profiles WHERE name = 'Claude Code - Node.js 20.x'
  AND NOT EXISTS (SELECT 1 FROM tls_fingerprint_profiles WHERE name = 'Claude Code - Node.js 20.x macOS arm64');
