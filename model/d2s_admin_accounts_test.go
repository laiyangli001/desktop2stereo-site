package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRecordD2SAdminLoginFailureUsesWindowAndResetsAfterLock(t *testing.T) {
	useD2STestDB(t)
	require.NoError(t, DB.AutoMigrate(&D2SAdminAccount{}))
	account := D2SAdminAccount{
		Email: "admin@example.com", PasswordHash: "hash",
		Role: D2SAdminRoleSuperAdmin, Status: D2SAdminStatusActive,
		CreatedAt: 1, UpdatedAt: 1,
	}
	require.NoError(t, DB.Create(&account).Error)

	first, locked, err := RecordD2SAdminLoginFailure(account.Email, 1000, 3, 900, 900)
	require.NoError(t, err)
	require.False(t, locked)
	require.Equal(t, 1, first.FailedAttempts)
	require.EqualValues(t, 1000, first.FailureWindowStartedAt)

	second, locked, err := RecordD2SAdminLoginFailure(account.Email, 1800, 3, 900, 900)
	require.NoError(t, err)
	require.False(t, locked)
	require.Equal(t, 2, second.FailedAttempts)

	windowReset, locked, err := RecordD2SAdminLoginFailure(account.Email, 1900, 3, 900, 900)
	require.NoError(t, err)
	require.False(t, locked)
	require.Equal(t, 1, windowReset.FailedAttempts)
	require.EqualValues(t, 1900, windowReset.FailureWindowStartedAt)

	_, locked, err = RecordD2SAdminLoginFailure(account.Email, 1901, 3, 900, 900)
	require.NoError(t, err)
	require.False(t, locked)
	lockedAccount, locked, err := RecordD2SAdminLoginFailure(account.Email, 1902, 3, 900, 900)
	require.NoError(t, err)
	require.True(t, locked)
	require.EqualValues(t, 2802, lockedAccount.LockedUntil)

	duringLock, locked, err := RecordD2SAdminLoginFailure(account.Email, 2000, 3, 900, 900)
	require.NoError(t, err)
	require.True(t, locked)
	require.Equal(t, 3, duringLock.FailedAttempts)

	afterLock, locked, err := RecordD2SAdminLoginFailure(account.Email, 2802, 3, 900, 900)
	require.NoError(t, err)
	require.False(t, locked)
	require.Equal(t, 1, afterLock.FailedAttempts)
	require.EqualValues(t, 2802, afterLock.FailureWindowStartedAt)
	require.Zero(t, afterLock.LockedUntil)

	require.NoError(t, RecordD2SAdminLoginSuccess(account.ID, 2803))
	var reloaded D2SAdminAccount
	require.NoError(t, DB.First(&reloaded, account.ID).Error)
	require.Zero(t, reloaded.FailedAttempts)
	require.Zero(t, reloaded.FailureWindowStartedAt)
	require.Zero(t, reloaded.LockedUntil)
}

func TestCreateD2SAdminAccountRequiresSuperAdminAndAuditsAtomically(t *testing.T) {
	useD2STestDB(t)
	require.NoError(t, DB.AutoMigrate(&D2SAdminAccount{}, &D2SAdminOperationAudit{}))
	operator, err := CreateInitialD2SAdminAccount("owner@example.com", "existing-hash", 1000)
	require.NoError(t, err)

	created, err := CreateD2SAdminAccountWithAudit(
		"reviewer@example.com", "new-hash", D2SAdminRoleSuperAdmin,
		operator.ID, operator.Email, 1100,
	)
	require.NoError(t, err)
	require.Equal(t, D2SAdminRoleSuperAdmin, created.Role)
	var audit D2SAdminOperationAudit
	require.NoError(t, DB.Where("operation_type = ? AND target_resource = ?", "CREATE_ADMIN_ACCOUNT", created.Email).First(&audit).Error)
	require.Equal(t, operator.ID, audit.OperatorID)
	require.Equal(t, "SUCCESS", audit.Result)

	reader := D2SAdminAccount{
		Email: "reader@example.com", PasswordHash: "reader-hash",
		Role: D2SAdminRoleAuditReader, Status: D2SAdminStatusActive,
		CreatedAt: 1000, UpdatedAt: 1000,
	}
	require.NoError(t, DB.Create(&reader).Error)
	_, err = CreateD2SAdminAccountWithAudit(
		"not-created@example.com", "new-hash", D2SAdminRoleAuditReader,
		reader.ID, reader.Email, 1200,
	)
	require.ErrorIs(t, err, ErrD2SAdminForbidden)
	var count int64
	require.NoError(t, DB.Model(&D2SAdminAccount{}).Where("email = ?", "not-created@example.com").Count(&count).Error)
	require.Zero(t, count)
}
