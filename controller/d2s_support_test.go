package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestD2SSupportSubmissionRequiresAuthenticatedAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/v1/support/tickets", strings.NewReader(
		`{"category":"PAYMENT","contact_email":"user@example.com","subject":"Payment issue","message":"My payment succeeded but my account does not show the purchase."}`,
	))

	D2SSubmitSupportTicket(context)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	require.Contains(t, recorder.Body.String(), "AUTH_REQUIRED")
}

func TestD2SSupportSubmissionPersistsAuthenticatedOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupD2SAdminExportTestDB(t, "d2s_support_submission_test", &model.D2SSupportTicket{})
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("id", 37)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/v1/support/tickets", strings.NewReader(
		`{"user_id":999,"category":"PAYMENT","contact_email":" USER@EXAMPLE.COM ","subject":"Payment issue","message":"My payment succeeded but my account does not show the purchase."}`,
	))
	context.Request.Header.Set("User-Agent", "support-test-agent")

	D2SSubmitSupportTicket(context)

	require.Equal(t, http.StatusCreated, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			TicketID string `json:"ticket_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.NotEmpty(t, response.Data.TicketID)
	var ticket model.D2SSupportTicket
	require.NoError(t, db.First(&ticket, "id = ?", response.Data.TicketID).Error)
	require.Equal(t, 37, ticket.UserID)
	require.Equal(t, "user@example.com", ticket.ContactEmail)
	require.Equal(t, "support-test-agent", ticket.UserAgent)
	require.Equal(t, model.D2SSupportOpen, ticket.Status)
}

func TestD2SSupportConfigOnlyReturnsSafeHTTPSURL(t *testing.T) {
	t.Setenv("D2S_ENTERPRISE_WECHAT_URL", "")
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/v1/support/config", nil)
	D2SSupportConfig(context)

	var response struct {
		Success bool `json:"success"`
		Data    struct {
			URL string `json:"enterprise_wechat_url"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Equal(t, d2sEnterpriseWeChatDefaultURL, response.Data.URL)

	for _, invalidURL := range []string{
		"http://work.weixin.qq.com/ca/contact",
		"https://evil.example/ca/contact",
		"https://work.weixin.qq.com.evil.example/ca/contact",
		"https://work.weixin.qq.com/contact",
		"https://user@work.weixin.qq.com/ca/contact",
		"https://work.weixin.qq.com:8443/ca/contact",
		"https://work.weixin.qq.com/ca/contact?next=https://evil.example",
		"https://work.weixin.qq.com/ca/contact#fragment",
	} {
		t.Setenv("D2S_ENTERPRISE_WECHAT_URL", invalidURL)
		recorder = httptest.NewRecorder()
		context, _ = gin.CreateTestContext(recorder)
		context.Request = httptest.NewRequest(http.MethodGet, "/api/v1/support/config", nil)
		D2SSupportConfig(context)
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
		require.Empty(t, response.Data.URL, invalidURL)
	}
}

func TestD2SAdminSupportTicketQueryFailureReturnsServerErrorAndAudits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupD2SAdminExportTestDB(t, "d2s_admin_support_query_failure_test", &model.D2SAdminOperationAudit{})
	context, recorder := newD2SAdminExportTestContext(http.MethodGet, "/api/v1/admin/support/tickets")

	D2SAdminSupportTickets(context)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.Contains(t, recorder.Body.String(), "SUPPORT_QUERY_FAILED")
	var audit model.D2SAdminOperationAudit
	require.NoError(t, db.First(&audit).Error)
	require.Equal(t, "VIEW_SUPPORT_TICKETS", audit.OperationType)
	require.Equal(t, "FAILURE", audit.Result)
}
