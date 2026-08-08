package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// AccountConnectionIdentity is a credential-free description of the outbound
// connection context used by one account. It is for diagnostics and cache
// correlation only; it is not an upstream client attestation.
type AccountConnectionIdentity struct {
	Key            string `json:"identity_key"`
	TargetHost     string `json:"target_host,omitempty"`
	ProxyScope     string `json:"proxy_scope,omitempty"`
	FingerprintKey string `json:"fingerprint_key,omitempty"`
	ProtocolMode   string `json:"protocol_mode,omitempty"`
}

// NewAccountConnectionIdentity creates a stable, non-sensitive identity
// summary. proxyKey must already be normalized by the transport layer and must
// not contain proxy credentials.
func NewAccountConnectionIdentity(
	accountID int64,
	targetHost string,
	proxyKey string,
	fingerprintKey string,
	protocolMode string,
	poolKey string,
) AccountConnectionIdentity {
	targetHost = strings.ToLower(strings.TrimSpace(targetHost))
	proxyKey = strings.TrimSpace(proxyKey)
	protocolMode = strings.TrimSpace(protocolMode)
	fingerprintKey = strings.TrimSpace(fingerprintKey)
	proxyScope := "direct"
	if proxyKey != "" && proxyKey != "direct" {
		proxyScope = "proxy:" + shortConnectionDigest(proxyKey)
	}

	canonical := fmt.Sprintf("account=%d\nhost=%s\nproxy=%s\nfp=%s\nprotocol=%s\npool=%s",
		accountID, targetHost, proxyKey, fingerprintKey, protocolMode, strings.TrimSpace(poolKey))
	return AccountConnectionIdentity{
		Key:            shortConnectionDigest(canonical),
		TargetHost:     targetHost,
		ProxyScope:     proxyScope,
		FingerprintKey: fingerprintKey,
		ProtocolMode:   protocolMode,
	}
}

func shortConnectionDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])[:16]
}
