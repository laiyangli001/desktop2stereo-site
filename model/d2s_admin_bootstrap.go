package model

import (
	"errors"
	"os"
	"strings"
	"unicode"

	"github.com/QuantumNous/new-api/common"
)

func ValidD2SAdminPassword(password string) bool {
	if len(password) < 12 || len(password) > 72 {
		return false
	}
	var lower, upper, digit, special bool
	for _, char := range password {
		switch {
		case unicode.IsLower(char):
			lower = true
		case unicode.IsUpper(char):
			upper = true
		case unicode.IsDigit(char):
			digit = true
		default:
			special = true
		}
	}
	classCount := 0
	for _, present := range []bool{lower, upper, digit, special} {
		if present {
			classCount++
		}
	}
	return classCount >= 3
}

// BootstrapD2SAdminFromEnvironment creates the first independent superadmin only.
// The environment variables should be removed from the deployment after this one-time bootstrap.
func BootstrapD2SAdminFromEnvironment() error {
	email := NormalizeEmail(os.Getenv("D2S_ADMIN_BOOTSTRAP_EMAIL"))
	password := os.Getenv("D2S_ADMIN_BOOTSTRAP_PASSWORD")
	if email == "" && password == "" {
		return nil
	}
	if email == "" || common.Validate.Var(email, "required,email") != nil || !ValidD2SAdminPassword(password) {
		return errors.New("invalid D2S admin bootstrap credentials")
	}
	count, err := CountD2SAdminAccounts()
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	passwordHash, err := common.Password2Hash(password)
	if err != nil {
		return err
	}
	_, err = CreateInitialD2SAdminAccount(email, passwordHash, 0)
	if err != nil && !errors.Is(err, ErrD2SAdminExists) {
		return err
	}
	return nil
}

func NormalizeD2SAdminRole(role string) string {
	return strings.ToLower(strings.TrimSpace(role))
}
