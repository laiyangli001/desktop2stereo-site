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
