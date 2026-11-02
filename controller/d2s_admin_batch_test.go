package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func createD2SAdminBatchPreviewForTest(t *testing.T, db *gorm.DB, now int64) string {
	t.Helper()
	loginBlocked := true
	selectorJSON, err := json.Marshal(d2SBatchSelector{EmailDomain: "example.com"})
	require.NoError(t, err)
	actionJSON, err := json.Marshal(d2SBatchActions{LoginBlocked: &loginBlocked, Reason: "incident"})
	require.NoError(t, err)
	userIDsJSON, err := json.Marshal([]int{1001})
	require.NoError(t, err)
	preview := model.D2SAdminBatchPreview{
		ID: "batch-preview-test", AdminID: 42,
		SelectorJSON: string(selectorJSON), ActionJSON: string(actionJSON), UserIDsJSON: string(userIDsJSON),
		MatchCount: 1, CreatedAt: now, ExpiresAt: now + 300,
	}
	require.NoError(t, db.Create(&preview).Error)
	return preview.ID
}

func createD2SAdminBatchUserForTest(t *testing.T, db *gorm.DB, userID int, email string, createdAt int64) {
	t.Helper()
	require.NoError(t, db.Create(&model.User{
		Id: userID, Username: "batch-user-" + strconv.Itoa(userID), Password: "test-password-hash",
		Email: email, AffCode: "batch-aff-" + strconv.Itoa(userID), CreatedAt: createdAt,
	}).Error)
}

func postD2SAdminBatchApply(previewID string) (*httptest.ResponseRecorder, *gin.Context) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/batch/apply",
		strings.NewReader(`{"preview_id":"`+previewID+`","confirmed":true,"confirmation_count":1}`))
	context.Request.RemoteAddr = "192.0.2.10:43123"
	context.Set("d2s_admin_id", 42)
	context.Set("d2s_admin_email", "admin@example.com")
	return recorder, context
}

func TestD2SAdminBatchApplyRejectsChangedSelection(t *testing.T) {
	db := setupD2SAdminExportTestDB(t, "d2s_admin_batch_changed_selection_test",
		&model.User{}, &model.D2SRegistrationRecord{}, &model.D2SAdminBatchPreview{},
		&model.D2SUserSecurityControl{}, &model.D2SAdminOperationAudit{},
	)
	now := time.Now().UTC().Unix()
	previewID := createD2SAdminBatchPreviewForTest(t, db, now)
	createD2SAdminBatchUserForTest(t, db, 1001, "first@example.com", now)
	createD2SAdminBatchUserForTest(t, db, 1002, "second@example.com", now+1)
	require.NoError(t, db.Create(&model.D2SRegistrationRecord{
		ID: "registration-1", UserID: 1001, Email: "first@example.com",
		RegisterIP: "203.0.113.1", RegisterTime: now,
	}).Error)
	require.NoError(t, db.Create(&model.D2SRegistrationRecord{
		ID: "registration-2", UserID: 1002, Email: "second@example.com",
		RegisterIP: "203.0.113.2", RegisterTime: now + 1,
	}).Error)

	recorder, context := postD2SAdminBatchApply(previewID)
	D2SAdminBatchEmergencyApply(context)

	require.Equal(t, http.StatusConflict, recorder.Code)
	var controls []model.D2SUserSecurityControl
	require.NoError(t, db.Find(&controls).Error)
	require.Empty(t, controls)

	var preview model.D2SAdminBatchPreview
	require.NoError(t, db.First(&preview, "id = ?", previewID).Error)
	require.Zero(t, preview.ConsumedAt)

	var audits []model.D2SAdminOperationAudit
	require.NoError(t, db.Where("operation_type = ?", "EMERGENCY_LOCK").Find(&audits).Error)
	require.Len(t, audits, 1)
	require.Equal(t, "FAILURE", audits[0].Result)
	require.Contains(t, audits[0].DetailsJSON, "selection_changed_since_preview")
	require.Contains(t, audits[0].DetailsJSON, `"preview_match_count":1`)
	require.Contains(t, audits[0].DetailsJSON, `"current_match_count":2`)
}

func TestD2SAdminBatchApplyRechecksAndAppliesUnchangedSelection(t *testing.T) {
	db := setupD2SAdminExportTestDB(t, "d2s_admin_batch_unchanged_selection_test",
		&model.User{}, &model.D2SRegistrationRecord{}, &model.D2SAdminBatchPreview{},
		&model.D2SUserSecurityControl{}, &model.D2SAdminOperationAudit{},
	)
	now := time.Now().UTC().Unix()
	previewID := createD2SAdminBatchPreviewForTest(t, db, now)
	createD2SAdminBatchUserForTest(t, db, 1001, "first@example.com", now)
	require.NoError(t, db.Create(&model.D2SRegistrationRecord{
		ID: "registration-1", UserID: 1001, Email: "first@example.com",
		RegisterIP: "203.0.113.1", RegisterTime: now,
	}).Error)

	recorder, context := postD2SAdminBatchApply(previewID)
	D2SAdminBatchEmergencyApply(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	var control model.D2SUserSecurityControl
	require.NoError(t, db.First(&control, "user_id = ?", 1001).Error)
	require.True(t, control.LoginBlocked)

	var preview model.D2SAdminBatchPreview
	require.NoError(t, db.First(&preview, "id = ?", previewID).Error)
	require.Greater(t, preview.ConsumedAt, int64(0))

	var audits []model.D2SAdminOperationAudit
	require.NoError(t, db.Where("operation_type = ?", "EMERGENCY_LOCK").Find(&audits).Error)
	require.Len(t, audits, 1)
	require.Equal(t, "SUCCESS", audits[0].Result)
}

func TestD2SAdminBatchPreviewRollsBackWhenSuccessAuditCannotBeWritten(t *testing.T) {
	db := setupD2SAdminExportTestDB(t, "d2s_admin_batch_preview_audit_failure_test",
		&model.User{}, &model.D2SRegistrationRecord{}, &model.D2SAdminBatchPreview{}, &model.D2SAdminOperationAudit{},
	)
	require.NoError(t, db.Migrator().DropTable(&model.D2SAdminOperationAudit{}))
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/batch/preview",
		strings.NewReader(`{"selector":{"email_domain":"example.com"},"actions":{"login_blocked":true}}`))
	context.Request.RemoteAddr = "192.0.2.10:43123"
	context.Set("d2s_admin_id", 42)
	context.Set("d2s_admin_email", "admin@example.com")

	D2SAdminBatchEmergencyPreview(context)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.Contains(t, recorder.Body.String(), "AUDIT_WRITE_FAILED")
	var count int64
	require.NoError(t, db.Model(&model.D2SAdminBatchPreview{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestD2SAdminBatchEmailDomainIncludesLegacyUsersWithoutRegistrationSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupD2SAdminExportTestDB(t, "d2s_admin_batch_legacy_user_test",
		&model.User{}, &model.D2SRegistrationRecord{}, &model.D2SAdminBatchPreview{},
		&model.D2SUserSecurityControl{}, &model.D2SAdminOperationAudit{},
	)
	now := time.Now().UTC().Unix()
	createD2SAdminBatchUserForTest(t, db, 1101, "legacy@legacy.example", now-86400)

	previewRecorder := httptest.NewRecorder()
	previewContext, _ := gin.CreateTestContext(previewRecorder)
	previewContext.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/batch/preview", strings.NewReader(
		`{"selector":{"email_domain":"legacy.example","register_from":`+strconv.FormatInt(now-172800, 10)+`,"register_until":`+strconv.FormatInt(now, 10)+`},"actions":{"login_blocked":true,"reason":"legacy review"}}`,
	))
	previewContext.Request.RemoteAddr = "192.0.2.10:43123"
	previewContext.Set("d2s_admin_id", 42)
	previewContext.Set("d2s_admin_email", "admin@example.com")
	D2SAdminBatchEmergencyPreview(previewContext)
	require.Equal(t, http.StatusOK, previewRecorder.Code)
	var previewResponse struct {
		Data struct {
			PreviewID  string `json:"preview_id"`
			MatchCount int64  `json:"match_count"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(previewRecorder.Body.Bytes(), &previewResponse))
	require.EqualValues(t, 1, previewResponse.Data.MatchCount)
	require.NotEmpty(t, previewResponse.Data.PreviewID)

	applyRecorder, applyContext := postD2SAdminBatchApply(previewResponse.Data.PreviewID)
	D2SAdminBatchEmergencyApply(applyContext)
	require.Equal(t, http.StatusOK, applyRecorder.Code)
	var control model.D2SUserSecurityControl
	require.NoError(t, db.First(&control, "user_id = ?", 1101).Error)
	require.True(t, control.LoginBlocked)
}

func TestD2SAdminBatchIPRangeUsesRegistrationSnapshotsAndSkipsDeletedUsers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupD2SAdminExportTestDB(t, "d2s_admin_batch_ip_range_users_test",
		&model.User{}, &model.D2SRegistrationRecord{}, &model.D2SAdminBatchPreview{},
		&model.D2SUserSecurityControl{}, &model.D2SAdminOperationAudit{},
	)
	now := time.Now().UTC().Unix()
	createD2SAdminBatchUserForTest(t, db, 1201, "active@example.com", now)
	createD2SAdminBatchUserForTest(t, db, 1202, "deleted@example.com", now)
	var deletedUser model.User
	require.NoError(t, db.First(&deletedUser, "id = ?", 1202).Error)
	require.NoError(t, db.Delete(&deletedUser).Error)
	require.NoError(t, db.Create(&model.D2SRegistrationRecord{
		ID: "active-ip-registration", UserID: 1201, Email: "active@example.com",
		RegisterIP: "203.0.113.5", RegisterTime: now,
	}).Error)
	require.NoError(t, db.Create(&model.D2SRegistrationRecord{
		ID: "deleted-ip-registration", UserID: 1202, Email: "deleted@example.com",
		RegisterIP: "203.0.113.6", RegisterTime: now,
	}).Error)

	previewRecorder := httptest.NewRecorder()
	previewContext, _ := gin.CreateTestContext(previewRecorder)
	previewContext.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/batch/preview", strings.NewReader(
		`{"selector":{"ip_cidr":"203.0.113.0/24"},"actions":{"purchase_blocked":true,"reason":"IP review"}}`,
	))
	previewContext.Request.RemoteAddr = "192.0.2.10:43123"
	previewContext.Set("d2s_admin_id", 42)
	previewContext.Set("d2s_admin_email", "admin@example.com")
	D2SAdminBatchEmergencyPreview(previewContext)
	require.Equal(t, http.StatusOK, previewRecorder.Code)
	var previewResponse struct {
		Data struct {
			PreviewID  string `json:"preview_id"`
			MatchCount int64  `json:"match_count"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(previewRecorder.Body.Bytes(), &previewResponse))
	require.EqualValues(t, 1, previewResponse.Data.MatchCount)

	applyRecorder, applyContext := postD2SAdminBatchApply(previewResponse.Data.PreviewID)
	D2SAdminBatchEmergencyApply(applyContext)
	require.Equal(t, http.StatusOK, applyRecorder.Code)
	var activeControl model.D2SUserSecurityControl
	require.NoError(t, db.First(&activeControl, "user_id = ?", 1201).Error)
	require.True(t, activeControl.PurchaseBlocked)
	var deletedControl model.D2SUserSecurityControl
	require.ErrorIs(t, db.First(&deletedControl, "user_id = ?", 1202).Error, gorm.ErrRecordNotFound)
}
