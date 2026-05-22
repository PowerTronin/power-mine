package account

import (
	"testing"

	"power-mine/internal/domain"
)

func TestNormalizeOfflineAccount(t *testing.T) {
	got := Normalize(domain.AccountConfig{Mode: domain.AccountOffline, OfflineName: "Local_Player"})
	if got.OfflineName != "Local_Player" {
		t.Fatalf("OfflineName = %q, want Local_Player", got.OfflineName)
	}
	if got.OfflineUUID != OfflineUUID("Local_Player") {
		t.Fatalf("OfflineUUID = %q, want deterministic UUID", got.OfflineUUID)
	}
}

func TestValidateRejectsInvalidOfflineName(t *testing.T) {
	if err := Validate(domain.AccountConfig{Mode: domain.AccountOffline, OfflineName: "bad name"}); err == nil {
		t.Fatal("Validate returned nil, want error")
	}
}
