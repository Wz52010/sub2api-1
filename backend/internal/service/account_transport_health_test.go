package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClassifyTransportHealthFailure(t *testing.T) {
	tests := []struct {
		name  string
		err   error
		stage TransportHealthFailureStage
	}{
		{name: "deadline", err: context.DeadlineExceeded, stage: TransportHealthFailureTimeout},
		{name: "proxy connect", err: errors.New("proxyconnect tcp: dial tcp 127.0.0.1:8080: connect: connection refused"), stage: TransportHealthFailureProxyConnect},
		{name: "tls handshake", err: errors.New("remote error: tls: handshake failure"), stage: TransportHealthFailureTLSHandshake},
		{name: "http2", err: errors.New("http2: server sent GOAWAY"), stage: TransportHealthFailureHTTP2},
		{name: "network", err: errors.New("dial tcp: lookup upstream: no such host"), stage: TransportHealthFailureNetwork},
		{name: "unknown", err: errors.New("upstream rejected request"), stage: TransportHealthFailureRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.stage, ClassifyTransportHealthFailure(tt.err))
		})
	}
}

func TestAccountTransportHealthSnapshotDoesNotExposeSecrets(t *testing.T) {
	health := NewAccountTransportHealth()
	health.RecordRequest(42)
	health.RecordSuccess(42, "openai_h2")
	health.RecordHTTP2Success(42)
	health.RecordHTTP2FallbackRequest(42)
	health.RecordTransportAcquireFailure(42, "openai_h2", errors.New("proxy password=secret-token"))
	health.RecordHTTP2Fallback(42)

	snapshot := health.SnapshotTransportHealth(42)
	require.Equal(t, int64(42), snapshot.AccountID)
	require.Equal(t, int64(1), snapshot.RequestsTotal)
	require.Equal(t, int64(1), snapshot.SuccessTotal)
	require.Equal(t, int64(1), snapshot.FailureTotal)
	require.Equal(t, int64(1), snapshot.TransportAcquireFailureTotal)
	require.Equal(t, int64(1), snapshot.HTTP2SuccessTotal)
	require.Equal(t, int64(1), snapshot.HTTP2FallbackTotal)
	require.Equal(t, int64(1), snapshot.HTTP2FallbackRequestTotal)
	require.NotNil(t, snapshot.LastFailureAt)

	payload, err := json.Marshal(snapshot)
	require.NoError(t, err)
	require.NotContains(t, string(payload), "secret-token")
	require.NotContains(t, string(payload), "password")
}

func TestAccountTransportHealthConcurrentUpdates(t *testing.T) {
	health := NewAccountTransportHealth()
	const workers = 16
	const iterations = 100

	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				health.RecordRequest(7)
				if i%2 == 0 {
					health.RecordSuccess(7, "openai_h2")
					health.RecordHTTP2Success(7)
				} else {
					health.RecordFailure(7, "openai_h2", errors.New("http2: stream error"))
				}
			}
		}()
	}
	wg.Wait()

	snapshot := health.SnapshotTransportHealth(7)
	require.Equal(t, int64(workers*iterations), snapshot.RequestsTotal)
	require.Equal(t, int64(workers*iterations/2), snapshot.SuccessTotal)
	require.Equal(t, int64(workers*iterations/2), snapshot.FailureTotal)
	require.Equal(t, int64(workers*iterations/2), snapshot.HTTP2SuccessTotal)
}
