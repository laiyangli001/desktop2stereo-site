package common

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func withTencentSESOptions(t *testing.T, values map[string]string) {
	t.Helper()
	OptionMapRWMutex.Lock()
	previous := OptionMap
	OptionMap = make(map[string]string, len(values))
	for key, value := range values {
		OptionMap[key] = value
	}
	OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		OptionMapRWMutex.Lock()
		OptionMap = previous
		OptionMapRWMutex.Unlock()
	})
}

func TestGetTencentSESConfigUsesSafeDefaults(t *testing.T) {
	withTencentSESOptions(t, map[string]string{"TencentSESEnabled": "true", "TencentSESSecretId": "sid", "TencentSESSecretKey": "key", "TencentSESFromEmail": "noreply@example.com"})

	config := GetTencentSESConfig()
	require.True(t, config.Enabled)
	require.Equal(t, "ap-guangzhou", config.Region)
	require.Equal(t, 10, config.TimeoutSeconds)
	require.Equal(t, 1, config.RetryCount)
	require.Empty(t, config.Templates)
}

func TestTencentSESTemplateMappingSelectsLanguageAndKeepsLegacyFormat(t *testing.T) {
	mapping, err := parseTencentSESTemplateMapping(`{"email_verification":{"zhCN":123,"en":456},"password_reset":789}`)
	require.NoError(t, err)
	config := TencentSESTemplateConfig{
		Templates:          mapping.Default,
		LocalizedTemplates: mapping.Localized,
	}
	require.Equal(t, uint64(123), resolveTencentSESTemplateID(config, "email_verification", "zh-CN"))
	require.Equal(t, uint64(456), resolveTencentSESTemplateID(config, "email_verification", "en-US"))
	require.Equal(t, uint64(789), resolveTencentSESTemplateID(config, "password_reset", "en"))
	require.Equal(t, uint64(789), firstTencentSESTemplateID(config))
}

func TestTencentSESConfigPublicDoesNotExposeSecretKey(t *testing.T) {
	withTencentSESOptions(t, map[string]string{
		"TencentSESEnabled":   "true",
		"TencentSESSecretId":  "fake-test-id",
		"TencentSESSecretKey": "fake-test-key",
		"TencentSESFromEmail": "noreply@example.com",
		"TencentSESTemplates": `{"email_verification":123}`,
	})

	public := TencentSESConfigPublic()
	require.Equal(t, "fa****id", public["secret_id"])
	require.Equal(t, true, public["has_secret_key"])
	require.NotContains(t, public, "secret_key")
}

func TestSendTencentSESTemplateRejectsMissingConfigurationBeforeNetwork(t *testing.T) {
	withTencentSESOptions(t, map[string]string{"TencentSESEnabled": "true"})

	_, err := SendTencentSESTemplate(context.Background(), TemplateEmailMessage{
		Scene: "email_verification",
		To:    []string{"receiver@example.com"},
	})
	require.ErrorIs(t, err, ErrTencentSESNotConfigured)
}

func TestSendTencentSESTemplateRejectsMissingTemplateBeforeNetwork(t *testing.T) {
	withTencentSESOptions(t, map[string]string{
		"TencentSESEnabled":   "true",
		"TencentSESSecretId":  "sid",
		"TencentSESSecretKey": "key",
		"TencentSESFromEmail": "noreply@example.com",
	})

	_, err := SendTencentSESTemplate(context.Background(), TemplateEmailMessage{
		Scene: "email_verification",
		To:    []string{"receiver@example.com"},
	})
	require.ErrorIs(t, err, ErrTencentSESTemplateMissing)
}

func TestSendTencentSESTemplateRejectsMissingRequiredVariableBeforeNetwork(t *testing.T) {
	withTencentSESOptions(t, map[string]string{
		"TencentSESEnabled":   "true",
		"TencentSESSecretId":  "sid",
		"TencentSESSecretKey": "key",
		"TencentSESFromEmail": "noreply@example.com",
		"TencentSESTemplates": `{"password_reset":123}`,
	})

	_, err := SendTencentSESTemplate(context.Background(), TemplateEmailMessage{
		Scene:        "password_reset",
		To:           []string{"receiver@example.com"},
		TemplateData: map[string]string{},
	})
	require.ErrorIs(t, err, ErrTencentSESVariableMissing)
}

func TestSendTencentSESTemplateSignsAndSendsTemplateRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, http.MethodPost, request.Method)
		require.Equal(t, "SendEmail", request.Header.Get("X-TC-Action"))
		require.NotEmpty(t, request.Header.Get("Authorization"))
		var payload struct {
			Destination []string `json:"Destination"`
			Template    struct {
				TemplateID uint64 `json:"TemplateID"`
				Data       string `json:"TemplateData"`
			} `json:"Template"`
		}
		require.NoError(t, json.NewDecoder(request.Body).Decode(&payload))
		require.Equal(t, []string{"receiver@example.com"}, payload.Destination)
		require.Equal(t, uint64(123), payload.Template.TemplateID)
		require.JSONEq(t, `{"code":"123456"}`, payload.Template.Data)
		_, _ = writer.Write([]byte(`{"Response":{"RequestId":"req-test","MessageId":"msg-test"}}`))
	}))
	defer server.Close()
	previousEndpoint := tencentSESAPIEndpoint
	tencentSESAPIEndpoint = server.URL
	t.Cleanup(func() { tencentSESAPIEndpoint = previousEndpoint })
	withTencentSESOptions(t, map[string]string{
		"TencentSESEnabled":   "true",
		"TencentSESRegion":    "ap-guangzhou",
		"TencentSESSecretId":  "sid",
		"TencentSESSecretKey": "key",
		"TencentSESFromEmail": "noreply@example.com",
		"TencentSESTemplates": `{"email_verification":123}`,
	})

	result, err := SendTencentSESTemplate(context.Background(), TemplateEmailMessage{
		Scene:        "email_verification",
		To:           []string{"receiver@example.com"},
		Subject:      "verify",
		TemplateData: map[string]string{"code": "123456"},
	})
	require.NoError(t, err)
	require.Equal(t, "req-test", result.RequestID)
	require.Equal(t, "msg-test", result.MessageID)
}
