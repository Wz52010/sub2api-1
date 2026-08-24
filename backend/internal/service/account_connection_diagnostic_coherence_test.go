package service

import "testing"

func coherenceFindingByCode(findings []ConnectionCoherenceFinding, code string) (ConnectionCoherenceFinding, bool) {
	for _, finding := range findings {
		if finding.Code == code {
			return finding, true
		}
	}
	return ConnectionCoherenceFinding{}, false
}

func TestEvaluateConnectionCoherence(t *testing.T) {
	tests := []struct {
		name         string
		result       *AccountConnectionDiagnostic
		wantStatus   string
		wantCodes    []string
		notWantCodes []string
	}{
		{
			name: "anthropic node profile h1 -> coherent (undici is h1)",
			result: &AccountConnectionDiagnostic{
				Platform:              PlatformAnthropic,
				TLSFingerprintEnabled: true,
				TLSProfileName:        "Claude Code - Node.js 24.x",
				Success:               true,
				HTTPProtocol:          "HTTP/1.1",
				ALPN:                  "http/1.1",
			},
			wantStatus:   coherenceStatusCoherent,
			wantCodes:    []string{"protocol_h1_node_ok"},
			notWantCodes: []string{"protocol_h2_vs_node_client", "codex_runtime_mismatch"},
		},
		{
			name: "anthropic node profile h2 -> warning (undici is not h2)",
			result: &AccountConnectionDiagnostic{
				Platform:              PlatformAnthropic,
				TLSFingerprintEnabled: true,
				TLSProfileName:        "Claude Code - Node.js 24.x",
				Success:               true,
				HTTPProtocol:          "HTTP/2.0",
				ALPN:                  "h2",
			},
			wantStatus:   coherenceStatusWarning,
			wantCodes:    []string{"protocol_h2_vs_node_client"},
			notWantCodes: []string{"protocol_h1_node_ok", "codex_runtime_mismatch"},
		},
		{
			name: "openai account with node profile on h1 -> codex mismatch + protocol + path note",
			result: &AccountConnectionDiagnostic{
				Platform:              PlatformOpenAI,
				TLSFingerprintEnabled: true,
				TLSProfileName:        "Codex CLI - Node.js 24.x",
				Success:               true,
				HTTPProtocol:          "HTTP/1.1",
			},
			wantStatus: coherenceStatusWarning,
			wantCodes: []string{
				"codex_runtime_mismatch",
				"protocol_h1_node_ok",
				"openai_probe_path_note",
			},
		},
		{
			name: "tls fingerprint disabled -> info only, coherent",
			result: &AccountConnectionDiagnostic{
				Platform:              PlatformAnthropic,
				TLSFingerprintEnabled: false,
				Success:               true,
				HTTPProtocol:          "HTTP/2.0",
			},
			wantStatus:   coherenceStatusCoherent,
			wantCodes:    []string{"tls_fingerprint_disabled"},
			notWantCodes: []string{"protocol_h1_node_ok", "protocol_h2_vs_node_client", "codex_runtime_mismatch"},
		},
		{
			name: "failed probe still yields config-level findings",
			result: &AccountConnectionDiagnostic{
				Platform:              PlatformOpenAI,
				TLSFingerprintEnabled: true,
				TLSProfileName:        "Codex CLI - Node.js 24.x",
				Success:               false,
			},
			wantStatus:   coherenceStatusWarning,
			wantCodes:    []string{"codex_runtime_mismatch", "openai_probe_path_note"},
			notWantCodes: []string{"protocol_h1_node_ok", "protocol_h2_vs_node_client"},
		},
		{
			name: "openai account with rust profile -> coherent, no mismatch",
			result: &AccountConnectionDiagnostic{
				Platform:              PlatformOpenAI,
				TLSFingerprintEnabled: true,
				TLSProfileName:        "Codex CLI - Rust (reqwest)",
				Success:               true,
				HTTPProtocol:          "HTTP/2.0",
			},
			wantStatus:   coherenceStatusCoherent,
			wantCodes:    []string{"codex_rust_ok", "openai_probe_path_note", "protocol_h2_rust_ok"},
			notWantCodes: []string{"codex_runtime_mismatch", "protocol_h1_vs_rust_client"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			evaluateConnectionCoherence(tc.result)
			if tc.result.CoherenceStatus != tc.wantStatus {
				t.Fatalf("status = %q, want %q (findings: %+v)", tc.result.CoherenceStatus, tc.wantStatus, tc.result.CoherenceFindings)
			}
			for _, code := range tc.wantCodes {
				if _, ok := coherenceFindingByCode(tc.result.CoherenceFindings, code); !ok {
					t.Errorf("expected finding %q, not present", code)
				}
			}
			for _, code := range tc.notWantCodes {
				if _, ok := coherenceFindingByCode(tc.result.CoherenceFindings, code); ok {
					t.Errorf("did not expect finding %q, but it was present", code)
				}
			}
		})
	}
}

func TestEvaluateConnectionCoherenceNilSafe(t *testing.T) {
	// Must not panic on a nil result.
	evaluateConnectionCoherence(nil)
}

func TestClassifyProfileClientType(t *testing.T) {
	nodeNames := []string{
		"Claude Code - Node.js 24.x",
		"Codex CLI - Node.js 24.x",
		"Claude/Codex shared - Node.js 22.17.1 Linux x64",
		"Claude/Codex shared - Node.js 24.3.0 macOS arm64",
	}
	for _, name := range nodeNames {
		if got := classifyProfileClientType(name); got != profileClientNode {
			t.Errorf("classifyProfileClientType(%q) = %v, want node", name, got)
		}
	}
	if got := classifyProfileClientType("Chrome 120 Windows"); got != profileClientBrowser {
		t.Errorf("classifyProfileClientType(chrome) = %v, want browser", got)
	}
	if got := classifyProfileClientType(""); got != profileClientUnknown {
		t.Errorf("classifyProfileClientType(empty) = %v, want unknown", got)
	}
}

func TestDiagnosticNegotiatedHTTPMajor(t *testing.T) {
	cases := []struct {
		name  string
		proto string
		alpn  string
		want  int
	}{
		{"proto h2", "HTTP/2.0", "", 2},
		{"proto h1", "HTTP/1.1", "", 1},
		{"alpn h2 fallback", "", "h2", 2},
		{"alpn http1 fallback", "", "http/1.1", 1},
		{"proto beats alpn", "HTTP/2.0", "http/1.1", 2},
		{"unknown", "", "none", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := diagnosticNegotiatedHTTPMajor(&AccountConnectionDiagnostic{HTTPProtocol: c.proto, ALPN: c.alpn})
			if got != c.want {
				t.Errorf("got %d, want %d", got, c.want)
			}
		})
	}
}
