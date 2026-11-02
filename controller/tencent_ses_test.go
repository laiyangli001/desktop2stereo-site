package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/require"
)

func TestFillTencentSESTestData(t *testing.T) {
	previousAddress := system_setting.ServerAddress
	previousVerificationMinutes := common.VerificationValidMinutes
	system_setting.ServerAddress = "https://d2s.site"
	common.VerificationValidMinutes = 20
	t.Cleanup(func() {
		system_setting.ServerAddress = previousAddress
		common.VerificationValidMinutes = previousVerificationMinutes
	})

	verification := fillTencentSESTestData("email_verification", nil)
	require.Equal(t, "123456", verification["code"])
	require.Equal(t, "15", verification["expire_minutes"])

	reset := fillTencentSESTestData("password_reset", map[string]string{"system_name": "D2S"})
	require.Contains(t, reset["reset_url"], "https://d2s.site/user/reset")
	require.Equal(t, "D2S", reset["system_name"])
	require.Equal(t, "15", reset["expire_minutes"])

	notification := fillTencentSESTestData("system_notification", nil)
	require.Equal(t, "测试通知", notification["title"])
	require.NotEmpty(t, notification["content"])

	custom := fillTencentSESTestData("email_verification", map[string]string{"code": "654321"})
	require.Equal(t, "654321", custom["code"])
}
