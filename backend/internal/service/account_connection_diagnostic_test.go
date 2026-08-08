//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type diagnosticHTTPUpstreamStub struct {
	responses []*http.Response
	errors    []error
	requests []*http.Request
	proxies  []string
}

type diagnosticPersistenceRepo struct {
	*mockAccountRepoForGemini
	updateErr error
}

func (r *diagnosticPersistenceRepo) UpdateExtra(_ context.Context, id int64, updates map[string]any) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	account := r.accountsByID[id]
	if account == nil {
		return errors.New("account not found")
	}
	if account.Extra == nil {
		account.Extra = make(map[string]any)
	}
	for key, value := range updates {
		account.Extra[key] = value
	}
	return nil
}

func (s *diagnosticHTTPUpstreamStub) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return nil, nil
}

func (s *diagnosticHTTPUpstreamStub) DoWithTLS(req *http.Request, proxyURL string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	s.requests = append(s.requests, req)
	s.proxies = append(s.proxies, proxyURL)
	response := s.responses[0]
	s.responses = s.responses[1:]
	var err error
	if len(s.errors) > 0 {
		err = s.errors[0]
		s.errors = s.errors[1:]
	}
	return response, err
}

func diagnosticResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Proto:      "HTTP/1.1",
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestDiagnoseAccountConnectionDoesNotSendCredentials(t *testing.T) {
	target := httptest.NewServer(http.NotFoundHandler())
	defer target.Close()

	account := &Account{
		ID:          42,
		Platform:    "upstream",
		Type:        AccountTypeAPIKey,
		Concurrency: 2,
		ProxyID:     func() *int64 { id := int64(7); return &id }(),
		Proxy:       &Proxy{ID: 7, Protocol: "http", Host: "proxy.local", Port: 8080},
		Credentials: map[string]any{
			"base_url":  target.URL,
			"api_key":   "must-not-be-sent",
			"secret":    "must-not-be-sent",
		},
	}
	repo := &diagnosticPersistenceRepo{mockAccountRepoForGemini: &mockAccountRepoForGemini{
		accountsByID: map[int64]*Account{account.ID: account},
	}}
	upstream := &diagnosticHTTPUpstreamStub{responses: []*http.Response{
		diagnosticResponse(http.StatusNotFound, "not found"),
		diagnosticResponse(http.StatusOK, `{"ip":"203.0.113.7"}`),
	}}
	cfg := &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{
		Enabled:           false,
		AllowInsecureHTTP: true,
	}}}

	result, err := (&AccountTestService{
		accountRepo:   repo,
		httpUpstream: upstream,
		cfg:           cfg,
	}).DiagnoseAccountConnection(context.Background(), account.ID)

	require.NoError(t, err)
	require.True(t, result.Success, "HTTP 404 still proves that the target was reached")
	require.Equal(t, http.StatusNotFound, result.HTTPStatus)
	require.Equal(t, "http://proxy.local:8080", result.ProxyEndpoint)
	require.Equal(t, "observed", result.ProxyExitIPStatus)
	require.Equal(t, "203.0.113.7", result.ProxyExitIP)
	require.Equal(t, []string{account.Proxy.URL(), account.Proxy.URL()}, upstream.proxies)
	require.Len(t, upstream.requests, 2)
	for _, request := range upstream.requests {
		require.Empty(t, request.Header.Get("Authorization"))
		require.Empty(t, request.Header.Get("x-api-key"))
		require.Empty(t, request.Header.Get("Cookie"))
	}

	last, err := (&AccountTestService{accountRepo: repo}).GetLastAccountConnectionDiagnostic(context.Background(), account.ID)
	require.NoError(t, err)
	require.NotNil(t, last)
	require.Equal(t, result.CheckedAt, last.CheckedAt)
	require.NotContains(t, string(mustJSONMarshal(last)), "must-not-be-sent")
}

func mustJSONMarshal(value any) []byte {
	payload, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return payload
}

func TestDiagnoseAccountConnectionKeepsExitProbeAfterTargetFailure(t *testing.T) {
	target := httptest.NewServer(http.NotFoundHandler())
	defer target.Close()

	account := &Account{
		ID:       43,
		Platform: "upstream",
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"base_url": target.URL,
		},
	}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
	upstream := &diagnosticHTTPUpstreamStub{responses: []*http.Response{
		nil,
		diagnosticResponse(http.StatusOK, `{"ip":"198.51.100.9"}`),
	}, errors: []error{errors.New("proxyconnect tcp: connect failed"), nil}}

	result, err := (&AccountTestService{
		accountRepo:   repo,
		httpUpstream: upstream,
		cfg: &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{
			Enabled: false, AllowInsecureHTTP: true,
		}}},
	}).DiagnoseAccountConnection(context.Background(), account.ID)

	require.NoError(t, err)
	require.False(t, result.Success)
	require.Equal(t, "proxy_or_tcp", result.FailureStage)
	require.Equal(t, "observed", result.ProxyExitIPStatus)
	require.Equal(t, "198.51.100.9", result.ProxyExitIP)
	require.Len(t, upstream.requests, 2)
}

func TestDiagnosticHelpers(t *testing.T) {
	require.Equal(t, "TLS 1.3", diagnosticTLSVersion(0x0304))
	require.Equal(t, "dns", diagnosticFailureStage(errors.New("lookup api.example.com: no such host")))
	require.Equal(t, "proxy_or_tcp", diagnosticFailureStage(errors.New("proxyconnect tcp: connect failed")))
	require.Equal(t, "timeout", diagnosticFailureStage(context.DeadlineExceeded))
	require.Equal(t, "GET [configured proxy]: failed", redactDiagnosticError(
		errors.New("GET http://proxy.local:8080: failed"),
		"http://proxy.local:8080",
	))
	require.Equal(t, "proxy [configured proxy] failed", redactDiagnosticError(
		errors.New("proxy http://user:password-a@proxy.local:8080 failed"),
		"http://user:password-a@proxy.local:8080",
	))
}
