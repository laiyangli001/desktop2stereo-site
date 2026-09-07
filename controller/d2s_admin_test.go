package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestD2SAdminLicensesIncludesUserRegion(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:d2s_admin_license_region_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
	})
	require.NoError(t, db.AutoMigrate(&model.D2SUserProfile{}, &model.D2SLicense{}))
	require.NoError(t, db.Create(&model.D2SUserProfile{UserID: 701, Region: model.D2SRegionINTL}).Error)
	require.NoError(t, db.Create(&model.D2SUserProfile{UserID: 702, Region: ""}).Error)
	require.NoError(t, db.Create(&model.D2SLicense{ID: "region-license-intl", LicenseCode: "D2S-INTL", UserID: 701, Product: model.D2SProductDesktop2Stereo, Kind: model.D2SLicenseKindPaid, Status: model.D2SLicenseStatusActive, Mode: model.D2SLicenseModeUnbound}).Error)
	require.NoError(t, db.Create(&model.D2SLicense{ID: "region-license-unlocked", LicenseCode: "D2S-UNLOCKED", UserID: 702, Product: model.D2SProductDesktop2Stereo, Kind: model.D2SLicenseKindTrial, Status: model.D2SLicenseStatusActive, Mode: model.D2SLicenseModeUnbound}).Error)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/licenses", nil)
	D2SAdminLicenses(context)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Data struct {
			Licenses []struct {
				UserID int    `json:"user_id"`
				Region string `json:"region"`
			} `json:"licenses"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	regions := map[int]string{}
	for _, license := range response.Data.Licenses {
		regions[license.UserID] = license.Region
	}
	assert.Equal(t, model.D2SRegionINTL, regions[701])
	assert.Equal(t, "", regions[702])
}

func TestD2SAdminSigningKeysMarksCurrentKey(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:d2s_admin_signing_keys_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	previousKeyID := os.Getenv("D2S_LICENSE_KEY_ID")
	t.Setenv("D2S_LICENSE_KEY_ID", "current-admin-key")
	t.Cleanup(func() {
		_ = os.Setenv("D2S_LICENSE_KEY_ID", previousKeyID)
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
	})
	require.NoError(t, db.AutoMigrate(&model.D2SSigningKey{}))
	require.NoError(t, db.Create(&model.D2SSigningKey{KeyID: "current-admin-key", Algorithm: "ES256", PublicJWK: "{}", Status: "active", CreatedAt: 1}).Error)
	require.NoError(t, db.Create(&model.D2SSigningKey{KeyID: "old-admin-key", Algorithm: "ES256", PublicJWK: "{}", Status: "active", CreatedAt: 2}).Error)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/signing-keys", nil)
	D2SAdminSigningKeys(context)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Data struct {
			Keys []struct {
				KeyID     string `json:"key_id"`
				IsCurrent bool   `json:"is_current"`
			} `json:"keys"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	current := map[string]bool{}
	for _, key := range response.Data.Keys {
		current[key.KeyID] = key.IsCurrent
	}
	assert.True(t, current["current-admin-key"])
	assert.False(t, current["old-admin-key"])
	assert.True(t, service.IsD2SCurrentSigningKey("current-admin-key"))
}

func TestD2SAdminBalancesDefaultsToNegativeAccounts(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:d2s_admin_balances_default_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
	})
	require.NoError(t, db.AutoMigrate(&model.D2SBalanceAccount{}))
	require.NoError(t, db.Create(&model.D2SBalanceAccount{ID: "negative-balance", UserID: 801, Currency: "USD", AvailableMinor: -140}).Error)
	require.NoError(t, db.Create(&model.D2SBalanceAccount{ID: "positive-balance", UserID: 802, Currency: "USD", AvailableMinor: 140}).Error)

	call := func(path string) []model.D2SBalanceAccount {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = httptest.NewRequest(http.MethodGet, path, nil)
		D2SAdminBalances(context)
		assert.Equal(t, http.StatusOK, recorder.Code)
		var response struct {
			Data struct {
				Accounts []model.D2SBalanceAccount `json:"accounts"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
		return response.Data.Accounts
	}

	defaultAccounts := call("/api/v1/admin/balances")
	require.Len(t, defaultAccounts, 1)
	assert.Equal(t, "negative-balance", defaultAccounts[0].ID)

	allAccounts := call("/api/v1/admin/balances?negative=false")
	assert.Len(t, allAccounts, 2)
}
