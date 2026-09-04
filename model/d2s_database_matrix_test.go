package model

import (
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestD2SSchemaConfiguredDatabases(t *testing.T) {
	tests := []struct {
		name    string
		envName string
		dbType  common.DatabaseType
		open    func(string) (*gorm.DB, error)
	}{
		{name: "mysql", envName: "D2S_TEST_MYSQL_DSN", dbType: common.DatabaseTypeMySQL, open: func(dsn string) (*gorm.DB, error) {
			return gorm.Open(mysql.Open(dsn), &gorm.Config{})
		}},
		{name: "postgres", envName: "D2S_TEST_POSTGRES_DSN", dbType: common.DatabaseTypePostgreSQL, open: func(dsn string) (*gorm.DB, error) {
			return gorm.Open(postgres.Open(dsn), &gorm.Config{})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dsn := strings.TrimSpace(os.Getenv(test.envName))
			if dsn == "" {
				t.Skip(test.envName + " is not configured")
			}
			db, err := test.open(dsn)
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { _ = sqlDB.Close() })
			previousDB, previousLogDB := DB, LOG_DB
			previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
			DB, LOG_DB = db, db
			common.SetDatabaseTypes(test.dbType, test.dbType)
			t.Cleanup(func() {
				DB, LOG_DB = previousDB, previousLogDB
				common.SetDatabaseTypes(previousMainType, previousLogType)
			})

			models := []any{
				&User{}, &D2SUserProfile{}, &D2SLicense{}, &D2SDeviceBinding{}, &D2SLicenseEvent{},
				&D2SDeviceCode{}, &D2SOfflineEntitlement{}, &D2SOnlineLease{}, &D2SFreeRevokeCooldown{},
				&D2SPaidRevokeQuota{}, &D2SManualUnbindRequest{}, &D2SOrder{}, &D2SPaymentEvent{},
				&D2SBalanceAccount{}, &D2SBalanceTransaction{}, &D2SInviteReward{}, &D2SWithdrawalRequest{},
				&D2SSigningKey{},
			}
			require.NoError(t, db.AutoMigrate(models...))
			require.NoError(t, db.AutoMigrate(models...), "migration must be idempotent")

			user := User{Username: "matrix-" + test.name, Password: "unused-hash", Email: "matrix-" + test.name + "@example.com", AffCode: "matrix-" + test.name, Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
			require.NoError(t, db.Create(&user).Error)
			licenses, err := ListD2SLicenses(user.Id, 2_000_700_000)
			require.NoError(t, err)
			require.Len(t, licenses, 1)
			bound, err := BindD2SLicense(user.Id, licenses[0].ID, strings.Repeat("d", 64), 1, 2_000_700_001)
			require.NoError(t, err)
			assert.Equal(t, D2SLicenseModeOnline, bound.Mode)
		})
	}
}
