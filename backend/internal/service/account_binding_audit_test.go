package service

import "testing"

func TestAccountBindingChangeAuditExtraSummarizesOnlyBindingChanges(t *testing.T) {
	oldProxy := int64(7)
	newProxy := int64(9)
	before := &Account{
		ID:      42,
		ProxyID: &oldProxy,
		Extra: map[string]any{
			"enable_tls_fingerprint":      false,
			"tls_fingerprint_profile_id": float64(3),
			"session_key":                 "must-not-be-copied",
		},
	}
	after := &Account{
		ID:      42,
		ProxyID: &newProxy,
		Extra: map[string]any{
			"enable_tls_fingerprint":      true,
			"tls_fingerprint_profile_id": int64(4),
			"session_key":                 "must-not-be-copied",
		},
	}

	got := AccountBindingChangeAuditExtra(before, after)
	if got[AccountBindingAuditAccountIDKey] != int64(42) {
		t.Fatalf("account id = %#v, want 42", got[AccountBindingAuditAccountIDKey])
	}
	if got[AccountBindingAuditChangeCountKey] != 3 {
		t.Fatalf("change count = %#v, want 3", got[AccountBindingAuditChangeCountKey])
	}
	if got[AccountBindingAuditProxyIDChangeKey] != "7 -> 9" {
		t.Fatalf("proxy change = %#v", got[AccountBindingAuditProxyIDChangeKey])
	}
	if got[AccountBindingAuditTLSFingerprintChangeKey] != "false -> true" {
		t.Fatalf("tls change = %#v", got[AccountBindingAuditTLSFingerprintChangeKey])
	}
	if got[AccountBindingAuditTLSProfileIDChangeKey] != "3 -> 4" {
		t.Fatalf("profile change = %#v", got[AccountBindingAuditTLSProfileIDChangeKey])
	}
	if _, ok := got["session_key"]; ok {
		t.Fatal("audit summary must not include credentials")
	}
}

func TestAccountBindingChangeAuditExtraTreatsAbsentAndFalseAsDifferentStates(t *testing.T) {
	before := &Account{ID: 42, Extra: map[string]any{}}
	after := &Account{ID: 42, Extra: map[string]any{"enable_tls_fingerprint": false}}

	got := AccountBindingChangeAuditExtra(before, after)
	if got[AccountBindingAuditTLSFingerprintChangeKey] != "unset -> false" {
		t.Fatalf("tls change = %#v", got[AccountBindingAuditTLSFingerprintChangeKey])
	}
}

func TestAccountBindingChangeAuditExtraIgnoresUnrelatedChanges(t *testing.T) {
	before := &Account{ID: 42, Extra: map[string]any{"notes": "old"}}
	after := &Account{ID: 42, Extra: map[string]any{"notes": "new"}}

	if got := AccountBindingChangeAuditExtra(before, after); got != nil {
		t.Fatalf("unrelated change produced audit fields: %#v", got)
	}
}
