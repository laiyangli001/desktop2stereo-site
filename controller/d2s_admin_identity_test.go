package controller

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func d2SIdentityReviewRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/users/:id/identity", func(c *gin.Context) {
		c.Set("d2s_admin_id", 17)
		c.Set("d2s_admin_email", "reviewer@example.com")
		D2SAdminGetIdentityVerification(c)
	})
	router.PUT("/users/:id/identity", func(c *gin.Context) {
		c.Set("d2s_admin_id", 17)
		c.Set("d2s_admin_email", "reviewer@example.com")
		D2SAdminReviewIdentityVerification(c)
	})
	return router
}

func TestD2SAdminIdentityReadRejectsLegacyUnkeyedHash(t *testing.T) {
	db := setupD2SEmergencyControlTest(t, false)
	require.NoError(t, db.AutoMigrate(&model.D2SIdentityVerification{}))
	legacyHash := sha256.Sum256([]byte("P12345678"))
	record := model.D2SIdentityVerification{
		UserID: 1901, IDDocumentType: "PASSPORT", IDDocumentNumberHash: hex.EncodeToString(legacyHash[:]),
		IDDocumentCountry: "US", Status: "VERIFIED", VerifiedAt: 123, VerifiedBy: 9,
	}
	require.NoError(t, db.Create(&record).Error)

	request := httptest.NewRequest(http.MethodGet, "/users/1901/identity", nil)
	request.RemoteAddr = "192.0.2.10:43123"
	recorder := httptest.NewRecorder()
	d2SIdentityReviewRouter().ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"id_verification_status":"PENDING"`)
	require.NotContains(t, recorder.Body.String(), `"id_verification_status":"VERIFIED"`)
	require.NotContains(t, recorder.Body.String(), record.IDDocumentNumberHash)
	var audits []model.D2SAdminOperationAudit
	require.NoError(t, db.Where("operation_type = ?", "VIEW_IDENTITY").Find(&audits).Error)
	require.Len(t, audits, 1)
	require.Contains(t, audits[0].DetailsJSON, `"legacy_hash_rejected":true`)
	require.Contains(t, audits[0].DetailsJSON, `"legacy_hash_cleared":true`)
	var persisted model.D2SIdentityVerification
	require.NoError(t, db.First(&persisted, "user_id = ?", 1901).Error)
	require.Equal(t, "PENDING", persisted.Status)
	require.Empty(t, persisted.IDDocumentNumberHash)
	require.Zero(t, persisted.VerifiedAt)
	require.Zero(t, persisted.VerifiedBy)
}

func TestD2SAdminIdentityReadKeepsLegacyHashWhenAuditCannotCommit(t *testing.T) {
	db := setupD2SEmergencyControlTest(t, false)
	require.NoError(t, db.AutoMigrate(&model.D2SIdentityVerification{}))
	legacyHash := sha256.Sum256([]byte("P12345678"))
	record := model.D2SIdentityVerification{
		UserID: 1901, IDDocumentNumberHash: hex.EncodeToString(legacyHash[:]), Status: "VERIFIED", VerifiedAt: 123, VerifiedBy: 9,
	}
	require.NoError(t, db.Create(&record).Error)
	require.NoError(t, db.Exec(`CREATE TRIGGER reject_legacy_hash_audit
		BEFORE INSERT ON d2s_admin_operation_audits
		BEGIN SELECT RAISE(FAIL, 'audit unavailable'); END`).Error)

	request := httptest.NewRequest(http.MethodGet, "/users/1901/identity", nil)
	request.RemoteAddr = "192.0.2.10:43123"
	recorder := httptest.NewRecorder()
	d2SIdentityReviewRouter().ServeHTTP(recorder, request)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.Contains(t, recorder.Body.String(), "IDENTITY_MIGRATION_FAILED")
	var persisted model.D2SIdentityVerification
	require.NoError(t, db.First(&persisted, "user_id = ?", 1901).Error)
	require.Equal(t, "VERIFIED", persisted.Status)
	require.Equal(t, hex.EncodeToString(legacyHash[:]), persisted.IDDocumentNumberHash)
}

func TestD2SAdminIdentityReviewCommitsAuditWithHashedDocument(t *testing.T) {
	const key = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	t.Setenv("D2S_ID_DOCUMENT_HMAC_KEY", key)
	db := setupD2SEmergencyControlTest(t, false)
	require.NoError(t, db.AutoMigrate(&model.D2SIdentityVerification{}))

	request := httptest.NewRequest(http.MethodPut, "/users/1901/identity", strings.NewReader(
		`{"id_document_type":"PASSPORT","id_document_number":"P12345678","id_document_country":"US","id_verification_status":"VERIFIED"}`,
	))
	request.RemoteAddr = "192.0.2.10:43123"
	recorder := httptest.NewRecorder()
	d2SIdentityReviewRouter().ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var record model.D2SIdentityVerification
	require.NoError(t, db.Where("user_id = ?", 1901).First(&record).Error)
	require.Equal(t, "VERIFIED", record.Status)
	require.Len(t, record.IDDocumentNumberHash, 64)
	require.Equal(t, d2SIdentityHashVersion, record.IDDocumentNumberHashVersion)
	decodedKey, err := hex.DecodeString(key)
	require.NoError(t, err)
	mac := hmac.New(sha256.New, decodedKey)
	_, _ = mac.Write([]byte("d2s-identity-document-v1\x00P12345678"))
	require.Equal(t, hex.EncodeToString(mac.Sum(nil)), record.IDDocumentNumberHash)
	require.NotContains(t, record.IDDocumentNumberHash, "P12345678")
	require.Equal(t, 17, record.VerifiedBy)
	var audits []model.D2SAdminOperationAudit
	require.NoError(t, db.Where("operation_type = ?", "REVIEW_IDENTITY").Find(&audits).Error)
	require.Len(t, audits, 1)
	require.Equal(t, "SUCCESS", audits[0].Result)
	require.NotContains(t, audits[0].DetailsJSON, "P12345678")
}

func TestD2SAdminIdentityReviewRollsBackWhenAuditInsertFails(t *testing.T) {
	t.Setenv("D2S_ID_DOCUMENT_HMAC_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	db := setupD2SEmergencyControlTest(t, false)
	require.NoError(t, db.AutoMigrate(&model.D2SIdentityVerification{}))
	require.NoError(t, db.Exec(`CREATE TRIGGER reject_admin_audit
		BEFORE INSERT ON d2s_admin_operation_audits
		BEGIN SELECT RAISE(FAIL, 'audit unavailable'); END`).Error)

	request := httptest.NewRequest(http.MethodPut, "/users/1901/identity", strings.NewReader(
		`{"id_document_type":"PASSPORT","id_document_number":"P12345678","id_document_country":"US","id_verification_status":"VERIFIED"}`,
	))
	request.RemoteAddr = "192.0.2.10:43123"
	recorder := httptest.NewRecorder()
	d2SIdentityReviewRouter().ServeHTTP(recorder, request)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	var record model.D2SIdentityVerification
	err := db.Where("user_id = ?", 1901).First(&record).Error
	require.Error(t, err)
	require.Empty(t, record.IDDocumentNumberHash)
}

func TestD2SAdminIdentityReviewFailsClosedWhenHashKeyIsUnavailable(t *testing.T) {
	db := setupD2SEmergencyControlTest(t, false)
	require.NoError(t, db.AutoMigrate(&model.D2SIdentityVerification{}))

	for _, key := range []string{"", "not-hex", "abcd"} {
		t.Setenv("D2S_ID_DOCUMENT_HMAC_KEY", key)
		request := httptest.NewRequest(http.MethodPut, "/users/1901/identity", strings.NewReader(
			`{"id_document_type":"PASSPORT","id_document_number":"P12345678","id_document_country":"US","id_verification_status":"VERIFIED"}`,
		))
		request.RemoteAddr = "192.0.2.10:43123"
		recorder := httptest.NewRecorder()
		d2SIdentityReviewRouter().ServeHTTP(recorder, request)

		require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
		require.Contains(t, recorder.Body.String(), "IDENTITY_HASH_KEY_UNAVAILABLE")
		var record model.D2SIdentityVerification
		require.Error(t, db.Where("user_id = ?", 1901).First(&record).Error)
	}
	var audits []model.D2SAdminOperationAudit
	require.NoError(t, db.Where("operation_type = ? AND result = ?", "REVIEW_IDENTITY", "FAILURE").Find(&audits).Error)
	require.Len(t, audits, 3)
	for _, audit := range audits {
		require.NotContains(t, audit.DetailsJSON, "P12345678")
	}
}

func TestD2SAdminIdentityReviewFailsClosedWhenFailureAuditCannotBeWritten(t *testing.T) {
	db := setupD2SEmergencyControlTest(t, false)
	require.NoError(t, db.Migrator().DropTable(&model.D2SAdminOperationAudit{}))

	request := httptest.NewRequest(http.MethodPut, "/users/1901/identity", strings.NewReader(
		`{"id_document_type":"UNSUPPORTED","id_verification_status":"PENDING"}`,
	))
	request.RemoteAddr = "192.0.2.10:43123"
	recorder := httptest.NewRecorder()
	d2SIdentityReviewRouter().ServeHTTP(recorder, request)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.Contains(t, recorder.Body.String(), "AUDIT_WRITE_FAILED")
}
