package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		&model.D2SRegistrationRecord{}, &model.D2SAdminBatchPreview{},
		&model.D2SUserSecurityControl{}, &model.D2SAdminOperationAudit{},
	)
	now := time.Now().UTC().Unix()
	previewID := createD2SAdminBatchPreviewForTest(t, db, now)
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
		&model.D2SRegistrationRecord{}, &model.D2SAdminBatchPreview{},
		&model.D2SUserSecurityControl{}, &model.D2SAdminOperationAudit{},
	)
	now := time.Now().UTC().Unix()
	previewID := createD2SAdminBatchPreviewForTest(t, db, now)
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
		&model.D2SRegistrationRecord{}, &model.D2SAdminBatchPreview{}, &model.D2SAdminOperationAudit{},
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
