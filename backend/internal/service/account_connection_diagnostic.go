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
	CheckedAt             time.Time `json:"checked_at"`
}

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
