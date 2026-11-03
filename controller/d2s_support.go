package controller

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const d2sEnterpriseWeChatDefaultURL = "https://work.weixin.qq.com/ca/cawcde40faa1d08998"

func validD2SEnterpriseWeChatURL(value *url.URL) bool {
	return value != nil && value.Scheme == "https" &&
		strings.EqualFold(value.Hostname(), "work.weixin.qq.com") && value.Port() == "" &&
		value.User == nil && value.RawQuery == "" && value.Fragment == "" &&
		strings.HasPrefix(value.Path, "/ca/")
}

type d2sSupportSubmission struct {
	Category     string `json:"category"`
	ContactEmail string `json:"contact_email"`
	OrderID      string `json:"order_id"`
	Subject      string `json:"subject"`
	Message      string `json:"message"`
	Website      string `json:"website"`
}

func D2SSupportConfig(c *gin.Context) {
	publicURL := strings.TrimSpace(os.Getenv("D2S_ENTERPRISE_WECHAT_URL"))
	if publicURL == "" {
		publicURL = d2sEnterpriseWeChatDefaultURL
	}
	parsed, err := url.Parse(publicURL)
	if err != nil || !validD2SEnterpriseWeChatURL(parsed) {
		publicURL = ""
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"enterprise_wechat_url": publicURL}})
}

func D2SSubmitSupportTicket(c *gin.Context) {
	var request d2sSupportSubmission
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "code": "INVALID_REQUEST"})
		return
	}
	request.Category = strings.ToUpper(strings.TrimSpace(request.Category))
	request.ContactEmail = model.NormalizeEmail(request.ContactEmail)
	validCategory := false
	switch request.Category {
	case "PAYMENT", "ORDER_LICENSE", "DOWNLOAD", "SOFTWARE", "ACCOUNT", "OTHER":
		validCategory = true
	}
	if strings.TrimSpace(request.Website) != "" || !validCategory || len(request.ContactEmail) > 254 || common.Validate.Var(request.ContactEmail, "required,email") != nil || !model.ValidD2SSupportSubmissionLengths(request.Subject, request.Message) || len(request.OrderID) > 64 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "code": "SUPPORT_VALIDATION_FAILED"})
		return
	}
	userID := c.GetInt("id")
	if userID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "code": "AUTH_REQUIRED"})
		return
	}
	ticket := &model.D2SSupportTicket{
		UserID: userID, Category: request.Category, ContactEmail: request.ContactEmail, OrderID: request.OrderID,
		Subject: request.Subject, Message: request.Message, SourceIP: c.ClientIP(), UserAgent: c.Request.UserAgent(),
	}
	if err := model.CreateD2SSupportTicket(ticket); err != nil {
		if errors.Is(err, model.ErrD2SSupportOwnerUnavailable) {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "code": "ACCOUNT_NOT_ACTIVE"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "SUPPORT_SUBMISSION_FAILED"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": gin.H{"ticket_id": ticket.ID}})
}

func D2SUserSupportTickets(c *gin.Context) {
	page, pageSize := 1, 50
	var err error
	if c.Query("page") != "" {
		page, err = strconv.Atoi(c.Query("page"))
	}
	if err != nil || page < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "code": "INVALID_PAGE"})
		return
	}
	if c.Query("page_size") != "" {
		pageSize, err = strconv.Atoi(c.Query("page_size"))
	}
	if err != nil || pageSize < 1 || pageSize > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "code": "INVALID_PAGE_SIZE"})
		return
	}
	tickets, total, err := model.ListD2SUserSupportTickets(c.GetInt("id"), page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "SUPPORT_QUERY_FAILED"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": tickets, "total": total, "page": page, "page_size": pageSize}})
}

func D2SAdminSupportTickets(c *gin.Context) {
	page, pageSize := 1, 50
	var err error
	if c.Query("page") != "" {
		page, err = strconv.Atoi(c.Query("page"))
	}
	if err != nil || page < 1 {
		respondD2SAdminAuditedFailure(c, "VIEW_SUPPORT_TICKETS", "d2s_support_tickets", "invalid_page", http.StatusBadRequest, "INVALID_PAGE")
		return
	}
	if c.Query("page_size") != "" {
		pageSize, err = strconv.Atoi(c.Query("page_size"))
	}
	if err != nil || pageSize < 1 || pageSize > 100 {
		respondD2SAdminAuditedFailure(c, "VIEW_SUPPORT_TICKETS", "d2s_support_tickets", "invalid_page_size", http.StatusBadRequest, "INVALID_PAGE_SIZE")
		return
	}
	status := strings.ToUpper(strings.TrimSpace(c.Query("status")))
	if status != "" {
		switch status {
		case model.D2SSupportOpen, model.D2SSupportInProgress, model.D2SSupportResolved, model.D2SSupportClosed:
		default:
			respondD2SAdminAuditedFailure(c, "VIEW_SUPPORT_TICKETS", "d2s_support_tickets", "invalid_status", http.StatusBadRequest, "INVALID_STATUS")
			return
		}
	}
	tickets, total, err := model.ListD2SSupportTickets(status, page, pageSize)
	if err != nil {
		respondD2SAdminAuditedFailure(c, "VIEW_SUPPORT_TICKETS", "d2s_support_tickets", "query_failed", http.StatusInternalServerError, "SUPPORT_QUERY_FAILED")
		return
	}
	if err := writeD2SAdminAudit(c, "VIEW_SUPPORT_TICKETS", "d2s_support_tickets", "SUCCESS", map[string]any{"status": status, "page": page, "page_size": pageSize, "total": total}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": tickets, "total": total, "page": page, "page_size": pageSize}})
}

func D2SAdminUpdateSupportTicket(c *gin.Context) {
	var request struct {
		Status string `json:"status"`
		Reply  string `json:"admin_reply"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || !model.ValidD2SSupportReplyLength(request.Reply) {
		respondD2SAdminAuditedFailureDetails(c, "UPDATE_SUPPORT_TICKET", "", nil, http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	ticket, err := model.UpdateD2SSupportTicketWithAudit(c.Param("id"), request.Status, request.Reply, c.GetInt("d2s_admin_id"), c.GetString("d2s_admin_email"), c.ClientIP())
	if err != nil {
		if auditErr := writeD2SAdminAudit(c, "UPDATE_SUPPORT_TICKET", "", "FAILURE", nil); auditErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
			return
		}
		code, status := "SUPPORT_UPDATE_FAILED", http.StatusInternalServerError
		if err == model.ErrD2SSupportTicketNotFound {
			code, status = "SUPPORT_TICKET_NOT_FOUND", http.StatusNotFound
		}
		c.JSON(status, gin.H{"success": false, "code": code})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": ticket})
}
