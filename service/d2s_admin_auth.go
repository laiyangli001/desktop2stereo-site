package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/model"
)

const (
	D2SAdminSessionCookieName = "d2s_admin_session"
	D2SAdminSessionLifetime   = 12 * time.Hour
)

var (
	ErrD2SAdminAuthentication = errors.New("admin authentication failed")
	ErrD2SAdminSessionInvalid = errors.New("admin session is invalid")
	ErrD2SAdminLoginLocked    = errors.New("admin account is temporarily locked")
)

func d2sAdminTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func AuthenticateD2SAdmin(email, password, sourceIP, userAgent string, now int64) (*model.D2SAdminAccount, string, error) {
	email = model.NormalizeEmail(email)
	account, err := model.GetD2SAdminAccountByEmail(email)
	if err != nil {
		return nil, "", ErrD2SAdminAuthentication
	}
	if now <= 0 {
		now = time.Now().UTC().Unix()
	}
	if account.Status != model.D2SAdminStatusActive {
		return nil, "", ErrD2SAdminAuthentication
	}
	if account.LockedUntil > now {
		return nil, "", ErrD2SAdminLoginLocked
	}
	if !model.VerifyD2SAdminPassword(password, account.PasswordHash) {
		_, locked, failureErr := model.RecordD2SAdminLoginFailure(email, now, 5, int64((15 * time.Minute).Seconds()), int64((15 * time.Minute).Seconds()))
		if failureErr != nil {
			return nil, "", failureErr
		}
		if locked {
			return nil, "", ErrD2SAdminLoginLocked
		}
		return nil, "", ErrD2SAdminAuthentication
	}
	randomToken := make([]byte, 32)
	if _, err := rand.Read(randomToken); err != nil {
		return nil, "", err
	}
	rawToken := base64.RawURLEncoding.EncodeToString(randomToken)
	session := &model.D2SAdminSession{
		TokenHash: d2sAdminTokenHash(rawToken), AdminID: account.ID, CreatedAt: now,
		ExpiresAt: time.Unix(now, 0).Add(D2SAdminSessionLifetime).Unix(),
		SourceIP:  strings.TrimSpace(sourceIP), UserAgent: userAgent,
	}
	if len(session.SourceIP) > 64 {
		session.SourceIP = session.SourceIP[:64]
	}
	if len(session.UserAgent) > 512 {
		session.UserAgent = session.UserAgent[:512]
	}
	if err := model.RecordD2SAdminLoginSuccess(account.ID, now); err != nil {
		return nil, "", err
	}
	if err := model.CreateD2SAdminSession(session); err != nil {
		return nil, "", err
	}
	return account, rawToken, nil
}

func ValidateD2SAdminSession(rawToken string, now int64) (*model.D2SAdminSession, *model.D2SAdminAccount, error) {
	if strings.TrimSpace(rawToken) == "" {
		return nil, nil, ErrD2SAdminSessionInvalid
	}
	if now <= 0 {
		now = time.Now().UTC().Unix()
	}
	session, account, err := model.GetActiveD2SAdminSession(d2sAdminTokenHash(rawToken), now)
	if err != nil {
		return nil, nil, ErrD2SAdminSessionInvalid
	}
	return session, account, nil
}

func RevokeD2SAdminSession(rawToken string, now int64) error {
	if strings.TrimSpace(rawToken) == "" {
		return ErrD2SAdminSessionInvalid
	}
	return model.RevokeD2SAdminSession(d2sAdminTokenHash(rawToken), now)
}
