package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func main() {
	if strings.TrimSpace(os.Getenv("SQL_DSN")) == "" {
		log.Fatal("SQL_DSN is required; refusing to provision against the default local database")
	}

	operatorEmail := strings.TrimSpace(takeEnv("D2S_ADMIN_PROVISION_OPERATOR_EMAIL"))
	operatorPassword := takeEnvRaw("D2S_ADMIN_PROVISION_OPERATOR_PASSWORD")
	targetEmail := strings.TrimSpace(takeEnv("D2S_ADMIN_PROVISION_EMAIL"))
	targetPassword := takeEnvRaw("D2S_ADMIN_PROVISION_PASSWORD")
	targetRole := strings.TrimSpace(takeEnv("D2S_ADMIN_PROVISION_ROLE"))
	if operatorEmail == "" || operatorPassword == "" || targetEmail == "" || targetPassword == "" {
		log.Fatal("verified operator and new account credentials are required in D2S_ADMIN_PROVISION_* variables")
	}
	if targetRole == "" {
		targetRole = model.D2SAdminRoleAuditReader
	}
	if common.Validate.Var(targetEmail, "required,email") != nil || !model.ValidD2SAdminPassword(targetPassword) {
		log.Fatal("new administrator email or password does not meet policy")
	}

	if err := model.InitDB(); err != nil {
		log.Fatal("database initialization failed; verify database availability and schema")
	}
	operator, err := model.GetD2SAdminAccountByEmail(operatorEmail)
	if err != nil || operator.Role != model.D2SAdminRoleSuperAdmin ||
		operator.Status != model.D2SAdminStatusActive || !model.VerifyD2SAdminPassword(operatorPassword, operator.PasswordHash) {
		log.Fatal("operator verification failed; an active super administrator is required")
	}

	passwordHash, err := common.Password2Hash(targetPassword)
	if err != nil {
		log.Fatal("could not hash the new administrator password")
	}
	account, err := model.CreateD2SAdminAccountWithAudit(
		targetEmail, passwordHash, targetRole, operator.ID, operator.Email, 0,
	)
	if err != nil {
		log.Fatalf("administrator account creation failed: %v", err)
	}
	fmt.Printf("Created D2S administrator account %s with role %s.\n", account.Email, account.Role)
}

func takeEnv(key string) string {
	value := takeEnvRaw(key)
	return strings.TrimSpace(value)
}

func takeEnvRaw(key string) string {
	value := os.Getenv(key)
	_ = os.Unsetenv(key)
	return value
}
