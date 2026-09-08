package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/require"
)

func TestFillTencentSESTestData(t *testing.T) {
	previousAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://100393.com"
	t.Cleanup(func() { system_setting.ServerAddress = previousAddress })

	verification := fillTencentSESTestData("email_verification", nil)
	require.Equal(t, "123456", verification["code"])
	require.NotEmpty(t, verification["expire_minutes"])

	reset := fillTencentSESTestData("password_reset", map[string]string{"system_name": "D2S"})
	require.Contains(t, reset["reset_url"], "https://100393.com/user/reset")
	require.Equal(t, "D2S", reset["system_name"])

	notification := fillTencentSESTestData("system_notification", nil)
	require.Equal(t, "测试通知", notification["title"])
	require.NotEmpty(t, notification["content"])

	custom := fillTencentSESTestData("email_verification", map[string]string{"code": "654321"})
	require.Equal(t, "654321", custom["code"])
}
