package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSystemBootstrapRootLoginEmailPolicy(t *testing.T) {
	for _, test := range []struct {
		name     string
		role     int
		email    string
		verified int64
		blocked  bool
		status   int
	}{
		{"initial_root", common.RoleRootUser, "", 0, false, http.StatusOK},
		{"unverified_user", common.RoleCommonUser, "user@example.com", 0, false, http.StatusForbidden},
		{"user_without_email", common.RoleCommonUser, "", 0, false, http.StatusForbidden},
		{"root_with_unverified_email", common.RoleRootUser, "root@example.com", 0, false, http.StatusForbidden},
		{"unverified_admin", common.RoleAdminUser, "", 0, false, http.StatusForbidden},
		{"verified_user", common.RoleCommonUser, "verified@example.com", 1793700000, false, http.StatusOK},
		{"blocked_initial_root", common.RoleRootUser, "", 0, true, http.StatusLocked},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.Log{},
				&model.D2SUserActionLog{}, &model.D2SUserSecurityControl{}, &model.D2SUserProfile{}))
			previousDB, previousLog, previousRedis := model.DB, model.LOG_DB, common.RedisEnabled
			model.DB, model.LOG_DB, common.RedisEnabled = db, db, false
			t.Cleanup(func() { model.DB, model.LOG_DB, common.RedisEnabled = previousDB, previousLog, previousRedis })
			user := &model.User{
				Username: "login-policy-user", Password: "unused-test-hash", Role: test.role,
				Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
				Email: test.email, EmailVerifiedAt: test.verified,
			}
			require.NoError(t, db.Create(user).Error)
			if test.blocked {
				require.NoError(t, db.Create(&model.D2SUserSecurityControl{
					UserID: user.Id, LoginBlocked: true, Reason: "explicit account restriction",
				}).Error)
			}
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodPost, "/api/user/login", nil)
			context.Request.RemoteAddr = "192.0.2.10:41234"
			setupLogin(user, context)
			require.Equal(t, test.status, recorder.Code, recorder.Body.String())
			var response struct {
				Success bool   `json:"success"`
				Code    string `json:"code"`
				Data    struct {
					AccessToken string `json:"access_token"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			if test.status == http.StatusOK {
				require.True(t, response.Success)
				require.NotEmpty(t, response.Data.AccessToken)
			} else {
				require.False(t, response.Success)
				require.Empty(t, response.Data.AccessToken)
			}
			if test.status == http.StatusForbidden {
				require.Equal(t, "EMAIL_VERIFICATION_REQUIRED", response.Code)
			}
			if test.name == "initial_root" {
				var stored model.User
				require.NoError(t, db.First(&stored, user.Id).Error)
				require.Zero(t, stored.EmailVerifiedAt, "management login must not pretend the email was verified")
				_, err = model.EnsureD2SProfileAndTrial(user.Id, 1793700000)
				require.ErrorIs(t, err, model.ErrD2SEmailVerificationRequired)
			}
		})
	}
}
