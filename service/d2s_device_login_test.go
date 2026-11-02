package service

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestExchangeD2SDeviceCodeRejectsLoginBlockedAccount(t *testing.T) {
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedisEnabled := common.RedisEnabled
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.UserSession{},
		&model.D2SUserProfile{},
		&model.D2SLicense{},
		&model.D2SLicenseEvent{},
		&model.D2SDeviceCode{},
		&model.D2SUserSecurityControl{},
	))
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.RedisEnabled = previousRedisEnabled
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	user := model.User{
		Username: "blocked-device-user", Email: "blocked-device@example.com",
		EmailVerifiedAt: 2_000_000_000, Status: common.UserStatusEnabled, AuthVersion: 1,
	}
	require.NoError(t, db.Create(&user).Error)

	device, secret, err := model.CreateD2SDeviceCode(strings.Repeat("a", 64), 1, "Test client", "windows", 2_000_000_001)
	require.NoError(t, err)
	_, err = model.ApproveD2SDeviceCode(user.Id, device.UserCode, 2_000_000_002)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.D2SUserSecurityControl{
		UserID: user.Id, LoginBlocked: true, UpdatedAt: 2_000_000_003,
	}).Error)

	credentials, err := ExchangeD2SDeviceCode(secret, "192.0.2.30", "D2S test client", 2_000_000_004)

	require.ErrorIs(t, err, model.ErrD2SAccountLoginBlocked)
	require.Nil(t, credentials)
	var sessionCount int64
	require.NoError(t, db.Model(&model.UserSession{}).Where("user_id = ?", user.Id).Count(&sessionCount).Error)
	require.Zero(t, sessionCount)

	var saved model.D2SDeviceCode
	require.NoError(t, db.Where("id = ?", device.ID).First(&saved).Error)
	require.Equal(t, model.D2SDeviceCodeConsumed, saved.Status)

	_, err = ExchangeD2SDeviceCode(secret, "192.0.2.30", "D2S test client", 2_000_000_005)
	require.ErrorIs(t, err, model.ErrD2SDeviceCodeConsumed)
}
