package controller

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupD2SAdminExportTestDB(t *testing.T, name string, models ...any) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
	})
	models = append(models, &model.D2SAdminExportApproval{})
	require.NoError(t, db.AutoMigrate(models...))
	return db
}

func newD2SAdminExportTestContext(method, target string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(method, target, nil)
	context.Set("d2s_admin_id", 42)
	context.Set("d2s_admin_email", "admin@example.com")
	return context, recorder
}

func createAndApproveD2SExport(t *testing.T, exportType, exportTarget string) string {
	t.Helper()
	parsed, err := url.Parse(exportTarget)
	require.NoError(t, err)
	query := parsed.Query()
	query.Set("export_type", exportType)
	requestContext, requestRecorder := newD2SAdminExportTestContext(http.MethodPost, "/api/v1/admin/audit/export-approvals?"+query.Encode())
	requestContext.Request = httptest.NewRequest(http.MethodPost, requestContext.Request.URL.String(), strings.NewReader(`{"reason":"Investigate documented compliance request R-123"}`))
	requestContext.Request.Header.Set("Content-Type", "application/json")
	D2SAdminRequestExportApproval(requestContext)
	require.Equal(t, http.StatusCreated, requestRecorder.Code)
	var response struct {
		Data model.D2SAdminExportApproval `json:"data"`
	}
	require.NoError(t, json.Unmarshal(requestRecorder.Body.Bytes(), &response))
	require.NotEmpty(t, response.Data.ID)

	reviewContext, reviewRecorder := newD2SAdminExportTestContext(http.MethodPut, "/api/v1/admin/audit/export-approvals/"+response.Data.ID)
	reviewContext.Params = gin.Params{{Key: "id", Value: response.Data.ID}}
	reviewContext.Set("d2s_admin_id", 43)
	reviewContext.Set("d2s_admin_email", "security@example.com")
	reviewContext.Request = httptest.NewRequest(http.MethodPut, reviewContext.Request.URL.String(), strings.NewReader(`{"decision":"APPROVED","reason":"Approved under request R-123"}`))
	reviewContext.Request.Header.Set("Content-Type", "application/json")
	D2SAdminReviewExportApproval(reviewContext)
	require.Equal(t, http.StatusOK, reviewRecorder.Code)
	return response.Data.ID
}

func addD2SExportApprovalID(t *testing.T, target, approvalID string) string {
	t.Helper()
	parsed, err := url.Parse(target)
	require.NoError(t, err)
	query := parsed.Query()
	query.Set("approval_id", approvalID)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func TestD2SAdminRegistrationExportAuditsCompletion(t *testing.T) {
	db := setupD2SAdminExportTestDB(t, "d2s_admin_registration_export_test",
		&model.D2SRegistrationRecord{}, &model.D2SAdminOperationAudit{},
	)
	require.NoError(t, db.Create(&model.D2SRegistrationRecord{
		ID: "registration-export-1", UserID: 7, Email: "user@example.com",
		RegisterIP: "203.0.113.5", RegisterTime: 1_800_000_000,
		RegisterUserAgent: "test-agent", EmailVerified: true, EmailVerifiedAt: 1_800_000_010,
	}).Error)

	target := "/api/v1/admin/audit/registrations/export"
	approvalID := createAndApproveD2SExport(t, model.D2SExportApprovalTypeRegistrations, target)
	context, recorder := newD2SAdminExportTestContext(http.MethodGet, addD2SExportApprovalID(t, target, approvalID))
	D2SAdminExportRegistrationRecords(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Header().Get("Content-Type"), "text/csv")
	assert.Contains(t, recorder.Body.String(), "user_id,email,register_ip")
	assert.Contains(t, recorder.Body.String(), "user@example.com")

	var audits []model.D2SAdminOperationAudit
	require.NoError(t, db.Where("operation_type = ?", "EXPORT_REGISTRATION").Order("rowid ASC").Find(&audits).Error)
	require.Len(t, audits, 2)
	assert.Equal(t, []string{"STARTED", "SUCCESS"}, []string{audits[0].Result, audits[1].Result})
	assert.Equal(t, audits[0].TargetResource, audits[1].TargetResource)
}

func TestD2SAdminExportRequiresAuditBeforeStreaming(t *testing.T) {
	db := setupD2SAdminExportTestDB(t, "d2s_admin_export_audit_required_test", &model.D2SRegistrationRecord{})
	require.NoError(t, db.Create(&model.D2SRegistrationRecord{
		ID: "registration-private-1", UserID: 8, Email: "private@example.com",
		RegisterIP: "203.0.113.6", RegisterTime: 1_800_000_000,
	}).Error)

	context, recorder := newD2SAdminExportTestContext(http.MethodGet, "/api/v1/admin/audit/registrations/export")
	D2SAdminExportRegistrationRecords(context)

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.NotContains(t, recorder.Header().Get("Content-Type"), "text/csv")
	assert.NotContains(t, recorder.Body.String(), "private@example.com")
	assert.Contains(t, recorder.Body.String(), "AUDIT_WRITE_FAILED")
}

func TestD2SAdminExportUserActionLogsIncludesAllFieldsAndAudits(t *testing.T) {
	db := setupD2SAdminExportTestDB(t, "d2s_admin_user_action_export_test",
		&model.D2SUserActionLog{}, &model.D2SAdminOperationAudit{},
	)
	userID := 7
	sourcePort := 54321
	destinationIP := "198.51.100.8"
	destinationPort := 443
	deviceFingerprint := strings.Repeat("b", 64)
	require.NoError(t, db.Create(&model.D2SUserActionLog{
		ID: "log-export-match", UserID: &userID, Email: "=formula@example.com",
		ActionType: model.D2SActionLogin, EventTime: 1_800_000_000,
		SourceIP: "203.0.113.9", SourcePort: &sourcePort,
		DestinationIP: &destinationIP, DestinationPort: &destinationPort,
		UserAgent: "=SUM(1,1)", DeviceFingerprint: &deviceFingerprint,
		RequestPath: "/api/v1/auth/login", ResultStatus: model.D2SActionResultSuccess,
		RecordHash: strings.Repeat("a", 64),
	}).Error)
	otherUserID := 8
	require.NoError(t, db.Create(&model.D2SUserActionLog{
		ID: "log-export-excluded", UserID: &otherUserID, Email: "other@example.com",
		ActionType: model.D2SActionLogin, EventTime: 1_800_000_000,
		SourceIP: "203.0.113.10", ResultStatus: model.D2SActionResultFailure,
		RecordHash: strings.Repeat("c", 64),
	}).Error)

	target := "/api/v1/admin/audit/user-events/export.csv?start_time=1799999999&end_time=1800000001&user_id=7&email=%3Dformula%40example.com&source_ip=203.0.113.9&action_type=LOGIN"
	approvalID := createAndApproveD2SExport(t, model.D2SExportApprovalTypeUserActionLogs, target)
	context, recorder := newD2SAdminExportTestContext(http.MethodGet, addD2SExportApprovalID(t, target, approvalID))
	D2SAdminExportUserActionLogs(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Header().Get("Content-Type"), "text/csv")
	assert.Contains(t, recorder.Header().Get("Content-Disposition"), "access_log_")
	csvRows, err := csv.NewReader(strings.NewReader(recorder.Body.String())).ReadAll()
	require.NoError(t, err)
	require.Len(t, csvRows, 2)
	assert.Equal(t, []string{
		"id", "user_id", "email", "action_type", "event_time", "source_ip",
		"source_port", "destination_ip", "destination_port", "user_agent",
		"device_fingerprint", "request_path", "result_status", "record_hash",
	}, csvRows[0])
	assert.Equal(t, []string{
		"log-export-match", "7", "'=formula@example.com", "LOGIN",
		time.Unix(1_800_000_000, 0).UTC().Format(time.RFC3339),
		"203.0.113.9", "54321", "198.51.100.8", "443", "'=SUM(1,1)",
		strings.Repeat("b", 64), "/api/v1/auth/login", "SUCCESS", strings.Repeat("a", 64),
	}, csvRows[1])

	var audits []model.D2SAdminOperationAudit
	require.NoError(t, db.Where("operation_type = ?", "EXPORT_LOG").Order("rowid ASC").Find(&audits).Error)
	require.Len(t, audits, 2)
	assert.Equal(t, []string{"STARTED", "SUCCESS"}, []string{audits[0].Result, audits[1].Result})
	assert.Equal(t, "admin@example.com", audits[0].OperatorEmail)
	assert.True(t, strings.HasPrefix(audits[0].TargetResource, "access_log_"))
	assert.Contains(t, audits[0].DetailsJSON, "\"record_count\":1")
}

func TestD2SAdminExportUserActionLogsPaginatesAllRows(t *testing.T) {
	db := setupD2SAdminExportTestDB(t, "d2s_admin_user_action_export_pagination_test",
		&model.D2SUserActionLog{}, &model.D2SAdminOperationAudit{},
	)
	entries := make([]model.D2SUserActionLog, 501)
	for index := range entries {
		userID := index + 1
		entries[index] = model.D2SUserActionLog{
			ID: fmt.Sprintf("paged-log-%03d", index), UserID: &userID,
			Email:      fmt.Sprintf("user-%03d@example.com", index),
			ActionType: model.D2SActionLogin, EventTime: int64(1_800_000_000 + index),
			SourceIP: "203.0.113.20", ResultStatus: model.D2SActionResultSuccess,
			RecordHash: strings.Repeat("e", 64),
		}
	}
	require.NoError(t, db.CreateInBatches(&entries, 100).Error)

	target := "/api/v1/admin/audit/user-events/export.csv"
	approvalID := createAndApproveD2SExport(t, model.D2SExportApprovalTypeUserActionLogs, target)
	context, recorder := newD2SAdminExportTestContext(http.MethodGet, addD2SExportApprovalID(t, target, approvalID))
	D2SAdminExportUserActionLogs(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	csvRows, err := csv.NewReader(strings.NewReader(recorder.Body.String())).ReadAll()
	require.NoError(t, err)
	require.Len(t, csvRows, 502)

	ids := make(map[string]struct{}, 501)
	for _, row := range csvRows[1:] {
		ids[row[0]] = struct{}{}
	}
	require.Len(t, ids, 501)
	assert.Contains(t, ids, "paged-log-000")
	assert.Contains(t, ids, "paged-log-500")

	var audits []model.D2SAdminOperationAudit
	require.NoError(t, db.Where("operation_type = ?", "EXPORT_LOG").Order("rowid ASC").Find(&audits).Error)
	require.Len(t, audits, 2)
	assert.Contains(t, audits[0].DetailsJSON, "\"record_count\":501")
	assert.Equal(t, "SUCCESS", audits[1].Result)
}

func TestD2SAdminUserActionExportRequiresAuditBeforeStreaming(t *testing.T) {
	db := setupD2SAdminExportTestDB(t, "d2s_admin_user_action_export_audit_required_test",
		&model.D2SUserActionLog{}, &model.D2SAdminExportApproval{},
	)
	userID := 17
	require.NoError(t, db.Create(&model.D2SUserActionLog{
		ID: "private-user-action-export", UserID: &userID, Email: "private-action@example.com",
		ActionType: model.D2SActionLogin, EventTime: 1_800_000_000,
		SourceIP: "203.0.113.17", ResultStatus: model.D2SActionResultSuccess,
		RecordHash: strings.Repeat("d", 64),
	}).Error)

	filterSnapshot, filterHash, err := d2sExportApprovalFingerprint(model.D2SExportApprovalTypeUserActionLogs, map[string]any{
		"start_time": int64(0), "end_time": int64(0), "user_id": (*int)(nil),
		"email": "", "source_ip": "", "action_type": "",
	})
	require.NoError(t, err)
	approval := model.D2SAdminExportApproval{
		ID: "approval-audit-failure", ExportType: model.D2SExportApprovalTypeUserActionLogs,
		FilterHash: filterHash, FilterSnapshot: filterSnapshot,
		RequestReason: "Required audit failure fixture", RequestedByID: 42, RequestedByEmail: "admin@example.com",
		RequestedAt: time.Now().UTC().Unix(), ExpiresAt: time.Now().UTC().Unix() + 1800, Status: model.D2SExportApprovalApproved,
	}
	require.NoError(t, db.Create(&approval).Error)
	context, recorder := newD2SAdminExportTestContext(http.MethodGet, "/api/v1/admin/audit/user-events/export.csv?approval_id="+approval.ID)
	D2SAdminExportUserActionLogs(context)

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.NotContains(t, recorder.Header().Get("Content-Type"), "text/csv")
	assert.NotContains(t, recorder.Body.String(), "private-action@example.com")
	assert.Contains(t, recorder.Body.String(), "AUDIT_WRITE_FAILED")
}

func TestD2SAdminCSVStreamFailureDoesNotRecordSuccess(t *testing.T) {
	db := setupD2SAdminExportTestDB(t, "d2s_admin_csv_stream_failure_test", &model.D2SAdminOperationAudit{})
	context, _ := newD2SAdminExportTestContext(http.MethodGet, "/export.csv")
	require.NoError(t, writeD2SAdminAudit(context, "EXPORT_REGISTRATION", "export.csv", "STARTED", map[string]any{"record_count": 1}))

	writeD2SCSV(context, "EXPORT_REGISTRATION", "export.csv", []string{"email"}, func(*csv.Writer) error {
		return errors.New("simulated stream failure")
	})

	var audits []model.D2SAdminOperationAudit
	require.NoError(t, db.Where("operation_type = ?", "EXPORT_REGISTRATION").Order("rowid ASC").Find(&audits).Error)
	require.Len(t, audits, 2)
	assert.Equal(t, []string{"STARTED", "FAILURE"}, []string{audits[0].Result, audits[1].Result})
}

func TestD2SAdminReadFailuresRequireAuditableFailure(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		handler func(*gin.Context)
	}{
		{name: "user action query", path: "/audit/user-events?page=0", handler: D2SAdminUserActionLogs},
		{name: "user action export", path: "/audit/user-events/export.csv?action_type=INVALID", handler: D2SAdminExportUserActionLogs},
		{name: "registration query", path: "/audit/registrations?start_time=invalid", handler: D2SAdminRegistrationRecords},
		{name: "registration export", path: "/audit/registrations/export.csv?start_time=invalid", handler: D2SAdminExportRegistrationRecords},
		{name: "purchase query", path: "/purchases?page=0", handler: D2SAdminPurchaseRecords},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setupD2SAdminExportTestDB(t, "d2s_admin_failed_read_"+strings.ReplaceAll(test.name, " ", "_"), &model.D2SUserActionLog{})
			context, recorder := newD2SAdminExportTestContext(http.MethodGet, test.path)
			test.handler(context)

			assert.Equal(t, http.StatusInternalServerError, recorder.Code)
			assert.Contains(t, recorder.Body.String(), "AUDIT_WRITE_FAILED")
			assert.NotContains(t, recorder.Header().Get("Content-Type"), "text/csv")
		})
	}
}

func TestD2SAdminPendingExportApprovalsReturnsEmptyArray(t *testing.T) {
	setupD2SAdminExportTestDB(t, "d2s_admin_export_approval_empty_list_test", &model.D2SAdminOperationAudit{})
	context, recorder := newD2SAdminExportTestContext(http.MethodGet, "/api/v1/admin/audit/export-approvals")
	context.Set("d2s_admin_role", model.D2SAdminRoleSuperAdmin)
	D2SAdminListExportApprovals(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload struct {
		Data struct {
			Items []model.D2SAdminExportApproval `json:"items"`
			Total int64                          `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	assert.NotNil(t, payload.Data.Items)
	assert.Empty(t, payload.Data.Items)
	assert.Zero(t, payload.Data.Total)
}

func TestD2SAdminExportApprovalRejectsSelfReviewAndAuditsDenial(t *testing.T) {
	db := setupD2SAdminExportTestDB(t, "d2s_admin_export_approval_self_review_test", &model.D2SAdminOperationAudit{})
	requestContext, requestRecorder := newD2SAdminExportTestContext(http.MethodPost, "/api/v1/admin/audit/export-approvals?export_type=REGISTRATIONS")
	requestContext.Request = httptest.NewRequest(http.MethodPost, requestContext.Request.URL.String(), strings.NewReader(`{"reason":"Review registration records for case R-42"}`))
	requestContext.Request.Header.Set("Content-Type", "application/json")
	D2SAdminRequestExportApproval(requestContext)
	require.Equal(t, http.StatusCreated, requestRecorder.Code)
	var response struct {
		Data model.D2SAdminExportApproval `json:"data"`
	}
	require.NoError(t, json.Unmarshal(requestRecorder.Body.Bytes(), &response))
	require.NotEmpty(t, response.Data.ID)

	reviewContext, reviewRecorder := newD2SAdminExportTestContext(http.MethodPut, "/api/v1/admin/audit/export-approvals/"+response.Data.ID)
	reviewContext.Params = gin.Params{{Key: "id", Value: response.Data.ID}}
	reviewContext.Request = httptest.NewRequest(http.MethodPut, reviewContext.Request.URL.String(), strings.NewReader(`{"decision":"APPROVED","reason":"Self review"}`))
	reviewContext.Request.Header.Set("Content-Type", "application/json")
	D2SAdminReviewExportApproval(reviewContext)
	assert.Equal(t, http.StatusForbidden, reviewRecorder.Code)
	assert.Contains(t, reviewRecorder.Body.String(), "EXPORT_SELF_APPROVAL_FORBIDDEN")

	approval, err := model.GetD2SAdminExportApproval(response.Data.ID)
	require.NoError(t, err)
	assert.Equal(t, model.D2SExportApprovalPending, approval.Status)
	var audit model.D2SAdminOperationAudit
	require.NoError(t, db.Where("operation_type = ? AND result = ?", "REVIEW_EXPORT_APPROVAL", "FAILURE").First(&audit).Error)
	assert.Equal(t, response.Data.ID, audit.TargetResource)
}

func TestD2SAdminInvalidRegistrationQueryAppendsFailureAudit(t *testing.T) {
	db := setupD2SAdminExportTestDB(t, "d2s_admin_invalid_registration_query_audit_test", &model.D2SAdminOperationAudit{})
	context, recorder := newD2SAdminExportTestContext(http.MethodGet, "/audit/registrations?page=0")
	D2SAdminRegistrationRecords(context)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	var audit model.D2SAdminOperationAudit
	require.NoError(t, db.Where("operation_type = ?", "VIEW_REGISTRATION").First(&audit).Error)
	assert.Equal(t, "FAILURE", audit.Result)
	assert.Contains(t, audit.DetailsJSON, "invalid_page")
}

func TestD2SAdminRetentionFailureRequiresAudit(t *testing.T) {
	t.Run("audit storage unavailable", func(t *testing.T) {
		setupD2SAdminExportTestDB(t, "d2s_admin_retention_audit_unavailable_test", &model.D2SUserActionLog{})
		context, recorder := newD2SAdminExportTestContext(http.MethodPost, "/audit/retention/execute")
		context.Request = httptest.NewRequest(http.MethodPost, "/audit/retention/execute", strings.NewReader(`{"retention_days":365}`))
		D2SAdminExecuteAuditRetention(context)

		assert.Equal(t, http.StatusInternalServerError, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "AUDIT_WRITE_FAILED")
	})

	t.Run("rejected retention is recorded", func(t *testing.T) {
		db := setupD2SAdminExportTestDB(t, "d2s_admin_retention_rejection_audit_test", &model.D2SAdminOperationAudit{})
		context, recorder := newD2SAdminExportTestContext(http.MethodPost, "/audit/retention/execute")
		context.Request = httptest.NewRequest(http.MethodPost, "/audit/retention/execute", strings.NewReader(`{"retention_days":364}`))
		D2SAdminExecuteAuditRetention(context)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		var audit model.D2SAdminOperationAudit
		require.NoError(t, db.Where("operation_type = ?", "AUDIT_RETENTION_CLEANUP").First(&audit).Error)
		assert.Equal(t, "FAILURE", audit.Result)
	})
}
