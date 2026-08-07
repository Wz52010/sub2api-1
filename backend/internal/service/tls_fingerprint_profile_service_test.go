//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/model"
)

func TestStableProfileSelectionIsIndependentOfMapOrder(t *testing.T) {
	profilesA := map[int64]*model.TLSFingerprintProfile{
		11: {ID: 11, Name: "profile-11"},
		22: {ID: 22, Name: "profile-22"},
		33: {ID: 33, Name: "profile-33"},
	}
	profilesB := map[int64]*model.TLSFingerprintProfile{
		33: {ID: 33, Name: "profile-33"},
		11: {ID: 11, Name: "profile-11"},
		22: {ID: 22, Name: "profile-22"},
	}

	first := &TLSFingerprintProfileService{localCache: profilesA}
	second := &TLSFingerprintProfileService{localCache: profilesB}
	for _, accountID := range []int64{1, 42, 1001, 922337203685477000} {
		gotA := first.getStableProfile(accountID)
		gotB := second.getStableProfile(accountID)
		if gotA == nil || gotB == nil {
			t.Fatalf("account %d returned nil profile", accountID)
		}
		if gotA.Name != gotB.Name {
			t.Fatalf("account %d changed profile when map order changed: %s != %s", accountID, gotA.Name, gotB.Name)
		}
	}
}

func TestStableProfileSelectionUsesAccountAndProfileIDs(t *testing.T) {
	if stableProfileScore(7, 11) == stableProfileScore(8, 11) {
		t.Fatal("different account IDs should normally produce different rendezvous scores")
	}
	if stableProfileScore(7, 11) == stableProfileScore(7, 12) {
		t.Fatal("different profile IDs should normally produce different rendezvous scores")
	}
}
