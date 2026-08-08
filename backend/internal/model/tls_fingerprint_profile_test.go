package model

import (
	"strings"
	"testing"
)

func TestTLSFingerprintProfileBuildMetadata(t *testing.T) {
	profile := &TLSFingerprintProfile{
		Name:             "macOS Node.js v24",
		ALPNProtocols:    []string{"h2", "http/1.1"},
		SupportedVersions: []uint16{0x0304, 0x0303},
		CipherSuites:     []uint16{0x1301, 0x1302},
	}

	metadata := profile.BuildMetadata()
	if metadata.ClientType != "Node.js / Claude Code" {
		t.Fatalf("client type = %q, want Node.js / Claude Code", metadata.ClientType)
	}
	if metadata.ClientVersionRange != "24.x" {
		t.Fatalf("client version range = %q, want 24.x", metadata.ClientVersionRange)
	}
	if metadata.TLSVersionRange != "TLS 1.3, TLS 1.2" {
		t.Fatalf("TLS version range = %q", metadata.TLSVersionRange)
	}
	if metadata.ALPNPreference != "h2, http/1.1" {
		t.Fatalf("ALPN preference = %q", metadata.ALPNPreference)
	}
	if len(metadata.FingerprintKey) != 64 || strings.Trim(metadata.FingerprintKey, "0123456789abcdef") != "" {
		t.Fatalf("fingerprint key should be a lowercase SHA-256 digest, got %q", metadata.FingerprintKey)
	}
}

func TestTLSFingerprintProfileMetadataDoesNotInferVersionFromTLSNumbers(t *testing.T) {
	profile := &TLSFingerprintProfile{
		Name:             "Custom TLS 1.3",
		SupportedVersions: []uint16{0x0304},
	}

	metadata := profile.BuildMetadata()
	if metadata.ClientType != "Custom" {
		t.Fatalf("client type = %q, want Custom", metadata.ClientType)
	}
	if metadata.ClientVersionRange != "Unspecified" {
		t.Fatalf("client version range = %q, want Unspecified", metadata.ClientVersionRange)
	}
}
