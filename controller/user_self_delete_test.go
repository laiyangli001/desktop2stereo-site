package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeleteSelfAnonymizesAuthenticatedAccountAndAuditsRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupD2SAdminExportTestDB(t, "d2s_delete_self_test",
		&model.User{}, &model.Token{}, &model.TwoFA{}, &model.TwoFABackupCode{},
		&model.UserSession{}, &model.AuthFlow{}, &model.PasskeyCredential{},
		&model.UserOAuthBinding{}, &model.ExternalIdentityClaim{}, &model.D2SUserProfile{},
		&model.D2SUserSecurityControl{}, &model.D2SRegistrationRecord{},
		&model.D2SUserActionLog{}, &model.D2SSupportTicket{},
	)
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedisEnabled })

	user := model.User{
		Username: "self-delete-user", Password: "unused-hash", Email: "self-delete@example.com",
		AffCode: "self-delete-aff", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
	}
	require.NoError(t, db.Create(&user).Error)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("id", user.Id)
	context.Request = httptest.NewRequest(http.MethodDelete, "/api/user/self", nil)
	context.Request.RemoteAddr = "192.0.2.21:4321"
	context.Request.Header.Set("User-Agent", "self-delete-controller-test")

	DeleteSelf(context)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)
	var anonymized model.User
	require.NoError(t, db.Unscoped().First(&anonymized, "id = ?", user.Id).Error)
	assert.True(t, anonymized.DeletedAt.Valid)
	assert.Equal(t, common.UserStatusDisabled, anonymized.Status)
	assert.NotEqual(t, user.Email, anonymized.Email)

	var action model.D2SUserActionLog
	require.NoError(t, db.Where("user_id = ? AND action_type = ?", user.Id, model.D2SActionProfileUpdate).First(&action).Error)
	require.NotNil(t, action.UserID)
	assert.Equal(t, user.Id, *action.UserID)
	assert.Equal(t, user.Email, action.Email)
	assert.Equal(t, "192.0.2.21", action.SourceIP)
	require.NotNil(t, action.SourcePort)
	assert.Equal(t, 4321, *action.SourcePort)
	assert.Equal(t, "self-delete-controller-test", action.UserAgent)
	assert.Equal(t, "/api/user/self", action.RequestPath)
	validHash, err := model.VerifyD2SUserActionLogHash(action.ID)
	require.NoError(t, err)
	assert.True(t, validHash)
}

func TestDeleteSelfRejectsRootAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupD2SAdminExportTestDB(t, "d2s_delete_root_self_test", &model.User{})
	root := model.User{
		Username: "root-self-delete", Password: "unused-hash", Email: "root-self-delete@example.com",
		AffCode: "root-self-delete-aff", Role: common.RoleRootUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
	}
	require.NoError(t, db.Create(&root).Error)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("id", root.Id)
	context.Request = httptest.NewRequest(http.MethodDelete, "/api/user/self", nil)

	DeleteSelf(context)

	assert.Contains(t, recorder.Body.String(), `"success":false`)
	var unchanged model.User
	require.NoError(t, db.First(&unchanged, root.Id).Error)
	assert.Equal(t, common.RoleRootUser, unchanged.Role)
	assert.Equal(t, common.UserStatusEnabled, unchanged.Status)
}
