package model

import (
	"github.com/glebarez/sqlite"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestD2SPurchaseSettingsPersistenceAndAtomicAudit(t *testing.T) {
	for _, engine := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			var dialect gorm.Dialector = sqlite.Open(":memory:")
			if engine != "sqlite" {
				dsn := os.Getenv("D2S_TEST_PURCHASE_" + map[string]string{"mysql": "MYSQL", "postgres": "POSTGRES"}[engine] + "_DSN")
				if dsn == "" {
					t.Skip("isolated database DSN not configured")
				}
				if engine == "mysql" {
					connection, err := mysqldriver.ParseDSN(dsn)
					require.NoError(t, err)
					require.Equal(t, "127.0.0.1:3306", connection.Addr)
					require.True(t, strings.HasPrefix(connection.DBName, "d2s_purchase_probe_"))
					dialect = mysql.Open(dsn)
				} else {
					connection, err := url.Parse(dsn)
					require.NoError(t, err)
					require.Equal(t, "127.0.0.1:15432", connection.Host)
					require.True(t, strings.HasPrefix(strings.TrimPrefix(connection.Path, "/"), "d2s_purchase_probe_"))
					dialect = postgres.Open(dsn)
				}
			}
			db, err := gorm.Open(dialect, &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			previous := DB
			DB = db
			t.Cleanup(func() { DB = previous; _ = sqlDB.Close() })
			require.False(t, db.Migrator().HasTable(&Option{}), "requires an empty dedicated test database")
			require.NoError(t, db.AutoMigrate(&Option{}, &D2SAdminOperationAudit{}))
			t.Setenv("D2S_PURCHASES_ENABLED", "false")
			enabled, err := D2SPurchasesEnabled()
			require.NoError(t, err)
			require.False(t, enabled)
			t.Setenv("D2S_PURCHASES_ENABLED", "true")
			enabled, err = D2SPurchasesEnabled()
			require.NoError(t, err)
			require.True(t, enabled)
			require.Error(t, SetD2SPurchasesEnabled(false, nil))
			enabled, err = D2SPurchasesEnabled()
			require.NoError(t, err)
			require.True(t, enabled, "audit failure must roll back setting")
			for _, value := range []bool{false, true, false} {
				require.NoError(t, SetD2SPurchasesEnabled(value, &D2SAdminOperationAudit{
					OperatorID: 1, OperatorEmail: "admin@example.com", OperationType: "UPDATE_PURCHASE_SETTINGS",
					SourceIP: "127.0.0.1", TargetResource: D2SPurchasesEnabledOption, Result: "SUCCESS",
				}))
				enabled, err = D2SPurchasesEnabled()
				require.NoError(t, err)
				require.Equal(t, value, enabled, "saved setting must override environment immediately")
			}
			var count int64
			require.NoError(t, db.Model(&D2SAdminOperationAudit{}).Count(&count).Error)
			require.EqualValues(t, 3, count)
			var option Option
			require.NoError(t, db.First(&option).Error)
			require.Equal(t, "false", option.Value)
			require.NoError(t, sqlDB.Close())
			enabled, err = D2SPurchasesEnabled()
			require.Error(t, err)
			require.False(t, enabled, "database errors must fail closed")
		})
	}
}
