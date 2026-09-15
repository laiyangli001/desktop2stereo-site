package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
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
	"io"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/google/uuid"
	"golang.org/x/crypto/hkdf"
	"gorm.io/gorm"
)

var (
	ErrD2SSigningKeyMissing  = errors.New("Desktop2Stereo signing key is not configured")
	ErrD2SSigningKeyInvalid  = errors.New("Desktop2Stereo signing key is invalid")
	ErrD2SSigningKeyCurrent  = errors.New("the active Desktop2Stereo signing key cannot be retired")
	ErrD2SSigningKeyNotFound = errors.New("Desktop2Stereo signing key was not found")
)

const D2SDefaultLicenseKeyID = "d2s-es256-2026-09"

type D2SOfflineClaims struct {
	Version                   int      `json:"version"`
	KeyID                     string   `json:"key_id"`
	EntitlementID             string   `json:"entitlement_id"`
	LicenseID                 string   `json:"license_id"`
	Product                   string   `json:"product"`
	DeviceHash                string   `json:"device_hash"`
	Mode                      string   `json:"mode"`
	Features                  []string `json:"features"`
	IssuedAt                  int64    `json:"issued_at"`
	NotBefore                 int64    `json:"not_before"`
	ExpiresAt                 int64    `json:"expires_at"`
	Trial                     bool     `json:"trial"`
	OfflinePeriodDays         int      `json:"offline_period_days"`
	CoreID                    string   `json:"core_id,omitempty"`
	CoreVersion               int      `json:"core_version,omitempty"`
	ResourceSHA256            string   `json:"resource_sha256,omitempty"`
	WrappedCoreKey            string   `json:"wrapped_core_key,omitempty"`
	KeyWrapEphemeralPublicKey string   `json:"key_wrap_ephemeral_public_key,omitempty"`
	KeyWrapNonce              string   `json:"key_wrap_nonce,omitempty"`
}

const D2SParallaxCoreID = "parallax-core"
const D2SParallaxCoreVersion = 1

type D2SCoreGrantClaims struct {
	Version                   int    `json:"version"`
	KeyID                     string `json:"key_id"`
	GrantID                   string `json:"grant_id"`
	LicenseID                 string `json:"license_id"`
	Product                   string `json:"product"`
	DeviceHash                string `json:"device_hash"`
	CoreID                    string `json:"core_id"`
	CoreVersion               int    `json:"core_version"`
	ResourceSHA256            string `json:"resource_sha256"`
	WrappedCoreKey            string `json:"wrapped_core_key"`
	KeyWrapEphemeralPublicKey string `json:"key_wrap_ephemeral_public_key"`
	KeyWrapNonce              string `json:"key_wrap_nonce"`
	IssuedAt                  int64  `json:"issued_at"`
	NotBefore                 int64  `json:"not_before"`
	ExpiresAt                 int64  `json:"expires_at"`
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
		return D2SDefaultLicenseKeyID
	}
	return keyID
}

// IsD2SCurrentSigningKey reports whether a persisted key is configured for new signatures.
func IsD2SCurrentSigningKey(keyID string) bool {
	return strings.TrimSpace(keyID) == d2sConfiguredKeyID()
}

func parseD2SPrivateKey(raw []byte) (*ecdsa.PrivateKey, error) {
	if block, _ := pem.Decode(raw); block != nil {
		raw = block.Bytes
	}
	var key *ecdsa.PrivateKey
	if parsed, err := x509.ParsePKCS8PrivateKey(raw); err == nil {
		key, _ = parsed.(*ecdsa.PrivateKey)
	}
	if key == nil {
		parsed, err := x509.ParseECPrivateKey(raw)
		if err != nil {
			return nil, ErrD2SSigningKeyInvalid
		}
		key = parsed
	}
	if key.Curve != elliptic.P256() {
		return nil, fmt.Errorf("%w: expected P-256", ErrD2SSigningKeyInvalid)
	}
	return key, nil
}

func d2sSigningKey() (*ecdsa.PrivateKey, string, error) {
	keyID := d2sConfiguredKeyID()
	raw := strings.TrimSpace(os.Getenv("D2S_LICENSE_PRIVATE_KEY_PEM"))
	if encoded := strings.TrimSpace(os.Getenv("D2S_LICENSE_PRIVATE_KEY_B64")); raw == "" && encoded != "" {
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, "", fmt.Errorf("%w: base64 decode failed", ErrD2SSigningKeyInvalid)
		}
		key, err := parseD2SPrivateKey(decoded)
		if err != nil {
			return nil, "", err
		}
		return key, keyID, nil
	}
	if raw == "" {
		return nil, "", ErrD2SSigningKeyMissing
	}
	raw = strings.ReplaceAll(raw, `\n`, "\n")
	key, err := parseD2SPrivateKey([]byte(raw))
	if err != nil {
		return nil, "", err
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
	return signD2SPayload(key, keyID, claims)
}

func signD2SPayload(key *ecdsa.PrivateKey, keyID string, claims any) (string, error) {
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

func d2sParallaxCoreConfig() (string, string, error) {
	resourceHash := strings.ToLower(strings.TrimSpace(os.Getenv("D2S_PARALLAX_CORE_SHA256")))
	if len(resourceHash) != 64 {
		return "", "", fmt.Errorf("Desktop2Stereo parallax resource hash is not configured")
	}
	if _, err := hex.DecodeString(resourceHash); err != nil {
		return "", "", fmt.Errorf("Desktop2Stereo parallax resource hash is invalid")
	}
	coreKey := strings.ToLower(strings.TrimSpace(os.Getenv("D2S_PARALLAX_CORE_KEY_HEX")))
	decoded, err := hex.DecodeString(coreKey)
	if err != nil || len(decoded) != 32 {
		return "", "", fmt.Errorf("Desktop2Stereo parallax core key is invalid")
	}
	return resourceHash, coreKey, nil
}

func wrapD2SCoreKey(coreKeyHex, devicePublicKey, grantID, licenseID, deviceHash, coreID string, coreVersion int, resourceHash string) (string, string, string, error) {
	coreKey, err := hex.DecodeString(coreKeyHex)
	if err != nil || len(coreKey) != 32 {
		return "", "", "", fmt.Errorf("Desktop2Stereo parallax core key is invalid")
	}
	publicBytes, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(devicePublicKey))
	if err != nil || len(publicBytes) != 32 {
		return "", "", "", fmt.Errorf("Desktop2Stereo device core key is invalid")
	}
	devicePublic, err := ecdh.X25519().NewPublicKey(publicBytes)
	if err != nil {
		return "", "", "", fmt.Errorf("Desktop2Stereo device core key is invalid")
	}
	ephemeral, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", "", err
	}
	shared, err := ephemeral.ECDH(devicePublic)
	if err != nil {
		return "", "", "", fmt.Errorf("Desktop2Stereo device core key exchange failed")
	}
	wrappingKey := make([]byte, 32)
	reader := hkdf.New(sha256.New, shared, nil, []byte("d2s-parallax-core-v1-key-wrap"))
	if _, err := io.ReadFull(reader, wrappingKey); err != nil {
		return "", "", "", err
	}
	block, err := aes.NewCipher(wrappingKey)
	if err != nil {
		return "", "", "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", "", "", err
	}
	aad := []byte(strings.Join([]string{grantID, licenseID, deviceHash, coreID, fmt.Sprint(coreVersion), resourceHash}, "|"))
	ciphertext := gcm.Seal(nil, nonce, coreKey, aad)
	encode := base64.RawURLEncoding.EncodeToString
	return encode(ephemeral.PublicKey().Bytes()), encode(nonce), encode(ciphertext), nil
}

func IssueD2SCoreGrant(userID int, licenseID, deviceHash, devicePublicKey, coreID string, coreVersion int, now int64) (string, *D2SCoreGrantClaims, error) {
	if now <= 0 {
		now = time.Now().Unix()
	}
	if coreID != D2SParallaxCoreID || coreVersion != D2SParallaxCoreVersion {
		return "", nil, model.ErrD2SLicenseUnavailable
	}
	resourceHash, coreKey, err := d2sParallaxCoreConfig()
	if err != nil {
		return "", nil, err
	}
	key, keyID, err := d2sSigningKey()
	if err != nil {
		return "", nil, err
	}
	var grant *D2SCoreGrantClaims
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		var license model.D2SLicense
		if err := model.LockForUpdate(tx).Where("id = ? AND user_id = ?", licenseID, userID).First(&license).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return model.ErrD2SLicenseNotFound
			}
			return err
		}
		if license.Product != model.D2SProductDesktop2Stereo || license.Status != model.D2SLicenseStatusActive ||
			license.DeviceHash != strings.ToLower(strings.TrimSpace(deviceHash)) || (license.ExpiresAt > 0 && license.ExpiresAt <= now) {
			return model.ErrD2SLicenseUnavailable
		}
		expiresAt := now + int64(30*time.Minute/time.Second)
		if license.Mode == model.D2SLicenseModeOnline {
			expiresAt = now + int64(model.D2SOnlineLeaseTTL/time.Second)
		}
		if license.ExpiresAt > 0 && license.ExpiresAt < expiresAt {
			expiresAt = license.ExpiresAt
		}
		grantID := uuid.NewString()
		ephemeral, nonce, wrapped, wrapErr := wrapD2SCoreKey(coreKey, devicePublicKey, grantID, license.ID, license.DeviceHash, coreID, coreVersion, resourceHash)
		if wrapErr != nil {
			return wrapErr
		}
		grant = &D2SCoreGrantClaims{
			Version: 1, KeyID: keyID, GrantID: grantID, LicenseID: license.ID,
			Product: license.Product, DeviceHash: license.DeviceHash, CoreID: coreID,
			CoreVersion: coreVersion, ResourceSHA256: resourceHash, WrappedCoreKey: wrapped,
			KeyWrapEphemeralPublicKey: ephemeral, KeyWrapNonce: nonce,
			IssuedAt: now, NotBefore: now - 60, ExpiresAt: expiresAt,
		}
		return nil
	})
	if err != nil {
		return "", nil, err
	}
	jws, err := signD2SPayload(key, keyID, *grant)
	if err != nil {
		return "", nil, err
	}
	return jws, grant, nil
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

func IssueD2SOfflineEntitlement(userID int, licenseID, deviceHash, devicePublicKey string, requestedDays int, now int64) (string, *D2SOfflineClaims, error) {
	if now <= 0 {
		now = time.Now().Unix()
	}
	resourceHash, coreKey, err := d2sParallaxCoreConfig()
	if err != nil {
		return "", nil, err
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
		entitlementID := uuid.NewString()
		ephemeral, nonce, wrapped, wrapErr := wrapD2SCoreKey(coreKey, devicePublicKey, entitlementID, license.ID, license.DeviceHash, D2SParallaxCoreID, D2SParallaxCoreVersion, resourceHash)
		if wrapErr != nil {
			return wrapErr
		}
		claims = &D2SOfflineClaims{
			Version: 1, KeyID: keyID, EntitlementID: entitlementID, LicenseID: license.ID,
			Product: model.D2SProductDesktop2Stereo, DeviceHash: license.DeviceHash, Mode: license.Mode,
			Features: []string{"runtime"}, IssuedAt: now, NotBefore: now - 60, ExpiresAt: expiresAt,
			Trial: license.Kind == model.D2SLicenseKindTrial, OfflinePeriodDays: days,
			CoreID: D2SParallaxCoreID, CoreVersion: D2SParallaxCoreVersion,
			ResourceSHA256: resourceHash, WrappedCoreKey: wrapped,
			KeyWrapEphemeralPublicKey: ephemeral, KeyWrapNonce: nonce,
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
