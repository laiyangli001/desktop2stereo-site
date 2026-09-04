package controller

import (
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func d2sRequestID(c *gin.Context) string {
	if requestID := c.GetString(common.RequestIdKey); requestID != "" {
		return requestID
	}
	return uuid.NewString()
}

func d2sSuccess(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{
		"version": 1, "success": true, "request_id": d2sRequestID(c), "data": data,
	})
}

func d2sError(c *gin.Context, err error) {
	status, code := http.StatusInternalServerError, "internal_error"
	switch {
	case errors.Is(err, model.ErrD2SEmailVerificationRequired):
		status, code = http.StatusForbidden, "email_verification_required"
	case errors.Is(err, model.ErrD2SLicenseNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		status, code = http.StatusNotFound, "license_not_found"
	case errors.Is(err, model.ErrD2SLicenseUnavailable):
		status, code = http.StatusForbidden, "license_unavailable"
	case errors.Is(err, model.ErrD2SDeviceMismatch):
		status, code = http.StatusConflict, "device_mismatch"
	case errors.Is(err, model.ErrD2SDeviceAlreadyBound):
		status, code = http.StatusConflict, "device_already_bound"
	case errors.Is(err, model.ErrD2SLicenseAlreadyBound):
		status, code = http.StatusConflict, "license_already_bound"
	case errors.Is(err, model.ErrD2SPermanentLocked):
		status, code = http.StatusConflict, "permanent_locked"
	case errors.Is(err, model.ErrD2SRevokeCooldown):
		status, code = http.StatusTooManyRequests, "revoke_cooldown"
	case errors.Is(err, model.ErrD2SLeaseConflict):
		status, code = http.StatusConflict, "license_in_use"
	case errors.Is(err, model.ErrD2SLeaseInvalid):
		status, code = http.StatusUnauthorized, "lease_invalid"
	case errors.Is(err, model.ErrD2SDeviceCodePending):
		status, code = http.StatusAccepted, "authorization_pending"
	case errors.Is(err, model.ErrD2SDeviceCodeExpired):
		status, code = http.StatusGone, "expired_token"
	case errors.Is(err, model.ErrD2SDeviceCodeConsumed):
		status, code = http.StatusConflict, "code_already_used"
	case errors.Is(err, model.ErrD2SDeviceCodeInvalid):
		status, code = http.StatusBadRequest, "invalid_device_code"
	case errors.Is(err, service.ErrD2SSigningKeyMissing), errors.Is(err, service.ErrD2SSigningKeyInvalid):
		status, code = http.StatusServiceUnavailable, "signing_key_unavailable"
	case errors.Is(err, model.ErrD2SRegionMismatch):
		status, code = http.StatusConflict, "region_mismatch"
	case errors.Is(err, model.ErrD2SPriceNotConfigured):
		status, code = http.StatusServiceUnavailable, "price_not_configured"
	case errors.Is(err, model.ErrD2SOrderInvalid):
		status, code = http.StatusBadRequest, "order_invalid"
	case errors.Is(err, model.ErrD2SOrderNotFound):
		status, code = http.StatusNotFound, "order_not_found"
	case errors.Is(err, model.ErrD2SOrderState):
		status, code = http.StatusConflict, "order_state_invalid"
	case errors.Is(err, model.ErrD2SPaidRevokeLimit):
		status, code = http.StatusTooManyRequests, "paid_revoke_limit"
	case errors.Is(err, model.ErrD2SPaidRevokePending):
		status, code = http.StatusConflict, "paid_revoke_pending"
	case errors.Is(err, model.ErrD2SInsufficientBalance):
		status, code = http.StatusPaymentRequired, "insufficient_balance"
	case errors.Is(err, model.ErrD2SWithdrawalNotAllowed):
		status, code = http.StatusForbidden, "withdrawal_not_allowed"
	case errors.Is(err, model.ErrD2SPaymentMismatch):
		status, code = http.StatusBadRequest, "payment_mismatch"
	}
	c.JSON(status, gin.H{
		"version": 1, "success": false, "request_id": d2sRequestID(c),
		"error": gin.H{"code": code, "message": err.Error()},
	})
}

func d2sInvalidInput(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, gin.H{
		"version": 1, "success": false, "request_id": d2sRequestID(c),
		"error": gin.H{"code": "invalid_input", "message": message},
	})
}

type d2sDeviceAuthorizeRequest struct {
	DeviceHash         string `json:"device_hash"`
	FingerprintVersion int    `json:"fingerprint_version"`
	ClientName         string `json:"client_name"`
	Platform           string `json:"platform"`
}

func D2SDeviceAuthorize(c *gin.Context) {
	var request d2sDeviceAuthorizeRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		d2sInvalidInput(c, "invalid JSON body")
		return
	}
	row, secret, err := model.CreateD2SDeviceCode(request.DeviceHash, request.FingerprintVersion, request.ClientName, request.Platform, time.Now().Unix())
	if err != nil {
		d2sError(c, err)
		return
	}
	verificationURI := strings.TrimRight(os.Getenv("D2S_DEVICE_VERIFICATION_URI"), "/")
	if verificationURI == "" {
		verificationURI = strings.TrimRight(os.Getenv("FRONTEND_BASE_URL"), "/") + "/device"
	}
	d2sSuccess(c, http.StatusCreated, gin.H{
		"device_code": secret, "user_code": row.UserCode, "verification_uri": verificationURI,
		"verification_uri_complete": verificationURI + "?user_code=" + row.UserCode,
		"expires_in":                row.ExpiresAt - row.CreatedAt, "interval": 5,
	})
}

type d2sDeviceApproveRequest struct {
	UserCode string `json:"user_code"`
}

func D2SDeviceApprove(c *gin.Context) {
	var request d2sDeviceApproveRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || strings.TrimSpace(request.UserCode) == "" {
		d2sInvalidInput(c, "user_code is required")
		return
	}
	row, err := model.ApproveD2SDeviceCode(c.GetInt("id"), request.UserCode, time.Now().Unix())
	if err != nil {
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusOK, gin.H{
		"approved": true, "user_code": row.UserCode, "device_hash": row.DeviceHash,
		"client_name": row.ClientName, "platform": row.Platform,
	})
}

type d2sDeviceTokenRequest struct {
	DeviceCode string `json:"device_code"`
}

func D2SDeviceToken(c *gin.Context) {
	var request d2sDeviceTokenRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || strings.TrimSpace(request.DeviceCode) == "" {
		d2sInvalidInput(c, "device_code is required")
		return
	}
	credentials, err := service.ExchangeD2SDeviceCode(request.DeviceCode, c.ClientIP(), c.Request.UserAgent(), time.Now().Unix())
	if err != nil {
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusOK, credentials)
}

func D2SDeviceCancel(c *gin.Context) {
	var request d2sDeviceTokenRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || strings.TrimSpace(request.DeviceCode) == "" {
		d2sInvalidInput(c, "device_code is required")
		return
	}
	if err := model.CancelD2SDeviceCode(request.DeviceCode, time.Now().Unix()); err != nil {
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusOK, gin.H{"canceled": true})
}

func D2SLicenseList(c *gin.Context) {
	licenses, err := model.ListD2SLicenses(c.GetInt("id"), time.Now().Unix())
	if err != nil {
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusOK, gin.H{"licenses": licenses})
}

func D2SLicenseStatus(c *gin.Context) {
	now := time.Now().Unix()
	licenses, err := model.ListD2SLicenses(c.GetInt("id"), now)
	if err != nil {
		d2sError(c, err)
		return
	}
	available := make([]model.D2SLicense, 0, len(licenses))
	for _, license := range licenses {
		if license.Status == model.D2SLicenseStatusActive && (license.ExpiresAt == 0 || license.ExpiresAt > now) {
			available = append(available, license)
		}
	}
	d2sSuccess(c, http.StatusOK, gin.H{"valid": len(available) > 0, "server_time": now, "licenses": available})
}

type d2sLicenseDeviceRequest struct {
	LicenseID          string `json:"license_id"`
	DeviceHash         string `json:"device_hash"`
	FingerprintVersion int    `json:"fingerprint_version"`
}

func D2SLicenseActivate(c *gin.Context) {
	var request d2sLicenseDeviceRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || request.LicenseID == "" {
		d2sInvalidInput(c, "license_id, device_hash and fingerprint_version are required")
		return
	}
	license, err := model.BindD2SLicense(c.GetInt("id"), request.LicenseID, request.DeviceHash, request.FingerprintVersion, time.Now().Unix())
	if err != nil {
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusOK, gin.H{"activated": true, "license": license})
}

func D2SLicenseSwitch(c *gin.Context) { D2SLicenseActivate(c) }

type d2sChangeModeRequest struct {
	LicenseID         string `json:"license_id"`
	DeviceHash        string `json:"device_hash"`
	Mode              string `json:"mode"`
	OfflinePeriodDays int    `json:"offline_period_days"`
	Confirmation      string `json:"confirmation"`
}

func D2SLicenseChangeMode(c *gin.Context) {
	var request d2sChangeModeRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || request.LicenseID == "" || request.DeviceHash == "" {
		d2sInvalidInput(c, "license_id, device_hash and mode are required")
		return
	}
	license, err := model.ChangeD2SLicenseMode(c.GetInt("id"), request.LicenseID, request.DeviceHash, request.Mode, request.Confirmation, request.OfflinePeriodDays, time.Now().Unix())
	if err != nil {
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusOK, gin.H{"changed": true, "license": license})
}

type d2sOfflineIssueRequest struct {
	LicenseID         string `json:"license_id"`
	DeviceHash        string `json:"device_hash"`
	OfflinePeriodDays int    `json:"offline_period_days"`
}

func D2SLicenseRenew(c *gin.Context) {
	var request d2sOfflineIssueRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || request.LicenseID == "" || request.DeviceHash == "" {
		d2sInvalidInput(c, "license_id and device_hash are required")
		return
	}
	jws, claims, err := service.IssueD2SOfflineEntitlement(c.GetInt("id"), request.LicenseID, request.DeviceHash, request.OfflinePeriodDays, time.Now().Unix())
	if err != nil {
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusOK, gin.H{"entitlement": jws, "claims": claims})
}

func D2SLicenseFreeRevoke(c *gin.Context) {
	var request d2sLicenseDeviceRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || request.LicenseID == "" || request.DeviceHash == "" {
		d2sInvalidInput(c, "license_id and device_hash are required")
		return
	}
	nextAvailableAt, err := model.FreeRevokeD2SLicense(c.GetInt("id"), request.LicenseID, request.DeviceHash, time.Now().Unix())
	if err != nil {
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusOK, gin.H{"revoked": true, "next_available_at": nextAvailableAt})
}

type d2sLeaseRequest struct {
	LicenseID  string `json:"license_id"`
	DeviceHash string `json:"device_hash"`
	LeaseToken string `json:"lease_token"`
}

func D2SLicenseOnlineHeartbeat(c *gin.Context) {
	var request d2sLeaseRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || request.LicenseID == "" || request.DeviceHash == "" {
		d2sInvalidInput(c, "license_id and device_hash are required")
		return
	}
	token, expiresAt, err := model.StartOrRenewD2SOnlineLease(c.GetInt("id"), request.LicenseID, request.DeviceHash, request.LeaseToken, time.Now().Unix())
	if err != nil {
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusOK, gin.H{"lease_token": token, "expires_at": expiresAt, "heartbeat_interval": 300})
}

func D2SLicenseOnlineLogout(c *gin.Context) {
	var request d2sLeaseRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || request.LicenseID == "" || request.LeaseToken == "" {
		d2sInvalidInput(c, "license_id and lease_token are required")
		return
	}
	if err := model.ReleaseD2SOnlineLease(c.GetInt("id"), request.LicenseID, request.LeaseToken); err != nil {
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusOK, gin.H{"released": true})
}

func D2SLicensePermanentConfirm(c *gin.Context) {
	var request d2sChangeModeRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || request.LicenseID == "" || request.DeviceHash == "" {
		d2sInvalidInput(c, "license_id, device_hash and confirmation are required")
		return
	}
	license, err := model.ChangeD2SLicenseMode(c.GetInt("id"), request.LicenseID, request.DeviceHash, model.D2SLicenseModePermanent, request.Confirmation, 7, time.Now().Unix())
	if err != nil {
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusOK, gin.H{"changed": true, "license": license})
}

func D2SLicensePublicKeys(c *gin.Context) {
	keys, err := service.D2SPublicSigningKeys()
	if err != nil {
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusOK, gin.H{"keys": keys})
}

type d2sManualUnbindRequest struct {
	LicenseID string `json:"license_id"`
	Reason    string `json:"reason"`
	ProofRef  string `json:"proof_ref"`
}

func D2SManualUnbindCreate(c *gin.Context) {
	var request d2sManualUnbindRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || request.LicenseID == "" || strings.TrimSpace(request.Reason) == "" {
		d2sInvalidInput(c, "license_id and reason are required")
		return
	}
	license, err := model.GetD2SLicense(c.GetInt("id"), request.LicenseID, nil)
	if err != nil {
		d2sError(c, err)
		return
	}
	if license.Mode != model.D2SLicenseModePermanent {
		d2sError(c, model.ErrD2SLicenseUnavailable)
		return
	}
	now := time.Now().Unix()
	row := model.D2SManualUnbindRequest{
		ID: uuid.NewString(), LicenseID: license.ID, UserID: license.UserID, Reason: strings.TrimSpace(request.Reason),
		ProofRef: strings.TrimSpace(request.ProofRef), Status: "pending", CreatedAt: now, UpdatedAt: now,
	}
	if err := model.DB.Create(&row).Error; err != nil {
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusCreated, row)
}

func D2SManualUnbindList(c *gin.Context) {
	var rows []model.D2SManualUnbindRequest
	if err := model.DB.Where("user_id = ?", c.GetInt("id")).Order("created_at DESC").Find(&rows).Error; err != nil {
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusOK, gin.H{"requests": rows})
}

func D2SAdminLicenses(c *gin.Context) {
	var rows []model.D2SLicense
	query := model.DB.Order("created_at DESC").Limit(500)
	if value := strings.TrimSpace(c.Query("user_id")); value != "" {
		query = query.Where("user_id = ?", value)
	}
	if err := query.Find(&rows).Error; err != nil {
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusOK, gin.H{"licenses": rows})
}
