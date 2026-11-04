package controller

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const d2SAlipaySettingsCipherAAD = "d2s-alipay-settings-v1"

type d2SAlipayConfigRequest struct {
	UseEnvironmentConfig bool   `json:"use_environment_config"`
	ChannelEnabled       *bool  `json:"channel_enabled"`
	QRPayMode            string `json:"qr_pay_mode"`
	AppID                string `json:"app_id"`
	SellerID             string `json:"seller_id"`
	GatewayURL           string `json:"gateway_url"`
	NotifyURL            string `json:"notify_url"`
	ReturnURL            string `json:"return_url"`
	MerchantPrivateKey   string `json:"merchant_private_key"`
	AlipayPublicKey      string `json:"alipay_public_key"`
}

type d2SAlipayPCSettingsResponse struct {
	QRPayMode                    string `json:"qr_pay_mode"`
	ConfigSource                 string `json:"config_source"`
	ChannelEnabled               bool   `json:"channel_enabled"`
	AppID                        string `json:"app_id"`
	SellerID                     string `json:"seller_id"`
	GatewayURL                   string `json:"gateway_url"`
	NotifyURL                    string `json:"notify_url"`
	ReturnURL                    string `json:"return_url"`
	MerchantPrivateKeyConfigured bool   `json:"merchant_private_key_configured"`
	AlipayPublicKeyConfigured    bool   `json:"alipay_public_key_configured"`
	Ready                        bool   `json:"ready"`
	UpdatedAt                    int64  `json:"updated_at"`
}

func D2SAdminConfigureAlipayPC(c *gin.Context) {
	var request d2SAlipayConfigRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		respondD2SAdminAuditedFailure(c, "UPDATE_ALIPAY_CONFIG", "alipay_pc_settings", "invalid_request", http.StatusBadRequest, "INVALID_ALIPAY_CONFIG")
		return
	}
	settings, err := model.GetD2SAlipayPCSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "SETTINGS_UNAVAILABLE"})
		return
	}
	updated := *settings
	fields := make([]string, 0, 10)
	if request.UseEnvironmentConfig {
		updated.ConfigManaged = false
		updated.ChannelEnabled = false
		updated.AppID, updated.SellerID, updated.GatewayURL = "", "", ""
		updated.NotifyURL, updated.ReturnURL = "", ""
		updated.PrivateKeyCiphertext, updated.PublicKeyCiphertext = "", ""
		fields = append(fields, "configuration_source")
	} else {
		if !updated.ConfigManaged {
			updated.AppID = strings.TrimSpace(os.Getenv("D2S_ALIPAY_APP_ID"))
			updated.SellerID = strings.TrimSpace(os.Getenv("D2S_ALIPAY_SELLER_ID"))
			updated.GatewayURL = strings.TrimSpace(os.Getenv("D2S_ALIPAY_GATEWAY_URL"))
			updated.NotifyURL = strings.TrimSpace(os.Getenv("D2S_ALIPAY_NOTIFY_URL"))
			updated.ReturnURL = strings.TrimSpace(os.Getenv("D2S_ALIPAY_RETURN_URL"))
			if updated.GatewayURL == "" {
				updated.GatewayURL = alipayPCGatewayURL
			}
			if raw := os.Getenv("D2S_ALIPAY_PRIVATE_KEY_PEM"); raw != "" {
				updated.PrivateKeyCiphertext, err = encryptD2SAlipaySetting(raw)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "SECRET_ENCRYPTION_UNAVAILABLE"})
					return
				}
			}
			if raw := os.Getenv("D2S_ALIPAY_PUBLIC_KEY_PEM"); raw != "" {
				updated.PublicKeyCiphertext, err = encryptD2SAlipaySetting(raw)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "SECRET_ENCRYPTION_UNAVAILABLE"})
					return
				}
			}
		}
		if request.QRPayMode != "" {
			request.QRPayMode = strings.TrimSpace(request.QRPayMode)
			if !validAlipayQRPayMode(request.QRPayMode) {
				respondD2SAdminAuditedFailure(c, "UPDATE_ALIPAY_CONFIG", "alipay_pc_settings", "invalid_qr_pay_mode", http.StatusBadRequest, "INVALID_QR_PAY_MODE")
				return
			}
			updated.QRPayMode = request.QRPayMode
			fields = append(fields, "qr_pay_mode")
		}
		if request.ChannelEnabled != nil {
			updated.ChannelEnabled = *request.ChannelEnabled
			fields = append(fields, "channel_enabled")
		}
		for _, field := range []struct {
			name, value string
			dest        *string
		}{
			{"app_id", request.AppID, &updated.AppID},
			{"seller_id", request.SellerID, &updated.SellerID},
			{"gateway_url", request.GatewayURL, &updated.GatewayURL},
			{"notify_url", request.NotifyURL, &updated.NotifyURL},
			{"return_url", request.ReturnURL, &updated.ReturnURL},
		} {
			value := strings.TrimSpace(field.value)
			if value == "" {
				continue
			}
			if len(value) > 512 || strings.ContainsAny(value, "\r\n\x00") {
				respondD2SAdminAuditedFailure(c, "UPDATE_ALIPAY_CONFIG", "alipay_pc_settings", "invalid_"+field.name, http.StatusBadRequest, "INVALID_ALIPAY_CONFIG")
				return
			}
			*field.dest = value
			fields = append(fields, field.name)
		}
		if request.MerchantPrivateKey != "" {
			if len(request.MerchantPrivateKey) > 16384 {
				respondD2SAdminAuditedFailure(c, "UPDATE_ALIPAY_CONFIG", "alipay_pc_settings", "merchant_private_key_too_large", http.StatusBadRequest, "INVALID_ALIPAY_PRIVATE_KEY")
				return
			}
			if _, err := parseAlipayPrivateKey(request.MerchantPrivateKey); err != nil {
				respondD2SAdminAuditedFailure(c, "UPDATE_ALIPAY_CONFIG", "alipay_pc_settings", "invalid_merchant_private_key", http.StatusBadRequest, "INVALID_ALIPAY_PRIVATE_KEY")
				return
			}
			updated.PrivateKeyCiphertext, err = encryptD2SAlipaySetting(request.MerchantPrivateKey)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "SECRET_ENCRYPTION_UNAVAILABLE"})
				return
			}
			fields = append(fields, "merchant_private_key")
		}
		if request.AlipayPublicKey != "" {
			if len(request.AlipayPublicKey) > 16384 {
				respondD2SAdminAuditedFailure(c, "UPDATE_ALIPAY_CONFIG", "alipay_pc_settings", "alipay_public_key_too_large", http.StatusBadRequest, "INVALID_ALIPAY_PUBLIC_KEY")
				return
			}
			if _, err := parseAlipayPublicKey(request.AlipayPublicKey); err != nil {
				respondD2SAdminAuditedFailure(c, "UPDATE_ALIPAY_CONFIG", "alipay_pc_settings", "invalid_alipay_public_key", http.StatusBadRequest, "INVALID_ALIPAY_PUBLIC_KEY")
				return
			}
			updated.PublicKeyCiphertext, err = encryptD2SAlipaySetting(request.AlipayPublicKey)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "SECRET_ENCRYPTION_UNAVAILABLE"})
				return
			}
			fields = append(fields, "alipay_public_key")
		}
		if updated.GatewayURL != "" && !validAlipayGatewayURL(updated.GatewayURL) {
			respondD2SAdminAuditedFailure(c, "UPDATE_ALIPAY_CONFIG", "alipay_pc_settings", "invalid_gateway_url", http.StatusBadRequest, "INVALID_ALIPAY_GATEWAY_URL")
			return
		}
		if updated.NotifyURL != "" && !validAlipayHTTPSURL(updated.NotifyURL) {
			respondD2SAdminAuditedFailure(c, "UPDATE_ALIPAY_CONFIG", "alipay_pc_settings", "invalid_notify_url", http.StatusBadRequest, "INVALID_ALIPAY_NOTIFY_URL")
			return
		}
		if updated.ReturnURL != "" && !validAlipayHTTPSURL(updated.ReturnURL) {
			respondD2SAdminAuditedFailure(c, "UPDATE_ALIPAY_CONFIG", "alipay_pc_settings", "invalid_return_url", http.StatusBadRequest, "INVALID_ALIPAY_RETURN_URL")
			return
		}
		updated.ConfigManaged = true
		fields = append(fields, "configuration_source")
	}
	if len(fields) == 0 {
		respondD2SAdminAuditedFailure(c, "UPDATE_ALIPAY_CONFIG", "alipay_pc_settings", "no_fields_changed", http.StatusBadRequest, "NO_ALIPAY_CONFIG_CHANGES")
		return
	}
	details := map[string]any{
		"fields_changed": fields,
		"config_source":  map[bool]string{true: "admin", false: "environment"}[updated.ConfigManaged],
	}
	if request.ChannelEnabled != nil {
		details["channel_enabled"] = updated.ChannelEnabled
	}
	if err := writeD2SAdminAudit(c, "UPDATE_ALIPAY_CONFIG", "alipay_pc_settings", "REQUESTED", details); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
		return
	}
	updated.UpdatedAt = time.Now().UTC().Unix()
	if err := model.SaveD2SAlipayPCSettings(&updated); err != nil {
		_ = writeD2SAdminAudit(c, "UPDATE_ALIPAY_CONFIG_RESULT", "alipay_pc_settings", "FAILURE", details)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "SETTINGS_UPDATE_FAILED"})
		return
	}
	if err := writeD2SAdminAudit(c, "UPDATE_ALIPAY_CONFIG_RESULT", "alipay_pc_settings", "SUCCESS", details); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
		return
	}
	response, err := getD2SAlipayPCSettingsResponse()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "SETTINGS_UNAVAILABLE"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": response})
}

func getD2SAlipayPCSettingsResponse() (*d2SAlipayPCSettingsResponse, error) {
	settings, err := model.GetD2SAlipayPCSettings()
	if err != nil {
		return nil, err
	}
	response := &d2SAlipayPCSettingsResponse{QRPayMode: settings.QRPayMode, UpdatedAt: settings.UpdatedAt}
	if response.QRPayMode == "" {
		response.QRPayMode = "4"
	}
	var privateKey, publicKey string
	if settings.ConfigManaged {
		response.ConfigSource = "admin"
		response.ChannelEnabled = settings.ChannelEnabled
		response.AppID, response.SellerID = settings.AppID, settings.SellerID
		response.GatewayURL, response.NotifyURL = settings.GatewayURL, settings.NotifyURL
		response.ReturnURL = settings.ReturnURL
		privateKey, err = decryptD2SAlipaySetting(settings.PrivateKeyCiphertext)
		if err != nil {
			return nil, err
		}
		publicKey, err = decryptD2SAlipaySetting(settings.PublicKeyCiphertext)
		if err != nil {
			return nil, err
		}
	} else {
		response.ConfigSource = "environment"
		response.ChannelEnabled = strings.TrimSpace(os.Getenv("D2S_ALIPAY_PC_ENABLED")) == "true"
		response.AppID, response.SellerID = strings.TrimSpace(os.Getenv("D2S_ALIPAY_APP_ID")), strings.TrimSpace(os.Getenv("D2S_ALIPAY_SELLER_ID"))
		response.GatewayURL = strings.TrimSpace(os.Getenv("D2S_ALIPAY_GATEWAY_URL"))
		if response.GatewayURL == "" {
			response.GatewayURL = alipayPCGatewayURL
		}
		response.NotifyURL, response.ReturnURL = strings.TrimSpace(os.Getenv("D2S_ALIPAY_NOTIFY_URL")), strings.TrimSpace(os.Getenv("D2S_ALIPAY_RETURN_URL"))
		privateKey, publicKey = os.Getenv("D2S_ALIPAY_PRIVATE_KEY_PEM"), os.Getenv("D2S_ALIPAY_PUBLIC_KEY_PEM")
	}
	response.MerchantPrivateKeyConfigured = strings.TrimSpace(privateKey) != ""
	response.AlipayPublicKeyConfigured = strings.TrimSpace(publicKey) != ""
	if _, err := getAlipayPCConfig(); err == nil {
		response.Ready = true
	}
	return response, nil
}

func encryptD2SAlipaySetting(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	secret := strings.TrimSpace(common.CryptoSecret)
	if secret == "" {
		return "", errors.New("stable application encryption secret is unavailable")
	}
	key := sha256.Sum256([]byte("d2s-alipay-settings-v1:" + secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := aead.Seal(nil, nonce, []byte(plaintext), []byte(d2SAlipaySettingsCipherAAD))
	return "v1:" + base64.RawURLEncoding.EncodeToString(append(nonce, sealed...)), nil
}

func decryptD2SAlipaySetting(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	if !strings.HasPrefix(ciphertext, "v1:") {
		return "", errors.New("Alipay setting ciphertext version is invalid")
	}
	secret := strings.TrimSpace(common.CryptoSecret)
	if secret == "" {
		return "", errors.New("stable application encryption secret is unavailable")
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(ciphertext, "v1:"))
	if err != nil {
		return "", errors.New("Alipay setting ciphertext is invalid")
	}
	key := sha256.Sum256([]byte("d2s-alipay-settings-v1:" + secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(payload) < aead.NonceSize() {
		return "", errors.New("Alipay setting ciphertext is truncated")
	}
	nonce, sealed := payload[:aead.NonceSize()], payload[aead.NonceSize():]
	plaintext, err := aead.Open(nil, nonce, sealed, []byte(d2SAlipaySettingsCipherAAD))
	if err != nil {
		return "", errors.New("Alipay setting could not be decrypted")
	}
	return string(plaintext), nil
}
