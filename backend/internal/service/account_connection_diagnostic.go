package service

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

const (
	accountDiagnosticTimeout = 12 * time.Second
	diagnosticIPURL          = "https://api.ipify.org?format=json"
	// AccountConnectionDiagnosticExtraKey is intentionally kept in accounts.extra
	// so this observability feature does not require a schema migration.
	AccountConnectionDiagnosticExtraKey = "connection_diagnostic"
	diagnosticPersistTimeout            = 2 * time.Second
	diagnosticMaxPersistedNotes         = 8
	diagnosticMaxPersistedText           = 512
)

// AccountConnectionDiagnostic is a read-only view of the account's outbound
// network path. It deliberately omits credentials and proxy authentication.
type AccountConnectionDiagnostic struct {
	AccountID             int64     `json:"account_id"`
	Platform              string    `json:"platform"`
	TargetHost            string    `json:"target_host"`
	ProxyConfigured       bool      `json:"proxy_configured"`
	ProxyID               *int64    `json:"proxy_id,omitempty"`
	ProxyEndpoint         string    `json:"proxy_endpoint,omitempty"`
	ProxyExitIP           string    `json:"proxy_exit_ip,omitempty"`
	ProxyExitIPStatus     string    `json:"proxy_exit_ip_status"`
	DNSStatus             string    `json:"dns_status"`
	DNSAddresses          []string  `json:"dns_addresses,omitempty"`
	TLSFingerprintEnabled bool      `json:"tls_fingerprint_enabled"`
	TLSProfileName        string    `json:"tls_profile_name,omitempty"`
	FingerprintKey        string    `json:"fingerprint_key,omitempty"`
	TLSHandshake          bool      `json:"tls_handshake"`
	TLSVersion            string    `json:"tls_version,omitempty"`
	ALPN                  string    `json:"alpn,omitempty"`
	HTTPProtocol          string    `json:"http_protocol,omitempty"`
	HTTPStatus            int       `json:"http_status,omitempty"`
	LatencyMs             int64     `json:"latency_ms,omitempty"`
	Success               bool      `json:"success"`
	FailureStage          string    `json:"failure_stage,omitempty"`
	FailureMessage        string    `json:"failure_message,omitempty"`
	Notes                 []string  `json:"notes,omitempty"`
	// CoherenceStatus / CoherenceFindings 为只读的出站身份一致性评估结果:对比"被模拟的
	// 客户端身份"与"实际协商到的协议/指纹"。完全基于已采集字段分析,不发起新请求,
	// 也不改变任何转发行为。
	CoherenceStatus       string                       `json:"coherence_status,omitempty"`
	CoherenceFindings     []ConnectionCoherenceFinding `json:"coherence_findings,omitempty"`
	CheckedAt             time.Time `json:"checked_at"`
}

// ConnectionCoherenceFinding 描述一条出站身份一致性发现(只读,不影响转发路径)。
type ConnectionCoherenceFinding struct {
	Code       string `json:"code"`
	Severity   string `json:"severity"` // info | warning
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

const (
	coherenceSeverityInfo    = "info"
	coherenceSeverityWarning = "warning"

	coherenceStatusNotChecked = "not_checked"
	coherenceStatusCoherent   = "coherent"
	coherenceStatusWarning    = "warning"
)

// DiagnoseAccountConnection checks the network path without sending account
// credentials or a model request. A provider response of 401/403/404 still
// proves that DNS, TCP, TLS and HTTP reached the target successfully.
func (s *AccountTestService) DiagnoseAccountConnection(parent context.Context, accountID int64) (*AccountConnectionDiagnostic, error) {
	if s == nil || s.accountRepo == nil || s.httpUpstream == nil {
		return nil, errors.New("account connection diagnostic is not configured")
	}

	account, err := s.accountRepo.GetByID(parent, accountID)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, fmt.Errorf("account %d not found", accountID)
	}

	targetURL, err := s.connectionDiagnosticTarget(account)
	if err != nil {
		return nil, err
	}
	target, err := url.Parse(targetURL)
	if err != nil {
		return nil, fmt.Errorf("invalid diagnostic target: %w", err)
	}

	ctx, cancel := context.WithTimeout(parent, accountDiagnosticTimeout)
	defer cancel()

	result := &AccountConnectionDiagnostic{
		AccountID:             account.ID,
		Platform:              account.Platform,
		TargetHost:            target.Hostname(),
		DNSStatus:             "not_checked",
		ProxyExitIPStatus:     "not_checked",
		TLSFingerprintEnabled: account.IsTLSFingerprintEnabled(),
		CheckedAt:             time.Now().UTC(),
		DNSAddresses:          []string{},
		Notes:                 []string{},
	}

	if account.Proxy != nil {
		result.ProxyConfigured = true
		result.ProxyID = account.ProxyID
		result.ProxyEndpoint = diagnosticProxyEndpoint(account.Proxy)
	}
	proxyURL := ""
	if account.Proxy != nil && account.ProxyID != nil {
		proxyURL = account.Proxy.URL()
	}

	var profile *tlsfingerprint.Profile
	if result.TLSFingerprintEnabled && s.tlsFPProfileService != nil {
		profile = s.tlsFPProfileService.ResolveTLSProfile(account)
		if profile != nil {
			result.TLSProfileName = profile.Name
			result.FingerprintKey = profile.FingerprintKey()
		}
	}

	lookupCtx, lookupCancel := context.WithTimeout(ctx, 3*time.Second)
	addresses, lookupErr := net.DefaultResolver.LookupIPAddr(lookupCtx, target.Hostname())
	lookupCancel()
	if lookupErr != nil {
		result.DNSStatus = "lookup_failed"
		result.Notes = append(result.Notes, "本机 DNS 查询失败；代理模式下目标域名也可能由代理端解析。")
	} else {
		result.DNSStatus = "local_resolved"
		result.DNSAddresses = make([]string, 0, len(addresses))
		for _, address := range addresses {
			result.DNSAddresses = append(result.DNSAddresses, address.IP.String())
		}
		sort.Strings(result.DNSAddresses)
		if result.ProxyConfigured {
			result.Notes = append(result.Notes, "已配置代理时，本机 DNS 结果不能证明目标域名最终由本机还是代理端解析。")
		}
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create diagnostic request: %w", err)
	}
	request.Header.Set("Accept", "*/*")
	request.Header.Set("User-Agent", "Sub2API-Connection-Diagnostic/1.0")
	request = request.WithContext(WithHTTPUpstreamRedirectsDisabled(request.Context()))

	started := time.Now()
	response, requestErr := s.httpUpstream.DoWithTLS(request, proxyURL, account.ID, account.Concurrency, profile)
	result.LatencyMs = time.Since(started).Milliseconds()
	if requestErr == nil && response == nil {
		requestErr = errors.New("upstream returned an empty response")
	}
	if requestErr != nil {
		result.FailureStage = diagnosticFailureStage(requestErr)
		result.FailureMessage = redactDiagnosticError(requestErr, proxyURL)
		// Keep the exit-IP probe independent from the provider target probe. A
		// provider-specific TLS or HTTP failure should not hide a working proxy.
		if ip, ipErr := s.probeDiagnosticExitIP(ctx, account, proxyURL, profile); ipErr != nil {
			result.ProxyExitIPStatus = "probe_failed"
			result.Notes = append(result.Notes, "出口 IP 探测失败："+redactDiagnosticError(ipErr, proxyURL))
		} else {
			result.ProxyExitIPStatus = "observed"
			result.ProxyExitIP = ip
			result.Notes = append(result.Notes, "目标请求失败，但代理出口仍可访问出口 IP 探测服务。")
		}
		return s.finishAccountConnectionDiagnostic(result), nil
	}
	if response.Body != nil {
		defer func() { _ = response.Body.Close() }()
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	}

	result.Success = true
	result.HTTPStatus = response.StatusCode
	result.HTTPProtocol = response.Proto
	if response.TLS != nil {
		result.TLSHandshake = response.TLS.HandshakeComplete
		result.TLSVersion = diagnosticTLSVersion(response.TLS.Version)
		result.ALPN = response.TLS.NegotiatedProtocol
		if result.ALPN == "" {
			result.ALPN = "none"
		}
	}
	if result.HTTPStatus >= 400 {
		result.Notes = append(result.Notes, fmt.Sprintf("目标返回 HTTP %d；这不等于代理或 TLS 失败。", result.HTTPStatus))
	}

	if ip, ipErr := s.probeDiagnosticExitIP(ctx, account, proxyURL, profile); ipErr != nil {
		result.ProxyExitIPStatus = "probe_failed"
		result.Notes = append(result.Notes, "出口 IP 探测失败："+redactDiagnosticError(ipErr, proxyURL))
	} else {
		result.ProxyExitIPStatus = "observed"
		result.ProxyExitIP = ip
	}

	return s.finishAccountConnectionDiagnostic(result), nil
}

// GetLastAccountConnectionDiagnostic returns the latest redacted diagnostic
// snapshot. A malformed or absent optional snapshot is treated as no result so
// an old account.extra value cannot block the account page.
func (s *AccountTestService) GetLastAccountConnectionDiagnostic(ctx context.Context, accountID int64) (*AccountConnectionDiagnostic, error) {
	if s == nil || s.accountRepo == nil {
		return nil, errors.New("account connection diagnostic is not configured")
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if account == nil || account.Extra == nil {
		return nil, nil
	}
	raw, ok := account.Extra[AccountConnectionDiagnosticExtraKey]
	if !ok || raw == nil {
		return nil, nil
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return nil, nil
	}
	var result AccountConnectionDiagnostic
	if err := json.Unmarshal(payload, &result); err != nil || result.CheckedAt.IsZero() {
		return nil, nil
	}
	if result.AccountID != 0 && result.AccountID != account.ID {
		return nil, nil
	}
	result.AccountID = account.ID
	return &result, nil
}

func (s *AccountTestService) finishAccountConnectionDiagnostic(result *AccountConnectionDiagnostic) *AccountConnectionDiagnostic {
	evaluateConnectionCoherence(result)
	if err := s.persistAccountConnectionDiagnostic(result); err != nil {
		// Persistence is best effort. The probe result remains useful even when
		// the account row cannot be updated at that moment.
		result.Notes = append(result.Notes, "本次诊断结果未能保存，下次打开窗口时可能看不到它。")
	}
	return result
}

func (s *AccountTestService) persistAccountConnectionDiagnostic(result *AccountConnectionDiagnostic) error {
	if s == nil || s.accountRepo == nil || result == nil {
		return nil
	}
	stored := *result
	stored.FailureMessage = truncateDiagnosticText(stored.FailureMessage)
	stored.Notes = make([]string, 0, minInt(len(result.Notes), diagnosticMaxPersistedNotes))
	for _, note := range result.Notes {
		stored.Notes = append(stored.Notes, truncateDiagnosticText(note))
		if len(stored.Notes) >= diagnosticMaxPersistedNotes {
			break
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), diagnosticPersistTimeout)
	defer cancel()
	return s.accountRepo.UpdateExtra(ctx, result.AccountID, map[string]any{
		AccountConnectionDiagnosticExtraKey: stored,
	})
}

func truncateDiagnosticText(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= diagnosticMaxPersistedText {
		return value
	}
	return string(runes[:diagnosticMaxPersistedText])
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func (s *AccountTestService) probeDiagnosticExitIP(ctx context.Context, account *Account, proxyURL string, profile *tlsfingerprint.Profile) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, diagnosticIPURL, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Sub2API-Connection-Diagnostic/1.0")
	response, err := s.httpUpstream.DoWithTLS(request, proxyURL, account.ID, account.Concurrency, profile)
	if err != nil {
		return "", err
	}
	if response == nil || response.Body == nil {
		return "", errors.New("exit IP endpoint returned an empty response")
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(response.Body, 4096))
	if err != nil {
		return "", err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("exit IP endpoint returned HTTP %d", response.StatusCode)
	}
	var payload struct {
		IP string `json:"ip"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || strings.TrimSpace(payload.IP) == "" {
		return "", errors.New("exit IP endpoint returned an invalid response")
	}
	return strings.TrimSpace(payload.IP), nil
}

func (s *AccountTestService) connectionDiagnosticTarget(account *Account) (string, error) {
	switch account.Platform {
	case PlatformAnthropic:
		return "https://api.anthropic.com/", nil
	case PlatformOpenAI:
		return "https://api.openai.com/", nil
	case PlatformGemini, PlatformAntigravity:
		return "https://generativelanguage.googleapis.com/", nil
	case PlatformGrok:
		return "https://api.x.ai/", nil
	}

	baseURL := strings.TrimSpace(account.GetCredential("base_url"))
	if baseURL == "" {
		return "", fmt.Errorf("platform %q has no safe diagnostic target", account.Platform)
	}
	normalized, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return "", fmt.Errorf("custom diagnostic target rejected: %w", err)
	}
	return strings.TrimRight(normalized, "/") + "/", nil
}

func diagnosticTLSVersion(version uint16) string {
	switch version {
	case tls.VersionTLS13:
		return "TLS 1.3"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS10:
		return "TLS 1.0"
	default:
		if version == 0 {
			return "unknown"
		}
		return fmt.Sprintf("0x%04x", version)
	}
}

// diagnosticProxyEndpoint returns the operator-useful part of a proxy address
// without including its username, password, path, or query string.
func diagnosticProxyEndpoint(proxy *Proxy) string {
	if proxy == nil {
		return ""
	}
	scheme := strings.TrimSpace(proxy.Protocol)
	host := strings.TrimSpace(proxy.Host)
	if scheme == "" || host == "" || proxy.Port <= 0 {
		return ""
	}
	return (&url.URL{
		Scheme: scheme,
		Host:   net.JoinHostPort(host, fmt.Sprintf("%d", proxy.Port)),
	}).String()
}

func diagnosticFailureStage(err error) string {
	if err == nil {
		return "unknown"
	}
	var networkErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &networkErr) && networkErr.Timeout()) {
		return "timeout"
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "dns"), strings.Contains(message, "no such host"):
		return "dns"
	case strings.Contains(message, "tls"), strings.Contains(message, "certificate"):
		return "tls"
	case strings.Contains(message, "proxy"), strings.Contains(message, "connect"):
		return "proxy_or_tcp"
	default:
		return "http_request"
	}
}

func redactDiagnosticError(err error, proxyURL string) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if proxyURL == "" {
		return message
	}

	// Errors from different transports do not use one consistent rendering:
	// some include the original proxy URL, while others include the parsed URL
	// with userinfo. Replace both forms before persisting or returning the text.
	message = strings.ReplaceAll(message, proxyURL, "[configured proxy]")
	if parsed, parseErr := url.Parse(proxyURL); parseErr == nil {
		if parsed.User != nil {
			message = strings.ReplaceAll(message, parsed.String(), "[configured proxy]")
			message = strings.ReplaceAll(message, parsed.Redacted(), "[configured proxy]")
			message = strings.ReplaceAll(message, parsed.User.String(), "[proxy credentials]")
		}
	}
	return message
}

// evaluateConnectionCoherence 在探测完成后,对已采集字段做只读一致性分析:对比"被模拟的
// 客户端身份"与"实际协商到的协议/指纹",把矛盾转成可读发现写入 result。
//
// 它不发起任何新请求,也不修改任何出站/转发行为——仅解释既有诊断数据。
func evaluateConnectionCoherence(result *AccountConnectionDiagnostic) {
	if result == nil {
		return
	}

	findings := make([]ConnectionCoherenceFinding, 0, 4)
	nodeLikeProfile := classifyProfileClientType(result.TLSProfileName) == profileClientNode

	// 1) 未启用 TLS 指纹:出站 ClientHello 使用 Go 运行时默认特征(非模拟客户端)。
	if !result.TLSFingerprintEnabled {
		findings = append(findings, ConnectionCoherenceFinding{
			Code:       "tls_fingerprint_disabled",
			Severity:   coherenceSeverityInfo,
			Message:    "未启用 TLS 指纹,出站 TLS ClientHello 使用 Go 运行时默认特征(JA3/JA4 非模拟客户端)。",
			Suggestion: "如需模拟 Claude Code / Node 客户端的握手特征,可为该账号绑定 TLS Profile 后再次诊断。",
		})
	}

	// 2) OpenAI/Codex 账号绑定 Node 指纹 —— 运行时错配。真实 Codex CLI 是 Rust 客户端。
	if result.Platform == PlatformOpenAI && result.TLSFingerprintEnabled && nodeLikeProfile {
		findings = append(findings, ConnectionCoherenceFinding{
			Code:       "codex_runtime_mismatch",
			Severity:   coherenceSeverityWarning,
			Message:    "账号平台为 OpenAI/Codex,但绑定的 TLS Profile 面向 Node.js。真实 Codex CLI 是 Rust 客户端(codex_cli_rs, reqwest/hyper),其 JA3/JA4 与 Node 不同。",
			Suggestion: "OpenAI 原生转发默认不套 utls 且走内置 HTTP/2 模式;为 Codex 账号套 Node 指纹通常增加不一致而非减少。除非确知转发路径会用到该 Profile,否则不建议启用。",
		})
	}

	// 3) 模拟 Node/Claude Code 却协商到 HTTP/1.1 —— 协议维度不一致(当前最主要的缺口)。
	if result.TLSFingerprintEnabled && nodeLikeProfile && result.Success {
		switch diagnosticNegotiatedHTTPMajor(result) {
		case 1:
			findings = append(findings, ConnectionCoherenceFinding{
				Code:       "protocol_h1_vs_node_client",
				Severity:   coherenceSeverityWarning,
				Message:    "TLS 指纹模拟 Node.js / Claude Code(真实客户端使用 HTTP/2),但本次连接协商为 HTTP/1.1,协议维度与被模拟客户端不一致。",
				Suggestion: "指纹 transport 当前未启用 HTTP/2(种子 Profile 的 ALPN 为 http/1.1,且 fingerprint transport 关闭了 ForceAttemptHTTP2)。要做到协议一致需推进 H2 方案。",
			})
		case 2:
			findings = append(findings, ConnectionCoherenceFinding{
				Code:     "protocol_h2_ok",
				Severity: coherenceSeverityInfo,
				Message:  "TLS 指纹模拟 Node / Claude Code,且本次连接协商为 HTTP/2,协议维度一致。",
			})
		}
	}

	// 4) OpenAI 账号:本诊断经 DoWithTLS 探测,与原生转发(Do + 内置 H2 模式)不是同一条路径。
	if result.Platform == PlatformOpenAI {
		findings = append(findings, ConnectionCoherenceFinding{
			Code:     "openai_probe_path_note",
			Severity: coherenceSeverityInfo,
			Message:  "本诊断经由 TLS 指纹路径探测;OpenAI 原生转发实际走非指纹路径(内置 HTTP/2 模式与 h1 回退),此处协商到的协议不代表原生转发的真实协议。",
		})
	}

	result.CoherenceFindings = findings
	result.CoherenceStatus = summarizeCoherenceStatus(findings, result.Success)
}

// summarizeCoherenceStatus 汇总总体状态:任一 warning 即 warning;否则若已探测或产出发现即 coherent;都无则 not_checked。
func summarizeCoherenceStatus(findings []ConnectionCoherenceFinding, probed bool) string {
	for _, finding := range findings {
		if finding.Severity == coherenceSeverityWarning {
			return coherenceStatusWarning
		}
	}
	if probed || len(findings) > 0 {
		return coherenceStatusCoherent
	}
	return coherenceStatusNotChecked
}

type profileClientType int

const (
	profileClientUnknown profileClientType = iota
	profileClientNode
	profileClientBrowser
)

// classifyProfileClientType 依据 Profile 名称粗分客户端族,判定口径与
// model.TLSFingerprintProfile.BuildMetadata 保持一致(claude/codex/node 视为 Node 族)。
func classifyProfileClientType(name string) profileClientType {
	text := strings.ToLower(strings.TrimSpace(name))
	if text == "" {
		return profileClientUnknown
	}
	switch {
	case strings.Contains(text, "claude"),
		strings.Contains(text, "codex"),
		strings.Contains(text, "node"):
		return profileClientNode
	case strings.Contains(text, "chrome"),
		strings.Contains(text, "firefox"),
		strings.Contains(text, "safari"),
		strings.Contains(text, "browser"):
		return profileClientBrowser
	}
	return profileClientUnknown
}

// diagnosticNegotiatedHTTPMajor 返回本次连接实际使用的 HTTP 主版本(1 或 2);无法判定时返回 0。
// 优先看响应 Proto,回退到协商的 ALPN。
func diagnosticNegotiatedHTTPMajor(result *AccountConnectionDiagnostic) int {
	proto := strings.ToLower(strings.TrimSpace(result.HTTPProtocol))
	switch {
	case strings.HasPrefix(proto, "http/2"):
		return 2
	case strings.HasPrefix(proto, "http/1"):
		return 1
	}
	switch strings.ToLower(strings.TrimSpace(result.ALPN)) {
	case "h2":
		return 2
	case "http/1.1", "http/1.0":
		return 1
	}
	return 0
}
