//go:build unit

package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountConnectionIdentityIsStableForEquivalentBindings(t *testing.T) {
	first := NewAccountConnectionIdentity(7, "API.EXAMPLE.COM:443", "http://proxy.local:8080", "profile-a", "openai_h2", "pool-a")
	second := NewAccountConnectionIdentity(7, "api.example.com:443", "http://proxy.local:8080", "profile-a", "openai_h2", "pool-a")

	require.Equal(t, first.Key, second.Key)
	require.Equal(t, "proxy:"+shortConnectionDigest("http://proxy.local:8080"), first.ProxyScope)
	require.Equal(t, "api.example.com:443", first.TargetHost)
}

func TestAccountConnectionIdentityChangesWithEffectiveConnectionInputs(t *testing.T) {
	base := NewAccountConnectionIdentity(7, "api.example.com:443", "direct", "profile-a", "default", "pool-a")

	for name, changed := range map[string]AccountConnectionIdentity{
		"profile": NewAccountConnectionIdentity(7, "api.example.com:443", "direct", "profile-b", "default", "pool-a"),
		"proxy":   NewAccountConnectionIdentity(7, "api.example.com:443", "proxy-b", "profile-a", "default", "pool-a"),
		"protocol": NewAccountConnectionIdentity(7, "api.example.com:443", "direct", "profile-a", "openai_h1", "pool-a"),
		"pool":    NewAccountConnectionIdentity(7, "api.example.com:443", "direct", "profile-a", "default", "pool-b"),
	} {
		t.Run(name, func(t *testing.T) {
			require.NotEqual(t, base.Key, changed.Key)
		})
	}
}

func TestAccountConnectionIdentityDoesNotExposeProxyCredentials(t *testing.T) {
	identity := NewAccountConnectionIdentity(7, "api.example.com:443", "http://auth-digest@proxy.local:8080", "profile-a", "default", "pool-a")
	payload, err := json.Marshal(identity)
	require.NoError(t, err)
	require.NotContains(t, string(payload), "password")
	require.NotContains(t, string(payload), "secret-token")
	require.NotContains(t, string(payload), "proxy.local:8080")
}

func TestAccountTransportHealthTracksIdentityGeneration(t *testing.T) {
	health := NewAccountTransportHealth()
	first := NewAccountConnectionIdentity(7, "api.example.com:443", "direct", "profile-a", "default", "pool-a")
	second := NewAccountConnectionIdentity(7, "api.example.com:443", "direct", "profile-b", "default", "pool-a")

	health.RecordConnectionIdentity(7, first)
	require.Equal(t, uint64(1), health.SnapshotTransportHealth(7).ConnectionIdentityGeneration)
	health.RecordConnectionIdentity(7, first)
	require.Equal(t, uint64(1), health.SnapshotTransportHealth(7).ConnectionIdentityGeneration)
	health.RecordConnectionIdentity(7, second)
	snapshot := health.SnapshotTransportHealth(7)
	require.Equal(t, uint64(2), snapshot.ConnectionIdentityGeneration)
	require.Equal(t, second.Key, snapshot.ConnectionIdentityKey)
	require.NotNil(t, snapshot.ConnectionIdentityChangedAt)
}
