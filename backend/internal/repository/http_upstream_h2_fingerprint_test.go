package repository

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// 档2 HTTP/2 帧级指纹：spec 选择、Akamai 串、Transport 构建与开关分支单测。
// 只构建 Transport，不发起拨号/网络。

func TestSelectH2Fingerprint(t *testing.T) {
	require.Equal(t, "undici", selectH2Fingerprint(nil).Name, "nil 回退 undici")
	require.Equal(t, "undici", selectH2Fingerprint(&tlsfingerprint.Profile{Name: "Claude Code - Node.js 24.x"}).Name)
	require.Equal(t, "reqwest", selectH2Fingerprint(&tlsfingerprint.Profile{Name: "Codex CLI - Node.js 24.x"}).Name, "真实 Codex 为 Rust reqwest")
	require.Equal(t, "reqwest", selectH2Fingerprint(&tlsfingerprint.Profile{Name: "my codex profile"}).Name)
}

// akamaiString 必须与探针(cmd/fpprobe)对 tls.peet.ws 实测确认过的目标串一致。
func TestH2FingerprintAkamaiString(t *testing.T) {
	require.Equal(t, "1:65536;2:0;4:6291456;6:262144|15663105|0|m,a,s,p", h2SpecUndici.akamaiString())
	require.Equal(t, "2:0;4:2097152;5:16384;6:16384|5177345|0|m,s,a,p", h2SpecReqwest.akamaiString())
}

func TestBuildFingerprintH2FrameTransport(t *testing.T) {
	profile := &tlsfingerprint.Profile{Name: "Claude Code - Node.js 24.x", ALPNProtocols: []string{"h2", "http/1.1"}}

	rt, err := buildFingerprintH2FrameTransport(poolSettings{}, nil, profile)
	require.NoError(t, err)
	require.NotNil(t, rt)
	fr, ok := rt.(*h2FrameRoundTripper)
	require.True(t, ok, "必须是 *h2FrameRoundTripper")
	require.Equal(t, "undici", fr.spec.Name)
	require.NotNil(t, fr.tr.DialTLS, "必须挂 utls 拨号器")
	require.Equal(t, uint32(15663105), fr.tr.ConnectionFlow)
	require.Equal(t, []string{":method", ":authority", ":scheme", ":path"}, fr.tr.PseudoHeaderOrder)

	// http 代理支持。
	httpProxy, err := url.Parse("http://127.0.0.1:8080")
	require.NoError(t, err)
	_, err = buildFingerprintH2FrameTransport(poolSettings{}, httpProxy, profile)
	require.NoError(t, err)

	// https 代理不支持指纹拨号，必须报错以触发 h1 回退。
	httpsProxy, err := url.Parse("https://127.0.0.1:8443")
	require.NoError(t, err)
	_, err = buildFingerprintH2FrameTransport(poolSettings{}, httpsProxy, profile)
	require.Error(t, err)
}

// 档2 开启 + ALPN 含 h2 + 直连：指纹链路应改用 fhttp 适配器，protocolMode=fp_h2_fp。
func (s *HTTPUpstreamSuite) TestFingerprintH2FrameEnabledUsesFhttp() {
	s.cfg.Gateway = config.GatewayConfig{
		TLSFingerprint: config.TLSFingerprintConfig{HTTP2FrameFingerprintEnabled: true},
	}
	svc := s.newService()
	entry, err := svc.getClientEntryWithTLS("", 1, 1, s.fingerprintProfile([]string{"h2", "http/1.1"}), service.HTTPUpstreamProfileDefault, false, false)
	require.NoError(s.T(), err)
	_, ok := entry.client.Transport.(*h2FrameRoundTripper)
	require.True(s.T(), ok, "档2 开启必须是 *h2FrameRoundTripper")
	require.Equal(s.T(), upstreamProtocolModeFingerprintH2Frame, entry.protocolMode)
}

// 同时开启档1 与档2：档2 优先。
func (s *HTTPUpstreamSuite) TestFingerprintH2FrameTakesPrecedenceOverDang1() {
	s.cfg.Gateway = config.GatewayConfig{
		TLSFingerprint: config.TLSFingerprintConfig{HTTP2Enabled: true, HTTP2FrameFingerprintEnabled: true},
	}
	svc := s.newService()
	entry, err := svc.getClientEntryWithTLS("", 1, 1, s.fingerprintProfile([]string{"h2", "http/1.1"}), service.HTTPUpstreamProfileDefault, false, false)
	require.NoError(s.T(), err)
	_, ok := entry.client.Transport.(*h2FrameRoundTripper)
	require.True(s.T(), ok)
	require.Equal(s.T(), upstreamProtocolModeFingerprintH2Frame, entry.protocolMode, "档2 优先于档1")
}

// 档2 开启但仅 http/1.1 Profile：不得升级。
func (s *HTTPUpstreamSuite) TestFingerprintH2FrameProfileHTTP1OnlyStaysHTTP1() {
	s.cfg.Gateway = config.GatewayConfig{
		TLSFingerprint: config.TLSFingerprintConfig{HTTP2FrameFingerprintEnabled: true},
	}
	svc := s.newService()
	entry, err := svc.getClientEntryWithTLS("", 1, 1, s.fingerprintProfile([]string{"http/1.1"}), service.HTTPUpstreamProfileDefault, false, false)
	require.NoError(s.T(), err)
	require.Equal(s.T(), upstreamProtocolModeDefault, entry.protocolMode)
}

// 档2 开启 + h2 Profile，但经 https 代理(不支持指纹拨号)：必须回退 h1。
func (s *HTTPUpstreamSuite) TestFingerprintH2FrameHTTPSProxyFallsBackHTTP1() {
	s.cfg.Gateway = config.GatewayConfig{
		TLSFingerprint: config.TLSFingerprintConfig{HTTP2FrameFingerprintEnabled: true},
	}
	svc := s.newService()
	entry, err := svc.getClientEntryWithTLS("https://127.0.0.1:8443", 1, 1, s.fingerprintProfile([]string{"h2", "http/1.1"}), service.HTTPUpstreamProfileDefault, false, false)
	require.NoError(s.T(), err)
	require.Equal(s.T(), upstreamProtocolModeDefault, entry.protocolMode, "https 代理不支持指纹 h2，必须回退 default")
}
