package model

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestD2SSupportTicketCreationAndOwnerIsolation(t *testing.T) {
	useD2STestDB(t)
	require.NoError(t, DB.AutoMigrate(&D2SSupportTicket{}))

	firstUser := createD2STestUser(t, "support-first")
	secondUser := createD2STestUser(t, "support-second")
	first := &D2SSupportTicket{
		UserID: firstUser.Id, Category: "PAYMENT", ContactEmail: " USER@EXAMPLE.COM ",
		Subject: "Payment not reflected", Message: "My payment succeeded but the account does not show the order.",
		SourceIP: "192.0.2.11", UserAgent: "test-agent",
	}
	second := &D2SSupportTicket{
		UserID: secondUser.Id, Category: "SOFTWARE", ContactEmail: "other@example.com",
		Subject: "Application help", Message: "The application closes when I open a project.",
	}
	require.NoError(t, CreateD2SSupportTicket(first))
	require.NoError(t, CreateD2SSupportTicket(second))
	assert.NotEmpty(t, first.ID)
	assert.Equal(t, "user@example.com", first.ContactEmail)
	assert.Equal(t, D2SSupportOpen, first.Status)
	assert.NotZero(t, first.CreatedAt)

	items, total, err := ListD2SUserSupportTickets(firstUser.Id, 1, 50)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, first.ID, items[0].ID)
	encoded, err := json.Marshal(items[0])
	require.NoError(t, err)
	for _, privateField := range []string{"contact_email", "source_ip", "user_agent", "handled_by_email"} {
		assert.NotContains(t, string(encoded), privateField)
	}

	items, total, err = ListD2SUserSupportTickets(secondUser.Id, 1, 50)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, second.ID, items[0].ID)

	_, total, err = ListD2SUserSupportTickets(firstUser.Id, 0, 101)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	assert.Error(t, func() error {
		_, _, err := ListD2SUserSupportTickets(0, 1, 50)
		return err
	}())
}

func TestD2SSupportTicketRequiresActiveVerifiedOwner(t *testing.T) {
	useD2STestDB(t)
	require.NoError(t, DB.AutoMigrate(&D2SSupportTicket{}))

	verified := createD2STestUser(t, "support-verified-owner")
	unverified := User{
		Username: "support-unverified-owner", Password: "unused-hash", Email: "support-unverified-owner@example.com",
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
	}
	require.NoError(t, DB.Create(&unverified).Error)
	disabled := createD2STestUser(t, "support-disabled-owner")
	disabled.Status = common.UserStatusDisabled
	require.NoError(t, DB.Save(&disabled).Error)
	deleted := createD2STestUser(t, "support-deleted-owner")
	require.NoError(t, DB.Delete(&deleted).Error)

	for _, userID := range []int{unverified.Id, disabled.Id, deleted.Id, 99999} {
		ticket := &D2SSupportTicket{
			UserID: userID, Category: "ACCOUNT", ContactEmail: "user@example.com",
			Subject: "Account help", Message: "Please help me resolve this account issue.",
		}
		assert.ErrorIs(t, CreateD2SSupportTicket(ticket), ErrD2SSupportOwnerUnavailable)
	}

	var count int64
	require.NoError(t, DB.Model(&D2SSupportTicket{}).Count(&count).Error)
	assert.Zero(t, count)
	ticket := &D2SSupportTicket{
		UserID: verified.Id, Category: "ACCOUNT", ContactEmail: "user@example.com",
		Subject: "Account help", Message: "Please help me resolve this account issue.",
	}
	require.NoError(t, CreateD2SSupportTicket(ticket))
}

func TestD2SSupportTicketValidationAndAuditTransaction(t *testing.T) {
	useD2STestDB(t)
	require.NoError(t, DB.AutoMigrate(&D2SSupportTicket{}, &D2SAdminOperationAudit{}))
	user := createD2STestUser(t, "support-audit-owner")

	for _, ticket := range []*D2SSupportTicket{
		nil,
		{UserID: 0, ContactEmail: "user@example.com", Subject: "Help", Message: "More than ten characters"},
		{UserID: 1, ContactEmail: "user@example.com", Subject: " ", Message: "More than ten characters"},
		{UserID: 1, ContactEmail: "user@example.com", Subject: "Help", Message: "short"},
	} {
		assert.Error(t, CreateD2SSupportTicket(ticket))
	}

	ticket := &D2SSupportTicket{
		UserID: user.Id, Category: "PAYMENT", ContactEmail: "user@example.com",
		Subject: "Payment not reflected", Message: "My payment succeeded but the account does not show the order.",
	}
	require.NoError(t, CreateD2SSupportTicket(ticket))
	assert.False(t, ValidD2SSupportReplyLength(strings.Repeat("x", 5001)))
	assert.Error(t, func() error {
		_, err := UpdateD2SSupportTicketWithAudit(ticket.ID, "UNKNOWN", "reply", 7, "admin@example.com", "192.0.2.7")
		return err
	}())

	require.NoError(t, DB.Exec("CREATE TRIGGER fail_support_audit BEFORE INSERT ON d2s_admin_operation_audits BEGIN SELECT RAISE(ABORT, 'forced support audit failure'); END;").Error)
	_, err := UpdateD2SSupportTicketWithAudit(ticket.ID, D2SSupportResolved, "Resolved", 7, "admin@example.com", "192.0.2.7")
	require.Error(t, err)
	var stored D2SSupportTicket
	require.NoError(t, DB.First(&stored, "id = ?", ticket.ID).Error)
	assert.Equal(t, D2SSupportOpen, stored.Status)
	assert.Empty(t, stored.AdminReply)

	require.NoError(t, DB.Exec("DROP TRIGGER fail_support_audit").Error)
	updated, err := UpdateD2SSupportTicketWithAudit(ticket.ID, D2SSupportResolved, "Resolved", 7, "ADMIN@example.com", "192.0.2.7")
	require.NoError(t, err)
	assert.Equal(t, D2SSupportResolved, updated.Status)
	assert.Equal(t, "Resolved", updated.AdminReply)
	assert.Equal(t, "admin@example.com", updated.HandledByEmail)
	userTickets, _, err := ListD2SUserSupportTickets(ticket.UserID, 1, 50)
	require.NoError(t, err)
	require.Len(t, userTickets, 1)
	assert.Equal(t, "Resolved", userTickets[0].AdminReply)
	var audit D2SAdminOperationAudit
	require.NoError(t, DB.Where("operation_type = ?", "UPDATE_SUPPORT_TICKET").First(&audit).Error)
	assert.Empty(t, audit.TargetResource)
	assert.Empty(t, audit.DetailsJSON)
	assert.Equal(t, "192.0.2.7", audit.SourceIP)
	assert.Equal(t, "SUCCESS", audit.Result)

	closed, err := UpdateD2SSupportTicketWithAudit(ticket.ID, D2SSupportClosed, "Closed", 7, "admin@example.com", "192.0.2.7")
	require.NoError(t, err)
	assert.NotZero(t, closed.ClosedAt)
	firstClosedAt := closed.ClosedAt
	closedAgain, err := UpdateD2SSupportTicketWithAudit(ticket.ID, D2SSupportClosed, "Still closed", 7, "admin@example.com", "192.0.2.7")
	require.NoError(t, err)
	assert.Equal(t, firstClosedAt, closedAgain.ClosedAt)
	reopened, err := UpdateD2SSupportTicketWithAudit(ticket.ID, D2SSupportInProgress, "Reopened", 7, "admin@example.com", "192.0.2.7")
	require.NoError(t, err)
	assert.Zero(t, reopened.ClosedAt)
}

func TestPruneD2SSupportTicketsRetainsOnlyRecentOrOpenTickets(t *testing.T) {
	useD2STestDB(t)
	require.NoError(t, DB.AutoMigrate(&D2SSupportTicket{}, &D2SAdminOperationAudit{}))

	now := time.Date(2026, time.November, 30, 12, 0, 0, 0, time.UTC)
	cutoff := d2sSupportTicketRetentionCutoff(now)
	older := &D2SSupportTicket{ID: "old-closed", UserID: 1, Category: "OTHER", ContactEmail: "old@example.com", Subject: "Old", Message: "Old ticket content", Status: D2SSupportClosed, ClosedAt: 0, CreatedAt: cutoff - 100, UpdatedAt: cutoff - 1}
	boundary := &D2SSupportTicket{ID: "cutoff-closed", UserID: 2, Category: "OTHER", ContactEmail: "boundary@example.com", Subject: "Boundary", Message: "Boundary ticket content", Status: D2SSupportClosed, ClosedAt: cutoff, CreatedAt: cutoff - 100, UpdatedAt: cutoff}
	recent := &D2SSupportTicket{ID: "recent-closed", UserID: 3, Category: "OTHER", ContactEmail: "recent@example.com", Subject: "Recent", Message: "Recent ticket content", Status: D2SSupportClosed, ClosedAt: cutoff + 1, CreatedAt: cutoff - 100, UpdatedAt: cutoff + 1}
	open := &D2SSupportTicket{ID: "old-open", UserID: 4, Category: "OTHER", ContactEmail: "open@example.com", Subject: "Open", Message: "Open ticket content", Status: D2SSupportOpen, ClosedAt: 0, CreatedAt: cutoff - 100, UpdatedAt: cutoff - 100}
	require.NoError(t, DB.Create([]*D2SSupportTicket{older, boundary, recent, open}).Error)

	deleted, err := PruneD2SSupportTickets(now.Unix())
	require.NoError(t, err)
	assert.EqualValues(t, 2, deleted)
	for _, id := range []string{"old-closed", "cutoff-closed"} {
		var count int64
		require.NoError(t, DB.Model(&D2SSupportTicket{}).Where("id = ?", id).Count(&count).Error)
		assert.Zero(t, count)
	}
	for _, id := range []string{"recent-closed", "old-open"} {
		var count int64
		require.NoError(t, DB.Model(&D2SSupportTicket{}).Where("id = ?", id).Count(&count).Error)
		assert.EqualValues(t, 1, count)
	}
	var audit D2SAdminOperationAudit
	require.NoError(t, DB.Where("operation_type = ?", "CLEANUP_SUPPORT_TICKETS").First(&audit).Error)
	assert.Empty(t, audit.TargetResource)
	assert.Equal(t, "SUCCESS", audit.Result)
	assert.Contains(t, audit.DetailsJSON, "\"record_count\":2")
	for _, privateValue := range []string{"old-closed", "cutoff-closed", "old@example.com", "Boundary ticket content"} {
		assert.NotContains(t, audit.DetailsJSON, privateValue)
	}

	deleted, err = PruneD2SSupportTickets(now.Unix())
	require.NoError(t, err)
	assert.Zero(t, deleted)
	var auditCount int64
	require.NoError(t, DB.Model(&D2SAdminOperationAudit{}).Where("operation_type = ?", "CLEANUP_SUPPORT_TICKETS").Count(&auditCount).Error)
	assert.EqualValues(t, 1, auditCount)
}

func TestPruneD2SSupportTicketsRollsBackAndAuditsFailure(t *testing.T) {
	useD2STestDB(t)
	require.NoError(t, DB.AutoMigrate(&D2SSupportTicket{}, &D2SAdminOperationAudit{}))
	now := time.Date(2026, time.November, 30, 12, 0, 0, 0, time.UTC)
	cutoff := d2sSupportTicketRetentionCutoff(now)
	ticket := &D2SSupportTicket{ID: "cleanup-failure-private-id", UserID: 9, Category: "OTHER", ContactEmail: "private@example.com", Subject: "Private", Message: "Private ticket content", Status: D2SSupportClosed, ClosedAt: cutoff - 1, CreatedAt: cutoff - 10, UpdatedAt: cutoff - 1}
	require.NoError(t, DB.Create(ticket).Error)
	require.NoError(t, DB.Exec("CREATE TRIGGER fail_support_ticket_delete BEFORE DELETE ON d2s_support_tickets BEGIN SELECT RAISE(ABORT, 'forced support ticket delete failure'); END;").Error)

	deleted, err := PruneD2SSupportTickets(now.Unix())
	assert.ErrorIs(t, err, ErrD2SSupportTicketCleanupFailed)
	assert.Zero(t, deleted)
	var remaining D2SSupportTicket
	require.NoError(t, DB.First(&remaining, "id = ?", ticket.ID).Error)
	var audit D2SAdminOperationAudit
	require.NoError(t, DB.Where("operation_type = ? AND result = ?", "CLEANUP_SUPPORT_TICKETS", "FAILURE").First(&audit).Error)
	assert.Empty(t, audit.TargetResource)
	assert.Contains(t, audit.DetailsJSON, "\"candidate_record_count\":1")
	for _, privateValue := range []string{ticket.ID, ticket.ContactEmail, ticket.Message, ticket.Subject} {
		assert.NotContains(t, audit.DetailsJSON, privateValue)
	}
	require.NoError(t, DB.Exec("DROP TRIGGER fail_support_ticket_delete").Error)
}

func TestAnonymizeAndDeleteUserAccountRetainsRequiredRecords(t *testing.T) {
	useD2STestDB(t)
	require.NoError(t, DB.AutoMigrate(
		&Token{}, &TwoFA{}, &TwoFABackupCode{}, &UserSession{}, &AuthFlow{},
		&PasskeyCredential{}, &UserOAuthBinding{}, &ExternalIdentityClaim{},
		&D2SUserSecurityControl{}, &D2SIdentityVerification{},
		&D2SRegistrationRecord{}, &D2SUserActionLog{}, &D2SSupportTicket{}, &D2SAdminOperationAudit{},
	))

	user := createD2STestUser(t, "delete-account-source")
	user.Email = "privacy-request@example.com"
	user.DisplayName = "Private Display"
	user.GitHubId = "private-github-id"
	user.DiscordId = "private-discord-id"
	user.OidcId = "private-oidc-id"
	user.WeChatId = "private-wechat-id"
	user.TelegramId = "private-telegram-id"
	user.LinuxDOId = "private-linuxdo-id"
	user.EmailVerifiedAt = time.Now().UTC().Unix()
	user.StripeCustomer = "private-customer"
	user.Remark = "private account note"
	user.Setting = "private settings"
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]any{
		"email": user.Email, "display_name": user.DisplayName,
		"github_id": user.GitHubId, "discord_id": user.DiscordId,
		"oidc_id": user.OidcId, "wechat_id": user.WeChatId,
		"telegram_id": user.TelegramId, "linux_do_id": user.LinuxDOId,
		"email_verified_at": user.EmailVerifiedAt, "stripe_customer": user.StripeCustomer,
		"remark": user.Remark, "setting": user.Setting,
	}).Error)
	require.NoError(t, DB.Create(&D2SUserProfile{UserID: user.Id, Region: "INTL", EmailVerified: true}).Error)
	require.NoError(t, DB.Create(&D2SUserSecurityControl{UserID: user.Id, Reason: "private risk note", UpdatedAt: time.Now().Unix()}).Error)
	require.NoError(t, DB.Create(&D2SIdentityVerification{UserID: user.Id, IDDocumentNumberHash: strings.Repeat("a", 64), IDDocumentCountry: "US", Status: "PENDING"}).Error)
	require.NoError(t, DB.Create(&D2SRegistrationRecord{
		ID: "retained-registration-snapshot", UserID: user.Id, Email: user.Email,
		RegisterIP: "192.0.2.21", RegisterTime: time.Now().Unix(), EmailVerified: true,
	}).Error)
	userID := user.Id
	require.NoError(t, DB.Create(&D2SUserActionLog{
		ID: "retained-user-action", UserID: &userID, Email: user.Email,
		ActionType: D2SActionLogin, EventTime: time.Now().Unix(),
		SourceIP: "192.0.2.21", ResultStatus: D2SActionResultSuccess,
		RecordHash: strings.Repeat("b", 64),
	}).Error)
	ticket := &D2SSupportTicket{
		UserID: user.Id, Category: "SOFTWARE", ContactEmail: user.Email,
		Subject: "Account deletion question", Message: "Please retain this open support request.",
	}
	require.NoError(t, CreateD2SSupportTicket(ticket))
	preclosedAt := time.Now().UTC().Add(-45 * 24 * time.Hour).Unix()
	preclosedTicket := &D2SSupportTicket{
		ID: "already-closed-ticket", UserID: user.Id, Category: "SOFTWARE", ContactEmail: user.Email,
		Subject: "Previously closed issue", Message: "This ticket was already closed.",
		Status: D2SSupportClosed, ClosedAt: preclosedAt, CreatedAt: preclosedAt - 60, UpdatedAt: preclosedAt,
	}
	require.NoError(t, DB.Create(preclosedTicket).Error)

	require.NoError(t, DB.Create(&Token{UserId: user.Id, Key: "privacy-delete-token"}).Error)
	require.NoError(t, DB.Create(&TwoFA{UserId: user.Id, Secret: "private-secret"}).Error)
	require.NoError(t, DB.Create(&TwoFABackupCode{UserId: user.Id, CodeHash: "private-backup-hash"}).Error)
	require.NoError(t, DB.Create(&PasskeyCredential{UserID: user.Id, CredentialID: "private-credential", PublicKey: "private-public-key"}).Error)
	require.NoError(t, DB.Create(&UserOAuthBinding{UserId: user.Id, ProviderId: 1, ProviderUserId: "private-provider-id"}).Error)
	require.NoError(t, DB.Create(&ExternalIdentityClaim{Provider: ExternalIdentityProviderTelegram, Subject: user.TelegramId, UserId: user.Id}).Error)
	require.NoError(t, DB.Create(&UserSession{
		SID: "privacy-delete-session", UserID: user.Id, Version: 1, UserAuthVersion: 1,
		Status: UserSessionStatusActive, RefreshHash: "private-refresh-hash", LoginMethod: "password",
		LastActiveAt: 1, ExpiresAt: 2,
	}).Error)
	require.NoError(t, DB.Create(&AuthFlow{
		TokenHash: "privacy-delete-auth-flow", Purpose: AuthFlowPurposeTwoFALogin,
		UserId: user.Id, ExpiresAt: time.Now().Add(time.Minute),
	}).Error)

	require.NoError(t, AnonymizeAndDeleteUserAccount(user.Id, D2SActionLogInput{SourceIP: "192.0.2.21", UserAgent: "account-delete-test"}))

	var anonymized User
	require.NoError(t, DB.Unscoped().First(&anonymized, "id = ?", user.Id).Error)
	assert.True(t, anonymized.DeletedAt.Valid)
	assert.Equal(t, common.UserStatusDisabled, anonymized.Status)
	assert.Equal(t, common.RoleCommonUser, anonymized.Role)
	assert.NotEqual(t, user.Email, anonymized.Email)
	assert.Contains(t, anonymized.Email, "@d2s.invalid")
	assert.NotEqual(t, user.Username, anonymized.Username)
	assert.Empty(t, anonymized.DisplayName)
	assert.Empty(t, anonymized.GitHubId)
	assert.Empty(t, anonymized.DiscordId)
	assert.Empty(t, anonymized.OidcId)
	assert.Empty(t, anonymized.WeChatId)
	assert.Empty(t, anonymized.TelegramId)
	assert.Empty(t, anonymized.LinuxDOId)
	assert.Empty(t, anonymized.StripeCustomer)
	assert.Empty(t, anonymized.Remark)
	assert.Empty(t, anonymized.Setting)
	assert.Zero(t, anonymized.EmailVerifiedAt)
	assert.False(t, common.ValidatePasswordAndHash("unused-hash", anonymized.Password))

	var count int64
	for _, record := range []any{
		&Token{}, &TwoFA{}, &TwoFABackupCode{}, &PasskeyCredential{},
		&UserOAuthBinding{}, &ExternalIdentityClaim{}, &UserSession{}, &AuthFlow{},
		&D2SUserProfile{}, &D2SUserSecurityControl{}, &D2SIdentityVerification{},
	} {
		require.NoError(t, DB.Unscoped().Model(record).Where("user_id = ?", user.Id).Count(&count).Error)
		assert.Zero(t, count)
	}
	var registration D2SRegistrationRecord
	require.NoError(t, DB.First(&registration, "id = ?", "retained-registration-snapshot").Error)
	assert.Equal(t, user.Email, registration.Email)
	var action D2SUserActionLog
	require.NoError(t, DB.First(&action, "id = ?", "retained-user-action").Error)
	assert.Equal(t, user.Email, action.Email)
	var deletionAction D2SUserActionLog
	require.NoError(t, DB.Where("user_id = ? AND action_type = ?", user.Id, D2SActionProfileUpdate).First(&deletionAction).Error)
	assert.Equal(t, user.Email, deletionAction.Email)
	assert.Equal(t, D2SActionResultSuccess, deletionAction.ResultStatus)
	assert.Equal(t, "192.0.2.21", deletionAction.SourceIP)
	assert.Equal(t, "account-delete-test", deletionAction.UserAgent)
	assert.Equal(t, "/api/user/self", deletionAction.RequestPath)
	validHash, err := VerifyD2SUserActionLogHash(deletionAction.ID)
	require.NoError(t, err)
	assert.True(t, validHash)
	var retainedTicket D2SSupportTicket
	require.NoError(t, DB.First(&retainedTicket, "id = ?", ticket.ID).Error)
	assert.Equal(t, user.Id, retainedTicket.UserID)
	assert.Equal(t, user.Email, retainedTicket.ContactEmail)
	assert.Equal(t, D2SSupportClosed, retainedTicket.Status)
	assert.Greater(t, retainedTicket.ClosedAt, int64(0))
	var closedTicket D2SSupportTicket
	require.NoError(t, DB.First(&closedTicket, "id = ?", preclosedTicket.ID).Error)
	assert.Equal(t, preclosedAt, closedTicket.ClosedAt)

	beforeExpiry := time.Unix(retainedTicket.ClosedAt, 0).AddDate(0, D2SSupportTicketRetentionMonths-1, 0).Unix()
	deletedCount, err := PruneD2SSupportTickets(beforeExpiry)
	require.NoError(t, err)
	assert.EqualValues(t, 1, deletedCount)
	require.NoError(t, DB.First(&retainedTicket, "id = ?", ticket.ID).Error)
	afterExpiry := time.Unix(retainedTicket.ClosedAt, 0).AddDate(0, D2SSupportTicketRetentionMonths+1, 0).Unix()
	deletedCount, err = PruneD2SSupportTickets(afterExpiry)
	require.NoError(t, err)
	assert.EqualValues(t, 1, deletedCount)
	var deletedTicketCount int64
	require.NoError(t, DB.Model(&D2SSupportTicket{}).Where("id = ?", ticket.ID).Count(&deletedTicketCount).Error)
	assert.Zero(t, deletedTicketCount)

	reRegistered := User{Username: "delete-account-reuse", Password: "new-password", Email: user.Email}
	require.NoError(t, DB.Create(&reRegistered).Error)
}

func TestAnonymizeAndDeleteUserAccountRollsBackIfSupportClosureFails(t *testing.T) {
	useD2STestDB(t)
	require.NoError(t, DB.AutoMigrate(
		&Token{}, &TwoFA{}, &TwoFABackupCode{}, &UserSession{}, &AuthFlow{},
		&PasskeyCredential{}, &UserOAuthBinding{}, &ExternalIdentityClaim{},
		&D2SUserSecurityControl{}, &D2SIdentityVerification{}, &D2SUserActionLog{}, &D2SSupportTicket{},
	))

	user := createD2STestUser(t, "delete-account-rollback")
	ticket := &D2SSupportTicket{
		UserID: user.Id, Category: "ACCOUNT", ContactEmail: user.Email,
		Subject: "Rollback test", Message: "Keep account and ticket consistent if closing fails.",
	}
	require.NoError(t, CreateD2SSupportTicket(ticket))
	require.NoError(t, DB.Exec("CREATE TRIGGER fail_support_ticket_close BEFORE UPDATE ON d2s_support_tickets BEGIN SELECT RAISE(ABORT, 'forced support ticket close failure'); END;").Error)

	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedisEnabled })
	err := AnonymizeAndDeleteUserAccount(user.Id, D2SActionLogInput{SourceIP: "192.0.2.22", UserAgent: "account-delete-rollback-test"})
	require.Error(t, err)

	var unchanged User
	require.NoError(t, DB.First(&unchanged, "id = ?", user.Id).Error)
	assert.Equal(t, user.Email, unchanged.Email)
	assert.False(t, unchanged.DeletedAt.Valid)
	var retainedTicket D2SSupportTicket
	require.NoError(t, DB.First(&retainedTicket, "id = ?", ticket.ID).Error)
	assert.Equal(t, D2SSupportOpen, retainedTicket.Status)
	assert.Zero(t, retainedTicket.ClosedAt)
	var actionCount int64
	require.NoError(t, DB.Model(&D2SUserActionLog{}).Where("user_id = ? AND action_type = ?", user.Id, D2SActionProfileUpdate).Count(&actionCount).Error)
	assert.Zero(t, actionCount)
}
