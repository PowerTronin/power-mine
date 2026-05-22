package account

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"power-mine/internal/domain"
)

var offlineNamePattern = regexp.MustCompile(`^[A-Za-z0-9_]{3,16}$`)

func Default() domain.AccountConfig {
	return Normalize(domain.AccountConfig{
		Mode:        domain.AccountOffline,
		OfflineName: "Player",
	})
}

func Normalize(account domain.AccountConfig) domain.AccountConfig {
	account.Mode = domain.AccountMode(strings.ToLower(strings.TrimSpace(string(account.Mode))))
	if account.Mode == "" {
		account.Mode = domain.AccountOffline
	}

	if account.Mode != domain.AccountOffline {
		return account
	}

	account.OfflineName = strings.TrimSpace(account.OfflineName)
	if account.OfflineName == "" {
		account.OfflineName = "Player"
	}
	account.OfflineUUID = OfflineUUID(account.OfflineName)
	return account
}

func Validate(account domain.AccountConfig) error {
	account = Normalize(account)
	switch account.Mode {
	case domain.AccountOffline:
		if !offlineNamePattern.MatchString(account.OfflineName) {
			return fmt.Errorf("offline name must be 3-16 characters and contain only letters, numbers, or underscore")
		}
	case domain.AccountMicrosoft:
		return fmt.Errorf("microsoft account mode is not implemented yet")
	default:
		return fmt.Errorf("unsupported account mode %q", account.Mode)
	}
	return nil
}

func OfflineUUID(name string) string {
	sum := md5.Sum([]byte("OfflinePlayer:" + name))
	sum[6] = (sum[6] & 0x0f) | 0x30
	sum[8] = (sum[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(sum[:])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:]
}
