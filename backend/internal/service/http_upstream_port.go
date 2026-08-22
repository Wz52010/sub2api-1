package service

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

// HTTPUpstream 上游 HTTP 请求接口
// 用于向上游 API（Claude、OpenAI、Gemini 等）发送请求
type HTTPUpstream interface {
	// Do 执行 HTTP 请求（不启用 TLS 指纹）
	Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error)

	// DoWithTLS 执行带 TLS 指纹伪装的 HTTP 请求
	//
	// profile 参数:
	//   - nil: 不启用 TLS 指纹，行为与 Do 方法相同
	//   - non-nil: 使用指定的 Profile 进行 TLS 指纹伪装
	//
	// Profile 由调用方通过 TLSFingerprintProfileService 解析后传入，
	// 支持按账号绑定的数据库 profile 或内置默认 profile。
	DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error)
}

// FingerprintProbeResult 是对指纹回显端点(如 tls.peet.ws)实测到的出站指纹快照。
// 用它校验"部署实际发出的 JA3/JA4/H2"是否与账号所绑 Profile 的目标一致——既做上线前
// 校准确认,也做依赖(fhttp/utls/Go)升级后的漂移守卫。只读、诊断用途,不改任何转发行为。
type FingerprintProbeResult struct {
	EchoURL          string `json:"echo_url"`
	ProfileName      string `json:"profile_name"`
	ProtocolMode     string `json:"protocol_mode"`
	HTTPVersion      string `json:"http_version"`
	JA3              string `json:"ja3"`
	JA3Hash          string `json:"ja3_hash"`
	JA4              string `json:"ja4"`
	PeetPrintHash    string `json:"peetprint_hash"`
	H2Akamai         string `json:"h2_akamai_fingerprint"`
	H2AkamaiHash     string `json:"h2_akamai_fingerprint_hash"`
	ExpectedH2Akamai string `json:"expected_h2_akamai"`
	H2Match          bool   `json:"h2_match"`
}

// HTTPUpstreamHealth is an optional read-only capability implemented by the
// shared upstream transport. Keeping it separate from HTTPUpstream preserves
// the lightweight interface used by gateway test doubles.
type HTTPUpstreamHealth interface {
	SnapshotTransportHealth(accountID int64) AccountTransportHealthSnapshot
}
