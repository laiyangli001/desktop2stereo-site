package model

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// This schema matches production commit e9c948d before encrypted payment settings.
type d2SAlipaySettingsBeforeConfig struct {
	ID        string `gorm:"type:varchar(32);primaryKey"`
	QRPayMode string `gorm:"type:char(1);not null;default:'4'"`
	UpdatedAt int64  `gorm:"type:bigint;not null"`
}

func (d2SAlipaySettingsBeforeConfig) TableName() string { return "d2s_alipay_pc_settings" }

type d2SAlipayMigrationRecorder struct {
	logger.Interface
	ddl []string
}

func (r *d2SAlipayMigrationRecorder) Trace(_ context.Context, _ time.Time, sql func() (string, int64), _ error) {
	statement, _ := sql()
	normalized := strings.ToUpper(strings.TrimSpace(statement))
	for _, prefix := range []string{"CREATE ", "ALTER ", "DROP ", "RENAME "} {
		if strings.HasPrefix(normalized, prefix) {
			r.ddl = append(r.ddl, statement)
			break
		}
	}
}

func TestD2SAlipaySettingsMySQLMigration(t *testing.T) {
	for _, scenario := range []struct {
		name string
		env  string
	}{
		{"fresh", "D2S_TEST_ALIPAY_MYSQL_FRESH_DSN"},
		{"upgrade", "D2S_TEST_ALIPAY_MYSQL_UPGRADE_DSN"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			dsn := os.Getenv(scenario.env)
			if dsn == "" {
				t.Skip(scenario.env + " is not configured")
			}
			connection, err := mysqldriver.ParseDSN(dsn)
			require.NoError(t, err)
			require.Equal(t, "tcp", connection.Net)
			require.Equal(t, "127.0.0.1:3306", connection.Addr)
			require.True(t, strings.HasPrefix(connection.DBName, "d2s_alipay_migration_"), "only isolated local migration databases are allowed")

			recorder := &d2SAlipayMigrationRecorder{Interface: logger.Discard}
			db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: recorder})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			previousDB := DB
			DB = db
			t.Cleanup(func() {
				DB = previousDB
				_ = sqlDB.Close()
			})
			require.False(t, db.Migrator().HasTable(&D2SAlipayPCSettings{}), "the test requires a fresh dedicated database")

			var version string
			require.NoError(t, db.Raw("SELECT VERSION()").Scan(&version).Error)
			t.Logf("engine=%s database=%s scenario=%s", version, connection.DBName, scenario.name)

			if scenario.name == "upgrade" {
				require.NoError(t, db.AutoMigrate(&d2SAlipaySettingsBeforeConfig{}))
				require.NoError(t, db.Create(&d2SAlipaySettingsBeforeConfig{
					ID: "default", QRPayMode: "1", UpdatedAt: 1793700000,
				}).Error)
			}
			require.NoError(t, db.AutoMigrate(&D2SAlipayPCSettings{}))
			if scenario.name == "upgrade" {
				preserved, err := GetD2SAlipayPCSettings()
				require.NoError(t, err)
				assert.Equal(t, "1", preserved.QRPayMode)
				assert.EqualValues(t, 1793700000, preserved.UpdatedAt)
				assert.False(t, preserved.ConfigManaged)
				assert.False(t, preserved.ChannelEnabled)
				assert.Empty(t, preserved.PrivateKeyCiphertext)
				assert.Empty(t, preserved.PublicKeyCiphertext)
			}
			recorder.ddl = nil
			require.NoError(t, db.AutoMigrate(&D2SAlipayPCSettings{}))
			require.Empty(t, recorder.ddl, "a repeated migration must not issue DDL")

			settings := &D2SAlipayPCSettings{
				ID: "default", QRPayMode: "4", ConfigManaged: true, ChannelEnabled: true,
				AppID: "2026000000000000", SellerID: "2088000000000000",
				GatewayURL: "https://openapi.alipay.com/gateway.do",
				NotifyURL:  "https://d2s.site/api/v1/alipay/d2s/notify",
				ReturnURL:  "https://d2s.site/d2s", UpdatedAt: 1793700100,
				PrivateKeyCiphertext: "v1:synthetic-private-ciphertext",
				PublicKeyCiphertext:  "v1:synthetic-public-ciphertext",
			}
			require.NoError(t, SaveD2SAlipayPCSettings(settings))
			actual, err := GetD2SAlipayPCSettings()
			require.NoError(t, err)
			require.Equal(t, settings, actual)

			settings.ConfigManaged, settings.ChannelEnabled = false, false
			settings.UpdatedAt++
			require.NoError(t, SaveD2SAlipayPCSettings(settings))
			actual, err = GetD2SAlipayPCSettings()
			require.NoError(t, err)
			require.Equal(t, settings, actual, "upserts must persist false values")

			settings.ConfigManaged, settings.ChannelEnabled = true, true
			require.NoError(t, SaveD2SAlipayPCSettings(settings))
			_, err = SetD2SAlipayPCQRPayMode("3", 1793700200)
			require.NoError(t, err)
			settings.QRPayMode, settings.UpdatedAt = "3", 1793700200
			actual, err = GetD2SAlipayPCSettings()
			require.NoError(t, err)
			require.Equal(t, settings, actual, "QR-only updates must preserve payment settings and ciphertext")

			var tableName, before, after string
			require.NoError(t, db.Raw("SHOW CREATE TABLE d2s_alipay_pc_settings").Row().Scan(&tableName, &before))
			recorder.ddl = nil
			require.NoError(t, db.AutoMigrate(&D2SAlipayPCSettings{}))
			require.Empty(t, recorder.ddl)
			require.NoError(t, db.Raw("SHOW CREATE TABLE d2s_alipay_pc_settings").Row().Scan(&tableName, &after))
			require.Equal(t, before, after, "schema and constraints must remain stable")
			actual, err = GetD2SAlipayPCSettings()
			require.NoError(t, err)
			require.Equal(t, settings, actual, "repeat migration must preserve all stored values")
			require.Error(t, db.Create(&D2SAlipayPCSettings{ID: "default", QRPayMode: "0", UpdatedAt: 1}).Error, "primary key must remain unique")
			var rows int64
			require.NoError(t, db.Model(&D2SAlipayPCSettings{}).Count(&rows).Error)
			require.EqualValues(t, 1, rows)
			t.Log("PASS: migration, repeated migration without DDL, data preservation, boolean upserts, QR-only updates, primary-key uniqueness")
		})
	}
}
