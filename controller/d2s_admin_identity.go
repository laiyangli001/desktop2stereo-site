package controller

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const d2SIdentityHashVersion = "HMAC-SHA-256-v1"

var errD2SIdentityHashKeyUnavailable = errors.New("identity document HMAC key is unavailable")

func hashD2SIdentityDocumentNumber(documentNumber string) (string, error) {
	encodedKey := strings.TrimSpace(os.Getenv("D2S_ID_DOCUMENT_HMAC_KEY"))
	key, err := hex.DecodeString(encodedKey)
	if err != nil || len(key) != 32 {
		return "", errD2SIdentityHashKeyUnavailable
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("d2s-identity-document-v1\x00" + documentNumber))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

type d2SIdentityReviewRequest struct {
	DocumentType   string `json:"id_document_type"`
	DocumentNumber string `json:"id_document_number"`
	Country        string `json:"id_document_country"`
	Status         string `json:"id_verification_status"`
}

func D2SAdminGetIdentityVerification(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("id"))
	if err != nil || userID <= 0 {
		respondD2SAdminAuditedFailure(c, "VIEW_IDENTITY", c.Param("id"), "invalid_user_id", http.StatusBadRequest, "INVALID_USER_ID")
		return
	}
	var record model.D2SIdentityVerification
	err = model.DB.Where("user_id = ?", userID).First(&record).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		respondD2SAdminAuditedFailure(c, "VIEW_IDENTITY", strconv.Itoa(userID), "query_failed", http.StatusInternalServerError, "IDENTITY_QUERY_FAILED")
		return
	}
	if err == gorm.ErrRecordNotFound {
		record = model.D2SIdentityVerification{UserID: userID, Status: "NOT_REQUIRED"}
	}
	legacyHashRejected := record.Status == "VERIFIED" && record.IDDocumentNumberHashVersion != d2SIdentityHashVersion
	if legacyHashRejected {
		detailsJSON, marshalErr := common.Marshal(map[string]any{
			"status": "PENDING", "legacy_hash_rejected": true, "legacy_hash_cleared": true,
		})
		if marshalErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_SERIALIZE_FAILED"})
			return
		}
		operationTime := time.Now().UTC().Unix()
		audit := model.D2SAdminOperationAudit{
			ID: uuid.NewString(), OperatorID: c.GetInt("d2s_admin_id"), OperatorEmail: c.GetString("d2s_admin_email"),
			OperationType: "VIEW_IDENTITY", OperationTime: operationTime, TargetResource: strconv.Itoa(userID),
			SourceIP: c.ClientIP(), Result: "SUCCESS", DetailsJSON: string(detailsJSON),
		}
		if err := model.DB.Transaction(func(tx *gorm.DB) error {
			result := tx.Model(&model.D2SIdentityVerification{}).
				Where("user_id = ? AND status = ? AND id_document_number_hash_version = ? AND id_document_number_hash = ?",
					userID, "VERIFIED", record.IDDocumentNumberHashVersion, record.IDDocumentNumberHash).
				Updates(map[string]any{
					"status": "PENDING", "id_document_number_hash": "", "verified_at": 0, "verified_by": 0,
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return gorm.ErrRecordNotFound
			}
			return tx.Create(&audit).Error
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "IDENTITY_MIGRATION_FAILED"})
			return
		}
		record.Status = "PENDING"
		record.IDDocumentNumberHash = ""
		record.VerifiedAt = 0
		record.VerifiedBy = 0
	} else if err := writeD2SAdminAudit(c, "VIEW_IDENTITY", strconv.Itoa(userID), "SUCCESS", map[string]any{
		"status": record.Status, "legacy_hash_rejected": false,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": record})
}

func D2SAdminReviewIdentityVerification(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("id"))
	if err != nil || userID <= 0 {
		respondD2SAdminAuditedFailure(c, "REVIEW_IDENTITY", c.Param("id"), "invalid_user_id", http.StatusBadRequest, "INVALID_USER_ID")
		return
	}
	if _, err := model.GetUserById(userID, false); err != nil {
		respondD2SAdminAuditedFailure(c, "REVIEW_IDENTITY", strconv.Itoa(userID), "user_not_found", http.StatusNotFound, "USER_NOT_FOUND")
		return
	}
	var request d2SIdentityReviewRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		respondD2SAdminAuditedFailure(c, "REVIEW_IDENTITY", strconv.Itoa(userID), "invalid_request", http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	request.DocumentType = strings.ToUpper(strings.TrimSpace(request.DocumentType))
	request.Country = strings.ToUpper(strings.TrimSpace(request.Country))
	request.Status = strings.ToUpper(strings.TrimSpace(request.Status))
	if request.DocumentType != "" && request.DocumentType != "PASSPORT" && request.DocumentType != "FOREIGN_PERMANENT_RESIDENT_ID" && request.DocumentType != "OTHER" {
		respondD2SAdminAuditedFailure(c, "REVIEW_IDENTITY", strconv.Itoa(userID), "invalid_document_type", http.StatusBadRequest, "INVALID_DOCUMENT_TYPE")
		return
	}
	if request.Country != "" && (len(request.Country) < 2 || len(request.Country) > 3) {
		respondD2SAdminAuditedFailure(c, "REVIEW_IDENTITY", strconv.Itoa(userID), "invalid_country_code", http.StatusBadRequest, "INVALID_COUNTRY_CODE")
		return
	}
	if request.Status != "NOT_REQUIRED" && request.Status != "PENDING" && request.Status != "VERIFIED" && request.Status != "REJECTED" {
		respondD2SAdminAuditedFailure(c, "REVIEW_IDENTITY", strconv.Itoa(userID), "invalid_verification_status", http.StatusBadRequest, "INVALID_VERIFICATION_STATUS")
		return
	}
	var record model.D2SIdentityVerification
	if err := model.DB.Where("user_id = ?", userID).First(&record).Error; err != nil && err != gorm.ErrRecordNotFound {
		respondD2SAdminAuditedFailure(c, "REVIEW_IDENTITY", strconv.Itoa(userID), "identity_read_failed", http.StatusInternalServerError, "IDENTITY_READ_FAILED")
		return
	}
	record.UserID = userID
	if request.DocumentType != "" {
		record.IDDocumentType = request.DocumentType
	}
	if request.Country != "" {
		record.IDDocumentCountry = request.Country
	}
	documentNumber := strings.ToUpper(strings.TrimSpace(request.DocumentNumber))
	if documentNumber != "" {
		hash, err := hashD2SIdentityDocumentNumber(documentNumber)
		if err != nil {
			respondD2SAdminAuditedFailure(c, "REVIEW_IDENTITY", strconv.Itoa(userID), "identity_hash_key_unavailable", http.StatusServiceUnavailable, "IDENTITY_HASH_KEY_UNAVAILABLE")
			return
		}
		record.IDDocumentNumberHash = hash
		record.IDDocumentNumberHashVersion = d2SIdentityHashVersion
	}
	if request.Status == "VERIFIED" && (record.IDDocumentType == "" || record.IDDocumentNumberHash == "" || record.IDDocumentNumberHashVersion != d2SIdentityHashVersion || record.IDDocumentCountry == "") {
		respondD2SAdminAuditedFailure(c, "REVIEW_IDENTITY", strconv.Itoa(userID), "verified_identity_fields_required", http.StatusBadRequest, "IDENTITY_FIELDS_REQUIRED")
		return
	}
	record.Status = request.Status
	record.VerifiedAt = 0
	record.VerifiedBy = 0
	operationTime := time.Now().UTC().Unix()
	if record.Status == "VERIFIED" {
		record.VerifiedAt = operationTime
		record.VerifiedBy = c.GetInt("d2s_admin_id")
	}
	auditDetails, err := common.Marshal(map[string]any{
		"document_type": record.IDDocumentType, "country": record.IDDocumentCountry, "status": record.Status,
	})
	if err != nil {
		respondD2SAdminAuditedFailure(c, "REVIEW_IDENTITY", strconv.Itoa(userID), "audit_serialize_failed", http.StatusInternalServerError, "AUDIT_SERIALIZE_FAILED")
		return
	}
	audit := model.D2SAdminOperationAudit{
		ID: uuid.NewString(), OperatorID: c.GetInt("d2s_admin_id"), OperatorEmail: c.GetString("d2s_admin_email"),
		OperationType: "REVIEW_IDENTITY", OperationTime: operationTime, TargetResource: strconv.Itoa(userID),
		SourceIP: c.ClientIP(), Result: "SUCCESS", DetailsJSON: string(auditDetails),
	}
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&record).Error; err != nil {
			return err
		}
		return tx.Create(&audit).Error
	}); err != nil {
		respondD2SAdminAuditedFailureDetails(c, "REVIEW_IDENTITY", strconv.Itoa(userID), map[string]any{
			"status": request.Status, "reason": "identity_or_audit_transaction_failed",
		}, http.StatusInternalServerError, "IDENTITY_WRITE_FAILED")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": record})
}
