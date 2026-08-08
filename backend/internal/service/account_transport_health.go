package service

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// TransportHealthFailureStage is a bounded, credential-free description of
// where an outbound request failed.
type TransportHealthFailureStage string

const (
	TransportHealthFailureTransportAcquire TransportHealthFailureStage = "transport_acquire"
	TransportHealthFailureProxyConnect     TransportHealthFailureStage = "proxy_connect"
	TransportHealthFailureTLSHandshake     TransportHealthFailureStage = "tls_handshake"
	TransportHealthFailureHTTP2            TransportHealthFailureStage = "http2"
	TransportHealthFailureTimeout          TransportHealthFailureStage = "timeout"
	TransportHealthFailureNetwork          TransportHealthFailureStage = "network"
	TransportHealthFailureRequest          TransportHealthFailureStage = "request"
)

// AccountTransportHealthSnapshot is the safe read-only view exposed to the
// admin panel. It intentionally contains no proxy URL, error text, or secret.
type AccountTransportHealthSnapshot struct {
	AccountID                    int64                          `json:"account_id"`
	RequestsTotal                int64                          `json:"requests_total"`
	SuccessTotal                 int64                          `json:"success_total"`
	FailureTotal                 int64                          `json:"failure_total"`
	TransportReuseTotal          int64                          `json:"transport_reuse_total"`
	TransportCreateTotal         int64                          `json:"transport_create_total"`
	TransportAcquireFailureTotal int64                          `json:"transport_acquire_failure_total"`
	ProxyConnectFailureTotal     int64                          `json:"proxy_connect_failure_total"`
	TLSHandshakeFailureTotal     int64                          `json:"tls_handshake_failure_total"`
	HTTP2SuccessTotal            int64                          `json:"http2_success_total"`
	HTTP2FallbackTotal           int64                          `json:"http2_fallback_total"`
	HTTP2FallbackRequestTotal    int64                          `json:"http2_fallback_request_total"`
	TimeoutTotal                 int64                          `json:"timeout_total"`
	NetworkFailureTotal          int64                          `json:"network_failure_total"`
	LastFailureStage             TransportHealthFailureStage   `json:"last_failure_stage,omitempty"`
	LastProtocolMode             string                         `json:"last_protocol_mode,omitempty"`
	LastFailureAt                *time.Time                     `json:"last_failure_at,omitempty"`
	ConnectionIdentityKey        string                         `json:"connection_identity_key,omitempty"`
	ConnectionIdentityTargetHost string                         `json:"connection_identity_target_host,omitempty"`
	ConnectionIdentityProxyScope string                         `json:"connection_identity_proxy_scope,omitempty"`
	ConnectionIdentityFingerprintKey string                      `json:"connection_identity_fingerprint_key,omitempty"`
	ConnectionIdentityProtocolMode string                         `json:"connection_identity_protocol_mode,omitempty"`
	ConnectionIdentityGeneration uint64                         `json:"connection_identity_generation,omitempty"`
	ConnectionIdentityChangedAt  *time.Time                     `json:"connection_identity_changed_at,omitempty"`
}

type accountTransportHealthCounters struct {
	requestsTotal                atomic.Int64
	successTotal                 atomic.Int64
	failureTotal                 atomic.Int64
	transportReuseTotal          atomic.Int64
	transportCreateTotal         atomic.Int64
	transportAcquireFailureTotal atomic.Int64
	proxyConnectFailureTotal     atomic.Int64
	tlsHandshakeFailureTotal     atomic.Int64
	http2SuccessTotal            atomic.Int64
	http2FallbackTotal           atomic.Int64
	http2FallbackRequestTotal    atomic.Int64
	timeoutTotal                 atomic.Int64
	networkFailureTotal          atomic.Int64

	metaMu          sync.RWMutex
	lastFailureStage TransportHealthFailureStage
	lastProtocolMode string
	lastFailureAt    time.Time
	connectionIdentityKey            string
	connectionIdentityTargetHost     string
	connectionIdentityProxyScope     string
	connectionIdentityFingerprintKey string
	connectionIdentityProtocolMode   string
	connectionIdentityGeneration     uint64
	connectionIdentityChangedAt      time.Time
}

// AccountTransportHealth keeps cheap process-local counters for outbound
// account connections. It is deliberately not persisted: counters reset on
// restart and therefore cannot become stale historical truth.
type AccountTransportHealth struct {
	accounts sync.Map // map[int64]*accountTransportHealthCounters
}

// NewAccountTransportHealth creates an empty in-memory observer.
func NewAccountTransportHealth() *AccountTransportHealth {
	return &AccountTransportHealth{}
}

func (h *AccountTransportHealth) state(accountID int64) *accountTransportHealthCounters {
	if h == nil {
		return nil
	}
	if existing, ok := h.accounts.Load(accountID); ok {
		if state, ok := existing.(*accountTransportHealthCounters); ok && state != nil {
			return state
		}
	}
	state := &accountTransportHealthCounters{}
	actual, _ := h.accounts.LoadOrStore(accountID, state)
	if cached, ok := actual.(*accountTransportHealthCounters); ok && cached != nil {
		return cached
	}
	return state
}

// RecordRequest records an attempted outbound request, including attempts that
// fail while acquiring a Transport.
func (h *AccountTransportHealth) RecordRequest(accountID int64) {
	if state := h.state(accountID); state != nil {
		state.requestsTotal.Add(1)
	}
}

// RecordConnectionIdentity records the latest credential-free connection
// identity used by an account. Counters remain process-local account totals;
// the generation lets operators distinguish totals before and after a binding
// or Transport identity change.
func (h *AccountTransportHealth) RecordConnectionIdentity(accountID int64, identity AccountConnectionIdentity) {
	if state := h.state(accountID); state != nil {
		state.metaMu.Lock()
		if state.connectionIdentityKey != "" && state.connectionIdentityKey != identity.Key {
			state.connectionIdentityGeneration++
			state.connectionIdentityChangedAt = time.Now().UTC()
		}
		if state.connectionIdentityKey == "" {
			state.connectionIdentityGeneration = 1
			state.connectionIdentityChangedAt = time.Now().UTC()
		}
		state.connectionIdentityKey = identity.Key
		state.connectionIdentityTargetHost = identity.TargetHost
		state.connectionIdentityProxyScope = identity.ProxyScope
		state.connectionIdentityFingerprintKey = identity.FingerprintKey
		state.connectionIdentityProtocolMode = identity.ProtocolMode
		state.metaMu.Unlock()
	}
}

// RecordSuccess records that the upstream returned an HTTP response. HTTP 4xx
// and 5xx responses still count as transport successes because the network
// exchange itself completed.
func (h *AccountTransportHealth) RecordSuccess(accountID int64, protocolMode string) {
	if state := h.state(accountID); state != nil {
		state.successTotal.Add(1)
		state.setProtocol(protocolMode)
	}
}

// RecordTransportReuse records a request that reused an existing client and
// its connection pool entry.
func (h *AccountTransportHealth) RecordTransportReuse(accountID int64) {
	if state := h.state(accountID); state != nil {
		state.transportReuseTotal.Add(1)
	}
}

// RecordTransportCreate records a request that had to create a client and a
// fresh connection pool entry.
func (h *AccountTransportHealth) RecordTransportCreate(accountID int64) {
	if state := h.state(accountID); state != nil {
		state.transportCreateTotal.Add(1)
	}
}

// RecordTransportAcquireFailure records failure before a request reached the
// RoundTripper, such as an invalid proxy or a full client cache.
func (h *AccountTransportHealth) RecordTransportAcquireFailure(accountID int64, protocolMode string, err error) {
	if state := h.state(accountID); state != nil {
		state.transportAcquireFailureTotal.Add(1)
		state.recordFailure(accountTransportFailureStageForAcquire(err), protocolMode)
	}
}

// RecordFailure records a request-level failure using a bounded classifier.
// The error text is used only in memory to select a category and is never
// included in the snapshot.
func (h *AccountTransportHealth) RecordFailure(accountID int64, protocolMode string, err error) {
	if state := h.state(accountID); state != nil {
		stage := ClassifyTransportHealthFailure(err)
		state.recordFailure(stage, protocolMode)
	}
}

// RecordHTTP2Success records a completed request on the OpenAI HTTP/2 path.
func (h *AccountTransportHealth) RecordHTTP2Success(accountID int64) {
	if state := h.state(accountID); state != nil {
		state.http2SuccessTotal.Add(1)
	}
}

// RecordHTTP2Fallback records an HTTP/2 compatibility failure which can cause
// the proxy-scoped HTTP/1.1 fallback to activate.
func (h *AccountTransportHealth) RecordHTTP2Fallback(accountID int64) {
	if state := h.state(accountID); state != nil {
		state.http2FallbackTotal.Add(1)
	}
}

// RecordHTTP2FallbackRequest records a request that used the active fallback.
func (h *AccountTransportHealth) RecordHTTP2FallbackRequest(accountID int64) {
	if state := h.state(accountID); state != nil {
		state.http2FallbackRequestTotal.Add(1)
	}
}

// SnapshotTransportHealth returns a consistent-enough point-in-time view of
// one account. Counters are atomic; the small metadata block is copied under a
// read lock.
func (h *AccountTransportHealth) SnapshotTransportHealth(accountID int64) AccountTransportHealthSnapshot {
	snapshot := AccountTransportHealthSnapshot{AccountID: accountID}
	state := h.state(accountID)
	if state == nil {
		return snapshot
	}
	snapshot.RequestsTotal = state.requestsTotal.Load()
	snapshot.SuccessTotal = state.successTotal.Load()
	snapshot.FailureTotal = state.failureTotal.Load()
	snapshot.TransportReuseTotal = state.transportReuseTotal.Load()
	snapshot.TransportCreateTotal = state.transportCreateTotal.Load()
	snapshot.TransportAcquireFailureTotal = state.transportAcquireFailureTotal.Load()
	snapshot.ProxyConnectFailureTotal = state.proxyConnectFailureTotal.Load()
	snapshot.TLSHandshakeFailureTotal = state.tlsHandshakeFailureTotal.Load()
	snapshot.HTTP2SuccessTotal = state.http2SuccessTotal.Load()
	snapshot.HTTP2FallbackTotal = state.http2FallbackTotal.Load()
	snapshot.HTTP2FallbackRequestTotal = state.http2FallbackRequestTotal.Load()
	snapshot.TimeoutTotal = state.timeoutTotal.Load()
	snapshot.NetworkFailureTotal = state.networkFailureTotal.Load()

	state.metaMu.RLock()
	snapshot.LastFailureStage = state.lastFailureStage
	snapshot.LastProtocolMode = state.lastProtocolMode
	if !state.lastFailureAt.IsZero() {
		lastFailureAt := state.lastFailureAt.UTC()
		snapshot.LastFailureAt = &lastFailureAt
	}
	snapshot.ConnectionIdentityKey = state.connectionIdentityKey
	snapshot.ConnectionIdentityTargetHost = state.connectionIdentityTargetHost
	snapshot.ConnectionIdentityProxyScope = state.connectionIdentityProxyScope
	snapshot.ConnectionIdentityFingerprintKey = state.connectionIdentityFingerprintKey
	snapshot.ConnectionIdentityProtocolMode = state.connectionIdentityProtocolMode
	snapshot.ConnectionIdentityGeneration = state.connectionIdentityGeneration
	if !state.connectionIdentityChangedAt.IsZero() {
		changedAt := state.connectionIdentityChangedAt.UTC()
		snapshot.ConnectionIdentityChangedAt = &changedAt
	}
	state.metaMu.RUnlock()
	return snapshot
}

func (state *accountTransportHealthCounters) recordFailure(stage TransportHealthFailureStage, protocolMode string) {
	state.failureTotal.Add(1)
	switch stage {
	case TransportHealthFailureProxyConnect:
		state.proxyConnectFailureTotal.Add(1)
	case TransportHealthFailureTLSHandshake:
		state.tlsHandshakeFailureTotal.Add(1)
	case TransportHealthFailureTimeout:
		state.timeoutTotal.Add(1)
	case TransportHealthFailureNetwork:
		state.networkFailureTotal.Add(1)
	}
	state.metaMu.Lock()
	state.lastFailureStage = stage
	state.lastProtocolMode = strings.TrimSpace(protocolMode)
	state.lastFailureAt = time.Now().UTC()
	state.metaMu.Unlock()
}

func (state *accountTransportHealthCounters) setProtocol(protocolMode string) {
	state.metaMu.Lock()
	state.lastProtocolMode = strings.TrimSpace(protocolMode)
	state.metaMu.Unlock()
}

// ClassifyTransportHealthFailure maps common Go networking errors to a small,
// stable set of stages. Keep this conservative: unknown errors remain
// "request" instead of being presented as a more specific diagnosis.
func ClassifyTransportHealthFailure(err error) TransportHealthFailureStage {
	if err == nil {
		return TransportHealthFailureRequest
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return TransportHealthFailureTimeout
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return TransportHealthFailureTimeout
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded") {
		return TransportHealthFailureTimeout
	}
	if strings.Contains(msg, "http2") || strings.Contains(msg, "http/2") || strings.Contains(msg, "alpn") ||
		strings.Contains(msg, "protocol error") || strings.Contains(msg, "stream error") || strings.Contains(msg, "goaway") ||
		strings.Contains(msg, "refused_stream") {
		return TransportHealthFailureHTTP2
	}
	if strings.Contains(msg, "tls") || strings.Contains(msg, "certificate") || strings.Contains(msg, "handshake") {
		return TransportHealthFailureTLSHandshake
	}
	if strings.Contains(msg, "proxyconnect") || strings.Contains(msg, "connect to proxy") || strings.Contains(msg, "socks") {
		return TransportHealthFailureProxyConnect
	}
	if strings.Contains(msg, "dial tcp") || strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no route to host") || strings.Contains(msg, "network is unreachable") ||
		strings.Contains(msg, "lookup ") {
		return TransportHealthFailureNetwork
	}
	return TransportHealthFailureRequest
}

func accountTransportFailureStageForAcquire(err error) TransportHealthFailureStage {
	stage := ClassifyTransportHealthFailure(err)
	if stage == TransportHealthFailureRequest {
		return TransportHealthFailureTransportAcquire
	}
	return stage
}
