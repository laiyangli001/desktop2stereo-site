package service

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrD2SSigningKeyMissing  = errors.New("Desktop2Stereo signing key is not configured")
	ErrD2SSigningKeyInvalid  = errors.New("Desktop2Stereo signing key is invalid")
	ErrD2SSigningKeyCurrent  = errors.New("the active Desktop2Stereo signing key cannot be retired")
	ErrD2SSigningKeyNotFound = errors.New("Desktop2Stereo signing key was not found")
)

type D2SOfflineClaims struct {
	Version           int      `json:"version"`
	KeyID             string   `json:"key_id"`
	EntitlementID     string   `json:"entitlement_id"`
	LicenseID         string   `json:"license_id"`
	Product           string   `json:"product"`
	DeviceHash        string   `json:"device_hash"`
	Mode              string   `json:"mode"`
	Features          []string `json:"features"`
	IssuedAt          int64    `json:"issued_at"`
	NotBefore         int64    `json:"not_before"`
	ExpiresAt         int64    `json:"expires_at"`
	Trial             bool     `json:"trial"`
	OfflinePeriodDays int      `json:"offline_period_days"`
}

type D2SPublicJWK struct {
	KTY string `json:"kty"`
	CRV string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	Kid string `json:"kid"`
}

func d2sConfiguredKeyID() string {
	keyID := strings.TrimSpace(os.Getenv("D2S_LICENSE_KEY_ID"))
	if keyID == "" {
		return "d2s-es256-v1"
	}
	return keyID
}

// IsD2SCurrentSigningKey reports whether a persisted key is configured for new signatures.
func IsD2SCurrentSigningKey(keyID string) bool {
	return strings.TrimSpace(keyID) == d2sConfiguredKeyID()
}

func d2sSigningKey() (*ecdsa.PrivateKey, string, error) {
	keyID := d2sConfiguredKeyID()
	raw := strings.TrimSpace(os.Getenv("D2S_LICENSE_PRIVATE_KEY_PEM"))
	if encoded := strings.TrimSpace(os.Getenv("D2S_LICENSE_PRIVATE_KEY_B64")); raw == "" && encoded != "" {
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, "", fmt.Errorf("%w: base64 decode failed", ErrD2SSigningKeyInvalid)
		}
		raw = string(decoded)
	}
	if raw == "" {
		return nil, "", ErrD2SSigningKeyMissing
	}
	raw = strings.ReplaceAll(raw, `\n`, "\n")
	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		return nil, "", ErrD2SSigningKeyInvalid
	}
	var key *ecdsa.PrivateKey
	if parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		key, _ = parsed.(*ecdsa.PrivateKey)
	}
	if key == nil {
		parsed, err := x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			return nil, "", ErrD2SSigningKeyInvalid
		}
		key = parsed
	}
	if key.Curve != elliptic.P256() {
		return nil, "", fmt.Errorf("%w: expected P-256", ErrD2SSigningKeyInvalid)
	}
	return key, keyID, nil
}

func d2sPublicJWK(key *ecdsa.PrivateKey, keyID string) D2SPublicJWK {
	encode := func(value *big.Int) string {
		bytes := value.FillBytes(make([]byte, 32))
		return base64.RawURLEncoding.EncodeToString(bytes)
	}
	return D2SPublicJWK{
		KTY: "EC", CRV: "P-256", X: encode(key.PublicKey.X), Y: encode(key.PublicKey.Y),
		Use: "sig", Alg: "ES256", Kid: keyID,
	}
}

func signD2SClaims(key *ecdsa.PrivateKey, keyID string, claims D2SOfflineClaims) (string, error) {
	header, err := common.Marshal(map[string]string{"alg": "ES256", "kid": keyID, "typ": "JWT"})
	if err != nil {
		return "", err
	}
	payload, err := common.Marshal(claims)
	if err != nil {
		return "", err
	}
	encodedHeader := base64.RawURLEncoding.EncodeToString(header)
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	signingInput := encodedHeader + "." + encodedPayload
	digest := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		return "", err
	}
	signature := append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...)
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func D2SPublicSigningKeys() ([]D2SPublicJWK, error) {
	key, keyID, err := d2sSigningKey()
	if err != nil {
		return nil, err
	}
	current := d2sPublicJWK(key, keyID)
	currentJSON, err := common.Marshal(current)
	if err != nil {
		return nil, err
	}
	if err := model.DB.Where("key_id = ?", keyID).Assign(model.D2SSigningKey{
		KeyID: keyID, Algorithm: "ES256", PublicJWK: string(currentJSON), Status: "active",
	}).FirstOrCreate(&model.D2SSigningKey{KeyID: keyID, CreatedAt: time.Now().Unix()}).Error; err != nil {
		return nil, err
	}

	var stored []model.D2SSigningKey
	if err := model.DB.Where("status <> ?", "retired").Order("created_at DESC").Find(&stored).Error; err != nil {
		return nil, err
	}
	keys := make([]D2SPublicJWK, 0, len(stored))
	seen := make(map[string]struct{}, len(stored))
	for _, row := range stored {
		if row.Algorithm != "ES256" || strings.TrimSpace(row.PublicJWK) == "" {
			return nil, fmt.Errorf("%w: invalid public key metadata", ErrD2SSigningKeyInvalid)
		}
		var public D2SPublicJWK
		if err := common.Unmarshal([]byte(row.PublicJWK), &public); err != nil || public.Kid == "" || public.Alg != "ES256" {
			return nil, fmt.Errorf("%w: invalid public key metadata", ErrD2SSigningKeyInvalid)
		}
		if _, ok := seen[public.Kid]; ok {
			continue
		}
		seen[public.Kid] = struct{}{}
		keys = append(keys, public)
	}
	if len(keys) == 0 {
		return []D2SPublicJWK{current}, nil
	}
	return keys, nil
}

func RetireD2SSigningKey(keyID string, now int64) error {
	keyID = strings.TrimSpace(keyID)
	if keyID == "" {
		return model.ErrD2SOrderInvalid
	}
	if keyID == d2sConfiguredKeyID() {
		return ErrD2SSigningKeyCurrent
	}
	if now <= 0 {
		now = time.Now().Unix()
	}
	result := model.DB.Model(&model.D2SSigningKey{}).
		Where("key_id = ? AND status <> ?", keyID, "retired").
		Updates(map[string]any{"status": "retired", "retired_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrD2SSigningKeyNotFound
	}
	return nil
}

func IssueD2SOfflineEntitlement(userID int, licenseID, deviceHash string, requestedDays int, now int64) (string, *D2SOfflineClaims, error) {
	if now <= 0 {
		now = time.Now().Unix()
	}
	key, keyID, err := d2sSigningKey()
	if err != nil {
		return "", nil, err
	}
	jwkJSON, err := common.Marshal(d2sPublicJWK(key, keyID))
	if err != nil {
		return "", nil, err
	}
	var jws string
	var claims *D2SOfflineClaims
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		var license model.D2SLicense
		if err := model.LockForUpdate(tx).Where("id = ? AND user_id = ?", licenseID, userID).First(&license).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return model.ErrD2SLicenseNotFound
			}
			return err
		}
		if license.Status != model.D2SLicenseStatusActive || license.DeviceHash != strings.ToLower(strings.TrimSpace(deviceHash)) || (license.ExpiresAt > 0 && license.ExpiresAt <= now) {
			return model.ErrD2SLicenseUnavailable
		}
		if license.Mode != model.D2SLicenseModeOffline && license.Mode != model.D2SLicenseModePermanent {
			return model.ErrD2SLicenseUnavailable
		}
		days := requestedDays
		if license.Mode == model.D2SLicenseModeOffline {
			if days != 7 && days != 14 && days != 30 {
				return model.ErrD2SLicenseUnavailable
			}
			if days != license.OfflinePeriodDays {
				return model.ErrD2SLicenseUnavailable
			}
		}
		expiresAt := now + int64(days)*86400
		if license.Mode == model.D2SLicenseModePermanent {
			days = 0
			expiresAt = 253402300799
		}
		if license.ExpiresAt > 0 && license.ExpiresAt < expiresAt {
			expiresAt = license.ExpiresAt
		}
		if license.OfflineValidUntil > expiresAt {
			expiresAt = license.OfflineValidUntil
		}
		claims = &D2SOfflineClaims{
			Version: 1, KeyID: keyID, EntitlementID: uuid.NewString(), LicenseID: license.ID,
			Product: model.D2SProductDesktop2Stereo, DeviceHash: license.DeviceHash, Mode: license.Mode,
			Features: []string{"runtime"}, IssuedAt: now, NotBefore: now - 60, ExpiresAt: expiresAt,
			Trial: license.Kind == model.D2SLicenseKindTrial, OfflinePeriodDays: days,
		}
		jws, err = signD2SClaims(key, keyID, *claims)
		if err != nil {
			return err
		}
		digest := sha256.Sum256([]byte(jws))
		keyMetadata := model.D2SSigningKey{
			KeyID: keyID, Algorithm: "ES256", PublicJWK: string(jwkJSON), Status: "active", CreatedAt: now,
		}
		if err := tx.Where("key_id = ?", keyID).Assign(map[string]any{"public_jwk": string(jwkJSON), "status": "active"}).FirstOrCreate(&keyMetadata).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.D2SOfflineEntitlement{
			ID: claims.EntitlementID, LicenseID: license.ID, UserID: userID, DeviceHash: license.DeviceHash,
			KeyID: keyID, IssuedAt: now, ExpiresAt: expiresAt, JWSHash: hex.EncodeToString(digest[:]),
		}).Error; err != nil {
			return err
		}
		return tx.Model(&model.D2SLicense{}).Where("id = ?", license.ID).Updates(map[string]any{
			"offline_valid_until": expiresAt, "updated_at": now,
		}).Error
	})
	if err != nil {
		return "", nil, err
	}
	return jws, claims, nil
}

type D2SDeviceAuthCredentials struct {
	AccessToken     string           `json:"access_token"`
	RefreshToken    string           `json:"refresh_token"`
	TokenType       string           `json:"token_type"`
	AccessExpiresAt int64            `json:"access_expires_at"`
	Session         LoginSessionView `json:"session"`
	UserID          int              `json:"user_id"`
	DeviceHash      string           `json:"device_hash"`
}

func ExchangeD2SDeviceCode(deviceSecret, ip, userAgent string, now int64) (*D2SDeviceAuthCredentials, error) {
	row, err := model.ClaimD2SDeviceCode(deviceSecret, now)
	if err != nil {
		return nil, err
	}
	bundle, err := CreateLoginSession(row.UserID, "desktop2stereo_device_code", ip, userAgent)
	if err != nil {
		_ = model.FinishD2SDeviceCodeClaim(row.ID, false, now)
		return nil, err
	}
	if err := model.FinishD2SDeviceCodeClaim(row.ID, true, now); err != nil {
		_, _ = model.RevokeUserSession(row.UserID, bundle.Session.SID, "device_code_claim_failed")
		return nil, err
	}
	return &D2SDeviceAuthCredentials{
		AccessToken: bundle.AccessToken, RefreshToken: bundle.RefreshToken, TokenType: bundle.TokenType,
		AccessExpiresAt: bundle.AccessExpiresAt, Session: bundle.Session, UserID: row.UserID, DeviceHash: row.DeviceHash,
	}, nil
}
