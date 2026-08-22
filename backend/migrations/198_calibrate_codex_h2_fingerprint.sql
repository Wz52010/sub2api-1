-- 用真实 codex-cli 抓包校准 Codex(reqwest) 的 H2 帧级指纹。
-- 来源: codex-cli 0.148.0-alpha.15 / macOS 15.5 / arm64,tls.peet.ws 采样(H2 Akamai 稳定)。
-- H2 Akamai: 2:0;4:2097152;5:16384;6:16384|5177345|0|m,s,a,p
--
-- 仅更新仍为 197 未校准种子(h2_source LIKE 'seed reqwest%')的行,不覆盖后续人工/API 校准。
-- 幂等:重复执行时若已被校准则 WHERE 不命中,无副作用。
UPDATE tls_fingerprint_profiles SET
    h2_settings = '[[2,0],[4,2097152],[5,16384],[6,16384]]'::jsonb,
    h2_connection_flow = 5177345,
    h2_pseudo_header_order = '[":method",":scheme",":authority",":path"]'::jsonb,
    h2_akamai_expected = '2:0;4:2097152;5:16384;6:16384|5177345|0|m,s,a,p',
    h2_source = 'codex-cli 0.148.0-alpha.15 / macOS 15.5 arm64 (tls.peet.ws capture)'
WHERE name = 'Codex CLI - Node.js 24.x' AND h2_source LIKE 'seed reqwest%';
