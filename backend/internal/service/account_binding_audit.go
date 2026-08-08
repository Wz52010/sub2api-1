package service

import (
	"encoding/json"
	"math"
	"strconv"
)

const (
	AccountBindingAuditAccountIDKey             = "account_id"
	AccountBindingAuditChangeCountKey           = "binding_change_count"
	AccountBindingAuditProxyIDChangeKey         = "proxy_id_change"
	AccountBindingAuditTLSFingerprintChangeKey  = "tls_fingerprint_enabled_change"
	AccountBindingAuditTLSProfileIDChangeKey    = "tls_fingerprint_profile_id_change"
)

// AccountBindingChangeAuditExtra returns a small, credential-free summary of
// the account fields that affect proxy routing and TLS fingerprint selection.
// The middleware applies a second allowlist before persistence.
func AccountBindingChangeAuditExtra(before, after *Account) map[string]any {
	if before == nil || after == nil || before.ID != after.ID {
		return nil
	}

	changes := make(map[string]any, 5)
	changes[AccountBindingAuditAccountIDKey] = after.ID
	if beforeProxy, afterProxy := auditOptionalID(before.ProxyID), auditOptionalID(after.ProxyID); beforeProxy != afterProxy {
		changes[AccountBindingAuditProxyIDChangeKey] = beforeProxy + " -> " + afterProxy
	}
	if beforeTLS, afterTLS := auditExtraBool(before, "enable_tls_fingerprint"), auditExtraBool(after, "enable_tls_fingerprint"); beforeTLS != afterTLS {
		changes[AccountBindingAuditTLSFingerprintChangeKey] = beforeTLS + " -> " + afterTLS
	}
	if beforeProfile, afterProfile := auditExtraProfileID(before), auditExtraProfileID(after); beforeProfile != afterProfile {
		changes[AccountBindingAuditTLSProfileIDChangeKey] = beforeProfile + " -> " + afterProfile
	}

	if len(changes) == 1 {
		return nil
	}
	changes[AccountBindingAuditChangeCountKey] = len(changes) - 1
	return changes
}

// AccountBindingAuditSnapshot copies only the fields used by
// AccountBindingChangeAuditExtra, so later service mutations cannot alter the
// "before" side of the audit comparison.
func AccountBindingAuditSnapshot(account *Account) *Account {
	if account == nil {
		return nil
	}
	snapshot := &Account{ID: account.ID}
	if account.ProxyID != nil {
		proxyID := *account.ProxyID
		snapshot.ProxyID = &proxyID
	}
	snapshot.Extra = make(map[string]any, 2)
	for _, key := range []string{"enable_tls_fingerprint", "tls_fingerprint_profile_id"} {
		if value, ok := account.Extra[key]; ok {
			snapshot.Extra[key] = value
		}
	}
	return snapshot
}

func auditOptionalID(value *int64) string {
	if value == nil {
		return "null"
	}
	return strconv.FormatInt(*value, 10)
}

func auditExtraBool(account *Account, key string) string {
	if account == nil || account.Extra == nil {
		return "unset"
	}
	raw, ok := account.Extra[key]
	if !ok {
		return "unset"
	}
	value, ok := raw.(bool)
	if !ok {
		return "invalid"
	}
	return strconv.FormatBool(value)
}

func auditExtraProfileID(account *Account) string {
	if account == nil || account.Extra == nil {
		return "unset"
	}
	raw, ok := account.Extra["tls_fingerprint_profile_id"]
	if !ok {
		return "unset"
	}
	if raw == nil {
		return "null"
	}
	value, ok := auditInteger(raw)
	if !ok {
		return "invalid"
	}
	return strconv.FormatInt(value, 10)
}

func auditInteger(value any) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int8:
		return int64(v), true
	case int16:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case uint:
		if uint64(v) > math.MaxInt64 {
			return 0, false
		}
		return int64(v), true
	case uint8:
		return int64(v), true
	case uint16:
		return int64(v), true
	case uint32:
		return int64(v), true
	case uint64:
		if v > math.MaxInt64 {
			return 0, false
		}
		return int64(v), true
	case float32:
		return auditFloatInteger(float64(v))
	case float64:
		return auditFloatInteger(v)
	case json.Number:
		v, err := v.Int64()
		return v, err == nil
	default:
		return 0, false
	}
}

func auditFloatInteger(value float64) (int64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value || value < math.MinInt64 || value > math.MaxInt64 {
		return 0, false
	}
	return int64(value), true
}
