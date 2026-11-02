package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	dialect := strings.ToLower(strings.TrimSpace(os.Getenv("D2S_AUDIT_MIGRATION_DIALECT")))
	migrationDSN := strings.TrimSpace(os.Getenv("D2S_AUDIT_MIGRATION_DSN"))
	primaryDSN := strings.TrimSpace(os.Getenv("SQL_DSN"))
	if migrationDSN == "" {
		log.Fatal("D2S_AUDIT_MIGRATION_DSN is required")
	}
	if primaryDSN == "" {
		log.Fatal("SQL_DSN is required to verify the audit migration target")
	}

	switch dialect {
	case "mysql", "postgres", "postgresql":
	default:
		log.Fatal("D2S_AUDIT_MIGRATION_DIALECT must be mysql or postgres")
	}

	migrationDB, err := openAuditDatabase(dialect, migrationDSN)
	if err != nil {
		log.Fatalf("failed to open audit migration database: %v", err)
	}
	migrationSQLDB, err := migrationDB.DB()
	if err != nil {
		log.Fatalf("failed to access audit migration connection: %v", err)
	}
	defer migrationSQLDB.Close()

	primaryDB, err := openAuditDatabase(dialect, primaryDSN)
	if err != nil {
		log.Fatalf("failed to open primary database for scope verification: %v", err)
	}
	primarySQLDB, err := primaryDB.DB()
	if err != nil {
		log.Fatalf("failed to access primary database connection: %v", err)
	}
	defer primarySQLDB.Close()

	migrationScope, err := auditDatabaseScope(migrationDB, dialect)
	if err != nil {
		log.Fatalf("failed to inspect audit migration database scope: %v", err)
	}
	primaryScope, err := auditDatabaseScope(primaryDB, dialect)
	if err != nil {
		log.Fatalf("failed to inspect primary database scope: %v", err)
	}
	if migrationScope == "" || migrationScope != primaryScope {
		log.Fatal("D2S_AUDIT_MIGRATION_DSN must target the same database and schema as SQL_DSN")
	}

	if err := migrationDB.AutoMigrate(
		&model.D2SUserActionLog{},
		&model.D2SRegistrationRecord{},
		&model.D2SAdminOperationAudit{},
		&model.D2SAuditRetentionRun{},
	); err != nil {
		log.Fatalf("failed to migrate D2S audit tables: %v", err)
	}

	log.Printf("D2S audit tables migrated with dialect %s", dialect)
}

func openAuditDatabase(dialect, dsn string) (*gorm.DB, error) {
	switch dialect {
	case "mysql":
		return gorm.Open(mysql.Open(dsn), &gorm.Config{})
	case "postgres", "postgresql":
		return gorm.Open(postgres.Open(dsn), &gorm.Config{})
	default:
		return nil, fmt.Errorf("unsupported audit migration dialect %q", dialect)
	}
}

func auditDatabaseScope(db *gorm.DB, dialect string) (string, error) {
	query := "SELECT DATABASE()"
	if dialect == "postgres" || dialect == "postgresql" {
		query = "SELECT current_database() || '.' || current_schema()"
	}
	var scope string
	if err := db.Raw(query).Scan(&scope).Error; err != nil {
		return "", err
	}
	return strings.TrimSpace(scope), nil
}
