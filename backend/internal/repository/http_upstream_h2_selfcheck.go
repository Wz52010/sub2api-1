package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// buildProbeRoundTripper 按当前配置与 Profile 选出与网关出站一致的指纹 transport(直连,无代理),
// 返回 RoundTripper 及其 protocolMode(fp_h2_fp / fp_h2 / default),供 ProbeFingerprintAgainstEcho 复用。
func (s *httpUpstreamService) buildProbeRoundTripper(profile *tlsfingerprint.Profile) (http.RoundTripper, string, error) {
	if profileALPNOffersH2(profile) && fingerprintH2Supported(nil) {
		if s.tlsFingerprintHTTP2FrameEnabled() {
			rt, err := buildFingerprintH2FrameTransport(poolSettings{}, nil, profile)
			return rt, upstreamProtocolModeFingerprintH2Frame, err
		}
		if s.tlsFingerprintHTTP2Enabled() {
			rt, err := buildFingerprintHTTP2Transport(poolSettings{}, nil, profile)
			return rt, upstreamProtocolModeFingerprintH2, err
		}
	}
	rt, err := buildUpstreamTransportWithTLSFingerprint(poolSettings{}, nil, profile)
	return rt, upstreamProtocolModeDefault, err
}

// ProbeFingerprintAgainstEcho 用该 Profile 对应的真实指纹 transport 直连指纹回显端点,
// 回传实测 JA3/JA4/H2 + 该 Profile 的目标 H2 串 + 是否字节一致。仅诊断,不参与转发。
// 通过类型断言从 service 层调用(不进 HTTPUpstream 接口,避免影响测试替身)。
func (s *httpUpstreamService) ProbeFingerprintAgainstEcho(ctx context.Context, profile *tlsfingerprint.Profile, echoURL string) (*service.FingerprintProbeResult, error) {
	if echoURL == "" {
		echoURL = DefaultH2FingerprintEchoURL
	}
	rt, protoMode, err := s.buildProbeRoundTripper(profile)
	if err != nil {
		return nil, fmt.Errorf("build probe transport: %w", err)
	}
	if closer, ok := rt.(interface{ CloseIdleConnections() }); ok {
		defer closer.CloseIdleConnections()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, echoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "*/*")
	resp, err := rt.RoundTrip(req)
	if err != nil {
		return nil, fmt.Errorf("probe roundtrip %s: %w", echoURL, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var pr struct {
		HTTPVersion string `json:"http_version"`
		TLS         struct {
			JA3           string `json:"ja3"`
			JA3Hash       string `json:"ja3_hash"`
			JA4           string `json:"ja4"`
			PeetPrintHash string `json:"peetprint_hash"`
		} `json:"tls"`
		HTTP2 struct {
			AkamaiFingerprint     string `json:"akamai_fingerprint"`
			AkamaiFingerprintHash string `json:"akamai_fingerprint_hash"`
		} `json:"http2"`
	}
	if err := json.Unmarshal(body, &pr); err != nil {
		return nil, fmt.Errorf("decode echo json: %w (body=%.160s)", err, string(body))
	}
	expected := ExpectedProfileH2Akamai(profile)
	name := ""
	if profile != nil {
		name = profile.Name
	}
	return &service.FingerprintProbeResult{
		EchoURL:          echoURL,
		ProfileName:      name,
		ProtocolMode:     protoMode,
		HTTPVersion:      pr.HTTPVersion,
		JA3:              pr.TLS.JA3,
		JA3Hash:          pr.TLS.JA3Hash,
		JA4:              pr.TLS.JA4,
		PeetPrintHash:    pr.TLS.PeetPrintHash,
		H2Akamai:         pr.HTTP2.AkamaiFingerprint,
		H2AkamaiHash:     pr.HTTP2.AkamaiFingerprintHash,
		ExpectedH2Akamai: expected,
		H2Match:          expected != "" && pr.HTTP2.AkamaiFingerprint == expected,
	}, nil
}

// DefaultH2FingerprintEchoURL 是默认的 H2 指纹回显端点(返回 http2.akamai_fingerprint)。
const DefaultH2FingerprintEchoURL = "https://tls.peet.ws/api/all"

// VerifyProfileH2Akamai 用给定 Profile 的 H2 帧级指纹(经真实 net/http→fhttp 适配器)直连
// 回显端点,返回对端观测到的 Akamai H2 指纹串。
//
// 这是"同步/漂移"守卫的核心:
//   - 抓真实客户端(如 codex_cli_rs)在 tls.peet.ws 的 akamai_fingerprint → 写入 Profile.H2*;
//     调用本函数应返回同一串(字节级),否则说明配置或依赖(fhttp/utls)有偏差。
//   - 依赖升级(fhttp/bogdanfinn-utls/Go)后跑一遍,若观测串变了即为"我方漂移",及时发现。
//
// 仅用于诊断/校验,不参与正常转发路径;直连(无代理)构建指纹 transport。
func VerifyProfileH2Akamai(ctx context.Context, profile *tlsfingerprint.Profile, echoURL string) (string, error) {
	if echoURL == "" {
		echoURL = DefaultH2FingerprintEchoURL
	}
	rt, err := buildFingerprintH2FrameTransport(poolSettings{}, nil, profile)
	if err != nil {
		return "", fmt.Errorf("build h2-frame transport: %w", err)
	}
	if closer, ok := rt.(interface{ CloseIdleConnections() }); ok {
		defer closer.CloseIdleConnections()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, echoURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "*/*")
	resp, err := rt.RoundTrip(req)
	if err != nil {
		return "", fmt.Errorf("roundtrip %s: %w", echoURL, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var parsed struct {
		HTTP2 struct {
			AkamaiFingerprint string `json:"akamai_fingerprint"`
		} `json:"http2"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("decode echo json: %w (body=%.160s)", err, string(body))
	}
	return parsed.HTTP2.AkamaiFingerprint, nil
}

// ExpectedProfileH2Akamai 返回该 Profile 配置的目标 Akamai 串(不发起网络)。
// 与 VerifyProfileH2Akamai 的返回比较即得"配置 vs 实际发出"是否一致。
func ExpectedProfileH2Akamai(profile *tlsfingerprint.Profile) string {
	return h2SpecFromProfile(profile).akamaiString()
}
