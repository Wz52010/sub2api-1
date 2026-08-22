-- 200_calibrate_codex_signature_algorithms.sql
-- 精修 Codex(rustls/reqwest)profile 的 signature_algorithms,补齐 JA4 第三段,使 JA4 完全对齐真实客户端。
--
-- 背景: 199 已把 Codex 的 JA3(cipher/曲线/扩展)与 H2 字节级校准,但 signature_algorithms 当时是占位值
--       [1027,1283,1539,2055,2052,2053,2054,1025,1281,1537](升序),其 JA4 第三段哈希 = 2b9fad200144,
--       与真实目标 f9531d972513 不符。
--
-- 真实值恢复: JA3/JA4 均不含 sigalgs 明文(JA4 第三段是 sha256(排序扩展 + 顺序 sigalgs) 的前 12 位哈希,不可逆)。
--       对该哈希做爆破:先用真实 cipher 段自证 JA4 算法(算出 JA4_b=61a7ad8aa9b6,与抓包一致),
--       再枚举 rustls 常见 sigalgs 排列,命中 JA4 第三段 f9531d972513,得到真实顺序
--       0503,0403,0603,0807,0806,0805,0804,0601,0501,0401(即 rustls/aws-lc-rs 标准序:
--       ecdsa384/256/521、ed25519、rsa_pss 512/384/256、rsa_pkcs1 512/384/256)。
--       完整 JA4 复现 t13d1011h2_61a7ad8aa9b6_f9531d972513,与 codex-cli 0.148.0-alpha.15 / macOS 抓包逐字符一致。
--       verify 端点实测: JA4 恒定命中、H2 match=True、JA3 每连接变(shuffle 生效)。
--
-- 幂等: 仅当当前值仍是 199 的占位序时改写(jsonb 相等比较忽略空白);改写后再执行为空操作,
--       不覆盖后续人工/API 编辑。按值定位而非按名字,故 profile 重命名不影响。
UPDATE tls_fingerprint_profiles
SET signature_algorithms = '[1283,1027,1539,2055,2054,2053,2052,1537,1281,1025]'::jsonb
WHERE signature_algorithms = '[1027,1283,1539,2055,2052,2053,2054,1025,1281,1537]'::jsonb;
