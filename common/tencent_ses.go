package common

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"
)

const (
	TencentSESAPIEndpoint = "https://ses.tencentcloudapi.com/"
	tencentSESService     = "ses"
	tencentSESVersion     = "2020-10-02"
)

var tencentSESAPIEndpoint = TencentSESAPIEndpoint

var (
	ErrTencentSESDisabled        = errors.New("Tencent SES email delivery is disabled")
	ErrTencentSESNotConfigured   = errors.New("Tencent SES email delivery is not configured")
	ErrTencentSESTemplateMissing = errors.New("Tencent SES template is not configured")
	ErrTencentSESVariableMissing = errors.New("Tencent SES template variable is missing")
)

var requiredTencentSESTemplateVariables = map[string][]string{
	"email_verification":  {"code"},
	"password_reset":      {"reset_url"},
	"system_notification": {"title", "content"},
}

type TencentSESTemplateMapping struct {
	Default   map[string]uint64
	Localized map[string]map[string]uint64
}

type TencentSESTemplateConfig struct {
	Enabled            bool                         `json:"enabled"`
	Region             string                       `json:"region"`
	SecretID           string                       `json:"secret_id"`
	SecretKey          string                       `json:"secret_key"`
	FromEmail          string                       `json:"from_email"`
	FromName           string                       `json:"from_name"`
	ReplyTo            string                       `json:"reply_to"`
	SubjectPrefix      string                       `json:"subject_prefix"`
	TimeoutSeconds     int                          `json:"timeout_seconds"`
	RetryCount         int                          `json:"retry_count"`
	Templates          map[string]uint64            `json:"templates"`
	LocalizedTemplates map[string]map[string]uint64 `json:"localized_templates"`
}

type TemplateEmailMessage struct {
	Scene        string
	Language     string
	TemplateID   uint64
	To           []string
	Cc           []string
	Bcc          []string
	From         string
	ReplyTo      string
	Subject      string
	TemplateData map[string]string
}

type TencentSESSendResult struct {
	MessageID string `json:"message_id"`
	RequestID string `json:"request_id"`
}

type tencentSESTemplate struct {
	TemplateID   uint64 `json:"TemplateID"`
	TemplateData string `json:"TemplateData"`
}

type tencentSESSendRequest struct {
	FromEmailAddress string             `json:"FromEmailAddress"`
	ReplyToAddresses string             `json:"ReplyToAddresses,omitempty"`
	Destination      []string           `json:"Destination,omitempty"`
	Cc               []string           `json:"Cc,omitempty"`
	Bcc              []string           `json:"Bcc,omitempty"`
	Subject          string             `json:"Subject"`
	Template         tencentSESTemplate `json:"Template"`
}

type tencentSESResponse struct {
	Response struct {
		Error struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		} `json:"Error"`
		RequestID string `json:"RequestId"`
		MessageID string `json:"MessageId"`
	} `json:"Response"`
}

func GetTencentSESConfig() TencentSESTemplateConfig {
	OptionMapRWMutex.RLock()
	defer OptionMapRWMutex.RUnlock()
	config := TencentSESTemplateConfig{
		Enabled:            OptionMap["TencentSESEnabled"] == "true",
		Region:             strings.TrimSpace(OptionMap["TencentSESRegion"]),
		SecretID:           strings.TrimSpace(OptionMap["TencentSESSecretId"]),
		SecretKey:          strings.TrimSpace(OptionMap["TencentSESSecretKey"]),
		FromEmail:          strings.TrimSpace(OptionMap["TencentSESFromEmail"]),
		FromName:           strings.TrimSpace(OptionMap["TencentSESFromName"]),
		ReplyTo:            strings.TrimSpace(OptionMap["TencentSESReplyTo"]),
		SubjectPrefix:      strings.TrimSpace(OptionMap["TencentSESSubjectPrefix"]),
		TimeoutSeconds:     positiveIntOption(OptionMap["TencentSESTimeoutSeconds"], 10),
		RetryCount:         nonNegativeIntOption(OptionMap["TencentSESRetryCount"], 1),
		Templates:          map[string]uint64{},
		LocalizedTemplates: map[string]map[string]uint64{},
	}
	if config.Region == "" {
		config.Region = "ap-guangzhou"
	}
	if raw := strings.TrimSpace(OptionMap["TencentSESTemplates"]); raw != "" {
		if mapping, err := parseTencentSESTemplateMapping(raw); err == nil {
			config.Templates = mapping.Default
			config.LocalizedTemplates = mapping.Localized
		}
	}
	return config
}

func TencentSESConfigPublic() map[string]any {
	config := GetTencentSESConfig()
	return map[string]any{
		"enabled":             config.Enabled,
		"region":              config.Region,
		"secret_id":           maskSecret(config.SecretID),
		"has_secret_key":      config.SecretKey != "",
		"from_email":          config.FromEmail,
		"from_name":           config.FromName,
		"reply_to":            config.ReplyTo,
		"subject_prefix":      config.SubjectPrefix,
		"timeout_seconds":     config.TimeoutSeconds,
		"retry_count":         config.RetryCount,
		"templates":           config.Templates,
		"localized_templates": config.LocalizedTemplates,
	}
}

func parseTencentSESTemplateMapping(raw string) (TencentSESTemplateMapping, error) {
	var entries map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return TencentSESTemplateMapping{}, err
	}
	mapping := TencentSESTemplateMapping{
		Default:   map[string]uint64{},
		Localized: map[string]map[string]uint64{},
	}
	for scene, value := range entries {
		if strings.TrimSpace(scene) == "" {
			return TencentSESTemplateMapping{}, errors.New("scene cannot be empty")
		}
		var templateID uint64
		if err := json.Unmarshal(value, &templateID); err == nil {
			if templateID == 0 {
				return TencentSESTemplateMapping{}, fmt.Errorf("scene %q has an invalid TemplateID", scene)
			}
			mapping.Default[scene] = templateID
			continue
		}
		var localized map[string]uint64
		if err := json.Unmarshal(value, &localized); err != nil || len(localized) == 0 {
			return TencentSESTemplateMapping{}, fmt.Errorf("scene %q must map to a TemplateID or language mapping", scene)
		}
		for language, id := range localized {
			if normalizeTencentSESLanguage(language) == "" || id == 0 {
				return TencentSESTemplateMapping{}, fmt.Errorf("scene %q has an invalid language or TemplateID", scene)
			}
		}
		mapping.Localized[scene] = localized
	}
	return mapping, nil
}

func ParseTencentSESTemplateMapping(raw string) error {
	_, err := parseTencentSESTemplateMapping(raw)
	return err
}

func normalizeTencentSESLanguage(language string) string {
	normalized := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(language, "_", "-")))
	switch normalized {
	case "zh", "zhcn", "zh-cn", "zh-hans":
		return "zhCN"
	case "zhtw", "zh-tw", "zh-hant", "zh-hk":
		return "zhCN"
	case "en":
		return "en"
	default:
		if strings.HasPrefix(normalized, "en-") {
			return "en"
		}
		return ""
	}
}

func resolveTencentSESTemplateID(config TencentSESTemplateConfig, scene, language string) uint64 {
	localized := config.LocalizedTemplates[scene]
	requested := normalizeTencentSESLanguage(language)
	for _, candidate := range []string{requested, "zhCN", "en"} {
		if candidate != "" && localized[candidate] != 0 {
			return localized[candidate]
		}
	}
	return config.Templates[scene]
}

func firstTencentSESTemplateID(config TencentSESTemplateConfig) uint64 {
	for _, templateID := range config.Templates {
		if templateID != 0 {
			return templateID
		}
	}
	for _, localized := range config.LocalizedTemplates {
		for _, templateID := range localized {
			if templateID != 0 {
				return templateID
			}
		}
	}
	return 0
}

func SendTencentSESTemplate(ctx context.Context, message TemplateEmailMessage) (TencentSESSendResult, error) {
	config := GetTencentSESConfig()
	if !config.Enabled {
		return TencentSESSendResult{}, ErrTencentSESDisabled
	}
	if err := validateTencentSESConfig(config); err != nil {
		return TencentSESSendResult{}, err
	}
	if message.TemplateID == 0 && message.Scene != "" {
		message.TemplateID = resolveTencentSESTemplateID(config, message.Scene, message.Language)
	}
	if message.TemplateID == 0 {
		return TencentSESSendResult{}, fmt.Errorf("%w: %s", ErrTencentSESTemplateMissing, message.Scene)
	}
	for _, variable := range requiredTencentSESTemplateVariables[message.Scene] {
		if strings.TrimSpace(message.TemplateData[variable]) == "" {
			return TencentSESSendResult{}, fmt.Errorf("%w: %s", ErrTencentSESVariableMissing, variable)
		}
	}
	if len(message.To) == 0 {
		return TencentSESSendResult{}, errors.New("at least one email recipient is required")
	}
	for _, recipient := range append(append(append([]string{}, message.To...), message.Cc...), message.Bcc...) {
		if _, err := mail.ParseAddress(strings.TrimSpace(recipient)); err != nil {
			return TencentSESSendResult{}, fmt.Errorf("invalid email recipient: %w", err)
		}
	}
	if message.From == "" {
		message.From = config.FromEmail
	}
	if err := validateEmailAddress(message.From); err != nil {
		return TencentSESSendResult{}, fmt.Errorf("invalid sender: %w", err)
	}
	if message.ReplyTo == "" {
		message.ReplyTo = config.ReplyTo
	}
	if message.ReplyTo != "" {
		if err := validateEmailAddress(message.ReplyTo); err != nil {
			return TencentSESSendResult{}, fmt.Errorf("invalid reply-to: %w", err)
		}
	}
	if config.FromName != "" && !strings.ContainsAny(config.FromName, "\\r\\n:") && !strings.Contains(message.From, "<") {
		message.From = config.FromName + " <" + message.From + ">"
	}
	if message.Subject == "" {
		message.Subject = message.Scene
	}
	message.Subject = config.SubjectPrefix + message.Subject

	templateData, err := json.Marshal(message.TemplateData)
	if err != nil {
		return TencentSESSendResult{}, fmt.Errorf("marshal template data: %w", err)
	}
	body, err := json.Marshal(tencentSESSendRequest{
		FromEmailAddress: message.From,
		ReplyToAddresses: message.ReplyTo,
		Destination:      message.To,
		Cc:               message.Cc,
		Bcc:              message.Bcc,
		Subject:          message.Subject,
		Template:         tencentSESTemplate{TemplateID: message.TemplateID, TemplateData: string(templateData)},
	})
	if err != nil {
		return TencentSESSendResult{}, fmt.Errorf("marshal Tencent SES request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= config.RetryCount; attempt++ {
		result, requestErr := sendTencentSESRequest(ctx, config, body)
		if requestErr == nil {
			return result, nil
		}
		lastErr = requestErr
		if attempt < config.RetryCount {
			select {
			case <-ctx.Done():
				return TencentSESSendResult{}, ctx.Err()
			case <-time.After(time.Duration(1<<attempt) * 200 * time.Millisecond):
			}
		}
	}
	return TencentSESSendResult{}, lastErr
}

func TestTencentSESConnection(ctx context.Context) error {
	config := GetTencentSESConfig()
	if !config.Enabled {
		return ErrTencentSESDisabled
	}
	if err := validateTencentSESConfig(config); err != nil {
		return err
	}
	templateID := firstTencentSESTemplateID(config)
	if templateID == 0 {
		return fmt.Errorf("%w: configure at least one TemplateID before testing the connection", ErrTencentSESTemplateMissing)
	}
	requestBody, err := json.Marshal(map[string]uint64{"TemplateID": templateID})
	if err != nil {
		return err
	}
	_, err = callTencentSESAPI(ctx, config, "GetEmailTemplate", requestBody)
	return err
}

func sendTencentSESRequest(ctx context.Context, config TencentSESTemplateConfig, body []byte) (TencentSESSendResult, error) {
	return callTencentSESAPI(ctx, config, "SendEmail", body)
}

func callTencentSESAPI(ctx context.Context, config TencentSESTemplateConfig, action string, body []byte) (TencentSESSendResult, error) {
	now := time.Now().UTC()
	timestamp := strconv.FormatInt(now.Unix(), 10)
	date := now.Format("2006-01-02")
	host := "ses.tencentcloudapi.com"
	contentType := "application/json; charset=utf-8"
	payloadHash := sha256Hex(body)
	canonicalHeaders := "content-type:" + contentType + "\nhost:" + host + "\n"
	signedHeaders := "content-type;host"
	canonicalRequest := "POST\n/\n\n" + canonicalHeaders + "\n" + signedHeaders + "\n" + payloadHash
	credentialScope := date + "/" + tencentSESService + "/tc3_request"
	stringToSign := "TC3-HMAC-SHA256\n" + timestamp + "\n" + credentialScope + "\n" + sha256Hex([]byte(canonicalRequest))
	secretDate := hmacSHA256([]byte("TC3"+config.SecretKey), date)
	secretService := hmacSHA256(secretDate, tencentSESService)
	secretSigning := hmacSHA256(secretService, "tc3_request")
	signature := hex.EncodeToString(hmacSHA256(secretSigning, stringToSign))
	authorization := fmt.Sprintf("TC3-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s", config.SecretID, credentialScope, signedHeaders, signature)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tencentSESAPIEndpoint, bytes.NewReader(body))
	if err != nil {
		return TencentSESSendResult{}, err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Host", host)
	req.Header.Set("X-TC-Action", action)
	req.Header.Set("X-TC-Version", tencentSESVersion)
	req.Header.Set("X-TC-Region", config.Region)
	req.Header.Set("X-TC-Timestamp", timestamp)
	req.Header.Set("Authorization", authorization)

	client := &http.Client{Timeout: time.Duration(config.TimeoutSeconds) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return TencentSESSendResult{}, err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return TencentSESSendResult{}, err
	}
	var decoded tencentSESResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return TencentSESSendResult{}, fmt.Errorf("Tencent SES returned invalid response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || decoded.Response.Error.Code != "" {
		return TencentSESSendResult{RequestID: decoded.Response.RequestID}, fmt.Errorf("Tencent SES %s: %s (request_id=%s)", decoded.Response.Error.Code, decoded.Response.Error.Message, decoded.Response.RequestID)
	}
	return TencentSESSendResult{MessageID: decoded.Response.MessageID, RequestID: decoded.Response.RequestID}, nil
}

func validateTencentSESConfig(config TencentSESTemplateConfig) error {
	if config.SecretID == "" || config.SecretKey == "" || config.FromEmail == "" {
		return ErrTencentSESNotConfigured
	}
	if err := validateEmailAddress(config.FromEmail); err != nil {
		return fmt.Errorf("invalid Tencent SES sender: %w", err)
	}
	if config.ReplyTo != "" {
		if err := validateEmailAddress(config.ReplyTo); err != nil {
			return fmt.Errorf("invalid Tencent SES reply-to: %w", err)
		}
	}
	return nil
}

func validateEmailAddress(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("email address is empty")
	}
	if _, err := mail.ParseAddress(value); err != nil {
		return err
	}
	return nil
}

func hmacSHA256(key []byte, value string) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(value))
	return h.Sum(nil)
}

func sha256Hex(value []byte) string {
	hash := sha256.Sum256(value)
	return hex.EncodeToString(hash[:])
}

func positiveIntOption(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func nonNegativeIntOption(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func maskSecret(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 4 {
		return "****"
	}
	return value[:2] + "****" + value[len(value)-2:]
}
