package service

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestD2SDefaultLicenseKeyIDMatchesReleaseContract(t *testing.T) {
	t.Setenv("D2S_LICENSE_KEY_ID", "")
	assert.Equal(t, "d2s-es256-2026-09", d2sConfiguredKeyID())
	assert.Equal(t, D2SDefaultLicenseKeyID, d2sConfiguredKeyID())
}

func TestD2SOfflineEntitlementIsValidES256JWS(t *testing.T) {
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.D2SUserProfile{}, &model.D2SLicense{}, &model.D2SDeviceBinding{}, &model.D2SLicenseEvent{}, &model.D2SOfflineEntitlement{}, &model.D2SOnlineLease{}, &model.D2SSigningKey{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sqlDB.Close()
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
	})

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	encoded, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	t.Setenv("D2S_LICENSE_PRIVATE_KEY_PEM", string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded})))
	t.Setenv("D2S_LICENSE_KEY_ID", "test-key")
	user := model.User{Username: "signing-user", Password: "unused-hash", Email: "signing@example.com", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, db.Create(&user).Error)
	const now = int64(2_000_600_000)
	licenses, err := model.ListD2SLicenses(user.Id, now)
	require.NoError(t, err)
	device := strings.Repeat("c", 64)
	_, err = model.BindD2SLicense(user.Id, licenses[0].ID, device, 1, now)
	require.NoError(t, err)
	_, err = model.ChangeD2SLicenseMode(user.Id, licenses[0].ID, device, model.D2SLicenseModeOffline, "", 14, now)
	require.NoError(t, err)

	jws, claims, err := IssueD2SOfflineEntitlement(user.Id, licenses[0].ID, device, 14, now)
	require.NoError(t, err)
	assert.Equal(t, "test-key", claims.KeyID)
	assert.Equal(t, now+14*86400, claims.ExpiresAt)
	oldKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	oldJWK, err := common.Marshal(d2sPublicJWK(oldKey, "old-key"))
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.D2SSigningKey{
		KeyID: "old-key", Algorithm: "ES256", PublicJWK: string(oldJWK), Status: "active", CreatedAt: now - 100,
	}).Error)
	keys, err := D2SPublicSigningKeys()
	require.NoError(t, err)
	keyIDs := make(map[string]bool, len(keys))
	for _, publicKey := range keys {
		keyIDs[publicKey.Kid] = true
	}
	assert.True(t, keyIDs["test-key"])
	assert.True(t, keyIDs["old-key"])
	require.NoError(t, db.Model(&model.D2SSigningKey{}).Where("key_id = ?", "old-key").Update("status", "retired").Error)
	keys, err = D2SPublicSigningKeys()
	require.NoError(t, err)
	keyIDs = make(map[string]bool, len(keys))
	for _, publicKey := range keys {
		keyIDs[publicKey.Kid] = true
	}
	assert.False(t, keyIDs["old-key"])
	parts := strings.Split(jws, ".")
	require.Len(t, parts, 3)
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	require.NoError(t, err)
	require.Len(t, signature, 64)
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:])
	assert.True(t, ecdsa.Verify(&key.PublicKey, digest[:], r, s))
}

func TestD2SSigningKeyAcceptsBase64EncodedPKCS8DER(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	t.Setenv("D2S_LICENSE_PRIVATE_KEY_PEM", "")
	t.Setenv("D2S_LICENSE_PRIVATE_KEY_B64", base64.StdEncoding.EncodeToString(der))
	t.Setenv("D2S_LICENSE_KEY_ID", "base64-der-key")

	parsed, keyID, err := d2sSigningKey()
	require.NoError(t, err)
	assert.Equal(t, "base64-der-key", keyID)
	assert.Equal(t, key.PublicKey.X, parsed.PublicKey.X)
	assert.Equal(t, key.PublicKey.Y, parsed.PublicKey.Y)
}

func TestD2SRetireSigningKeyProtectsCurrentKey(t *testing.T) {
	previousDB := model.DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.D2SSigningKey{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sqlDB.Close()
		model.DB = previousDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
	})
	t.Setenv("D2S_LICENSE_KEY_ID", "current-key")
	assert.True(t, IsD2SCurrentSigningKey("current-key"))
	assert.False(t, IsD2SCurrentSigningKey("old-key"))
	require.NoError(t, db.Create(&model.D2SSigningKey{
		KeyID: "current-key", Algorithm: "ES256", PublicJWK: "{}", Status: "active", CreatedAt: 1,
	}).Error)
	require.NoError(t, db.Create(&model.D2SSigningKey{
		KeyID: "old-key", Algorithm: "ES256", PublicJWK: "{}", Status: "active", CreatedAt: 1,
	}).Error)

	err = RetireD2SSigningKey("current-key", 10)
	require.ErrorIs(t, err, ErrD2SSigningKeyCurrent)
	require.NoError(t, RetireD2SSigningKey("old-key", 11))
	require.ErrorIs(t, RetireD2SSigningKey("missing-key", 12), ErrD2SSigningKeyNotFound)
	var oldKey model.D2SSigningKey
	require.NoError(t, db.Where("key_id = ?", "old-key").First(&oldKey).Error)
	assert.Equal(t, "retired", oldKey.Status)
	assert.Equal(t, int64(11), oldKey.RetiredAt)
}
