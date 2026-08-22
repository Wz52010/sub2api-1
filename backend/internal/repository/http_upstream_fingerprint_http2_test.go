package repository

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// 档1 HTTP/2 指纹：纯函数与开关分支单测。这些用例只构建 Transport，不发起拨号/网络。

func TestProfileALPNOffersH2(t *testing.T) {
	cases := []struct {
		name string
		alpn []string
		want bool
	}{
		{"nil profile alpn", nil, false},
		{"http1 only", []string{"http/1.1"}, false},
		{"h2 first", []string{"h2", "http/1.1"}, true},
		{"h2 only", []string{"h2"}, true},
		{"mixed case with spaces", []string{" H2 "}, true},
		{"unrelated", []string{"spdy/3.1"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := profileALPNOffersH2(&tlsfingerprint.Profile{ALPNProtocols: c.alpn})
			require.Equal(t, c.want, got)
		})
	}
	require.False(t, profileALPNOffersH2(nil), "nil profile must not offer h2")
}

func TestFingerprintH2Supported(t *testing.T) {
	mustURL := func(raw string) *url.URL {
		u, err := url.Parse(raw)
		require.NoError(t, err)
		return u
	}
	require.True(t, fingerprintH2Supported(nil), "direct connection supports fingerprint h2")
	require.True(t, fingerprintH2Supported(mustURL("http://127.0.0.1:8080")))
	require.True(t, fingerprintH2Supported(mustURL("socks5://127.0.0.1:1080")))
	require.True(t, fingerprintH2Supported(mustURL("socks5h://127.0.0.1:1080")))
	// https 代理无法用明文 CONNECT 前导；未知类型不套指纹。
	require.False(t, fingerprintH2Supported(mustURL("https://127.0.0.1:8443")))
	require.False(t, fingerprintH2Supported(mustURL("quic://127.0.0.1:443")))
}

func TestBuildFingerprintHTTP2Transport(t *testing.T) {
	profile := &tlsfingerprint.Profile{Name: "test", ALPNProtocols: []string{"h2", "http/1.1"}}

	// 直连：应返回带健康 PING 的 *http2.Transport。
	tr, err := buildFingerprintHTTP2Transport(poolSettings{}, nil, profile)
	require.NoError(t, err)
	require.NotNil(t, tr)
	require.IsType(t, &http2.Transport{}, tr)
	require.Equal(t, openAIHTTP2ReadIdleTimeout, tr.ReadIdleTimeout, "必须启用空闲 PING 探测以剔除死连接")
	require.Equal(t, openAIHTTP2PingTimeout, tr.PingTimeout)
	require.NotNil(t, tr.DialTLSContext, "必须挂 utls 拨号器")
	require.False(t, tr.AllowHTTP, "指纹 h2 只走 TLS，不允许明文 h2c")

	// http 代理(CONNECT 隧道)：支持。
	httpProxy, err := url.Parse("http://127.0.0.1:8080")
	require.NoError(t, err)
	trProxy, err := buildFingerprintHTTP2Transport(poolSettings{}, httpProxy, profile)
	require.NoError(t, err)
	require.NotNil(t, trProxy.DialTLSContext)

	// https 代理：不支持，应返回 error 让调用方回退。
	httpsProxy, err := url.Parse("https://127.0.0.1:8443")
	require.NoError(t, err)
	_, err = buildFingerprintHTTP2Transport(poolSettings{}, httpsProxy, profile)
	require.Error(t, err, "https 代理不支持指纹 h2，必须报错以触发 h1 回退")
}

// --- 开关分支：经 getClientEntryWithTLS 观察实际选用的 Transport 类型与 protocolMode ---

func (s *HTTPUpstreamSuite) fingerprintProfile(alpn []string) *tlsfingerprint.Profile {
	return &tlsfingerprint.Profile{Name: "Claude Code - Node.js 24.x", ALPNProtocols: alpn}
}

// 开关关闭时，即便 Profile 提供 h2 也必须维持 HTTP/1.1 指纹 Transport（默认行为，可回滚）。
func (s *HTTPUpstreamSuite) TestFingerprintHTTP2DisabledStaysHTTP1() {
	s.cfg.Gateway = config.GatewayConfig{
		TLSFingerprint: config.TLSFingerprintConfig{HTTP2Enabled: false},
	}
	svc := s.newService()
	entry, err := svc.getClientEntryWithTLS("", 1, 1, s.fingerprintProfile([]string{"h2", "http/1.1"}), service.HTTPUpstreamProfileDefault, false, false)
	require.NoError(s.T(), err)
	_, ok := entry.client.Transport.(*http.Transport)
	require.True(s.T(), ok, "开关关闭必须是 *http.Transport")
	require.Equal(s.T(), upstreamProtocolModeDefault, entry.protocolMode)
}

// 开关开启 + Profile ALPN 含 h2 + 直连：指纹链路应改用 *http2.Transport，protocolMode=fp_h2。
func (s *HTTPUpstreamSuite) TestFingerprintHTTP2EnabledUsesHTTP2Transport() {
	s.cfg.Gateway = config.GatewayConfig{
		TLSFingerprint: config.TLSFingerprintConfig{HTTP2Enabled: true},
	}
	svc := s.newService()
	entry, err := svc.getClientEntryWithTLS("", 1, 1, s.fingerprintProfile([]string{"h2", "http/1.1"}), service.HTTPUpstreamProfileDefault, false, false)
	require.NoError(s.T(), err)
	_, ok := entry.client.Transport.(*http2.Transport)
	require.True(s.T(), ok, "开关开启且 ALPN 含 h2 必须是 *http2.Transport")
	require.Equal(s.T(), upstreamProtocolModeFingerprintH2, entry.protocolMode)
}

// 开关开启但 Profile 仅 http/1.1：不得升级，维持 h1。
func (s *HTTPUpstreamSuite) TestFingerprintHTTP2EnabledButProfileHTTP1Only() {
	s.cfg.Gateway = config.GatewayConfig{
		TLSFingerprint: config.TLSFingerprintConfig{HTTP2Enabled: true},
	}
	svc := s.newService()
	entry, err := svc.getClientEntryWithTLS("", 1, 1, s.fingerprintProfile([]string{"http/1.1"}), service.HTTPUpstreamProfileDefault, false, false)
	require.NoError(s.T(), err)
	_, ok := entry.client.Transport.(*http.Transport)
	require.True(s.T(), ok, "Profile 未提供 h2 时必须维持 *http.Transport")
	require.Equal(s.T(), upstreamProtocolModeDefault, entry.protocolMode)
}

// 开关开启 + h2 Profile，但经 https 代理(不支持指纹拨号)：必须回退 h1，不误标 fp_h2。
func (s *HTTPUpstreamSuite) TestFingerprintHTTP2EnabledHTTPSProxyFallsBackHTTP1() {
	s.cfg.Gateway = config.GatewayConfig{
		TLSFingerprint: config.TLSFingerprintConfig{HTTP2Enabled: true},
	}
	svc := s.newService()
	entry, err := svc.getClientEntryWithTLS("https://127.0.0.1:8443", 1, 1, s.fingerprintProfile([]string{"h2", "http/1.1"}), service.HTTPUpstreamProfileDefault, false, false)
	require.NoError(s.T(), err)
	require.Equal(s.T(), upstreamProtocolModeDefault, entry.protocolMode, "https 代理不支持指纹 h2，必须回退 default")
}
