-- Seed editable TLS transport compatibility profiles.
--
-- These profiles describe the client runtime transport shape, not a model
-- identity or an official authentication signal. Claude Code and Codex model
-- variants normally share the TLS stack of their client runtime, so the
-- descriptions intentionally cover all models in each client family.

INSERT INTO tls_fingerprint_profiles (
    name,
    description,
    enable_grease,
    cipher_suites,
    curves,
    point_formats,
    signature_algorithms,
    alpn_protocols,
    supported_versions,
    key_share_groups,
    psk_modes,
    extensions
)
VALUES
(
    'Claude Code - Node.js 24.x',
    'Claude Code transport baseline for all Claude model variants (Opus, Sonnet, Haiku and future models). TLS profile follows the Node.js 24.x runtime; model selection does not change ClientHello.',
    false,
    '[4865,4866,4867,49195,49199,49196,49200,52393,52392,49161,49171,49162,49172,156,157,47,53]'::jsonb,
    '[29,23,24]'::jsonb,
    '[0]'::jsonb,
    '[1027,2052,1025,1283,2053,1281,2054,1537,513]'::jsonb,
    '["http/1.1"]'::jsonb,
    '[772,771]'::jsonb,
    '[29]'::jsonb,
    '[1]'::jsonb,
    '[0,65037,23,65281,10,11,35,16,5,13,18,51,45,43]'::jsonb
),
(
    'Codex CLI - Node.js 24.x',
    'Codex CLI transport baseline for all Codex model variants. TLS profile follows the Node.js 24.x runtime; GPT-5.x/Codex model selection does not change ClientHello.',
    false,
    '[4865,4866,4867,49195,49199,49196,49200,52393,52392,49161,49171,49162,49172,156,157,47,53]'::jsonb,
    '[29,23,24]'::jsonb,
    '[0]'::jsonb,
    '[1027,2052,1025,1283,2053,1281,2054,1537,513]'::jsonb,
    '["http/1.1"]'::jsonb,
    '[772,771]'::jsonb,
    '[29]'::jsonb,
    '[1]'::jsonb,
    '[0,65037,23,65281,10,11,35,16,5,13,18,51,45,43]'::jsonb
),
(
    'Claude/Codex shared - Node.js 22.17.1 Linux x64',
    'Shared Linux x64 Node.js 22.17.1 transport profile for Claude Code and Codex CLI. Use for all model variants when the client runtime is Node.js 22.17.1 on Linux x64.',
    false,
    '[4866,4867,4865,49199,49195,49200,49196,158,49191,103,49192,107,163,159,52393,52392,52394,49327,49325,49315,49311,49245,49249,49239,49235,162,49326,49324,49314,49310,49244,49248,49238,49234,49188,106,49187,64,49162,49172,57,56,49161,49171,51,50,157,49313,49309,49233,156,49312,49308,49232,61,60,53,47,255]'::jsonb,
    '[29,23,30,25,24,256,257,258,259,260]'::jsonb,
    '[0,1,2]'::jsonb,
    '[1027,2052,1025,1283,2053,1281,2054,1537,513]'::jsonb,
    '["http/1.1"]'::jsonb,
    '[772,771]'::jsonb,
    '[29]'::jsonb,
    '[1]'::jsonb,
    '[0,11,10,35,16,22,23,13,43,45,51]'::jsonb
),
(
    'Claude/Codex shared - Node.js 24.3.0 macOS arm64',
    'Shared macOS arm64 Node.js 24.3.0 transport profile for Claude Code and Codex CLI. Use for all model variants when the client runtime is Node.js 24.3.0 on macOS arm64.',
    false,
    '[4865,4866,4867,49195,49199,49196,49200,52393,52392,49161,49171,49162,49172,156,157,47,53]'::jsonb,
    '[29,23,24]'::jsonb,
    '[0]'::jsonb,
    '[1027,2052,1025,1283,2053,1281,2054,1537,513]'::jsonb,
    '["http/1.1"]'::jsonb,
    '[772,771]'::jsonb,
    '[29]'::jsonb,
    '[1]'::jsonb,
    '[0,65037,23,65281,10,11,35,16,5,13,18,51,45,43]'::jsonb
)
ON CONFLICT (name) DO NOTHING;
