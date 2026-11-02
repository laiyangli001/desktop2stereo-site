package model

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestD2SSupportTicketCreationAndOwnerIsolation(t *testing.T) {
	useD2STestDB(t)
	require.NoError(t, DB.AutoMigrate(&D2SSupportTicket{}))

	first := &D2SSupportTicket{
		UserID: 11, Category: "PAYMENT", ContactEmail: " USER@EXAMPLE.COM ",
		Subject: "Payment not reflected", Message: "My payment succeeded but the account does not show the order.",
		SourceIP: "192.0.2.11", UserAgent: "test-agent",
	}
	second := &D2SSupportTicket{
		UserID: 12, Category: "SOFTWARE", ContactEmail: "other@example.com",
		Subject: "Application help", Message: "The application closes when I open a project.",
	}
	require.NoError(t, CreateD2SSupportTicket(first))
	require.NoError(t, CreateD2SSupportTicket(second))
	assert.NotEmpty(t, first.ID)
	assert.Equal(t, "user@example.com", first.ContactEmail)
	assert.Equal(t, D2SSupportOpen, first.Status)
	assert.NotZero(t, first.CreatedAt)

	items, total, err := ListD2SUserSupportTickets(11, 1, 50)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, first.ID, items[0].ID)
	encoded, err := json.Marshal(items[0])
	require.NoError(t, err)
	for _, privateField := range []string{"contact_email", "source_ip", "user_agent", "handled_by_email"} {
		assert.NotContains(t, string(encoded), privateField)
	}

	items, total, err = ListD2SUserSupportTickets(12, 1, 50)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, second.ID, items[0].ID)

	_, total, err = ListD2SUserSupportTickets(11, 0, 101)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	assert.Error(t, func() error {
		_, _, err := ListD2SUserSupportTickets(0, 1, 50)
		return err
	}())
}

func TestD2SSupportTicketValidationAndAuditTransaction(t *testing.T) {
	useD2STestDB(t)
	require.NoError(t, DB.AutoMigrate(&D2SSupportTicket{}, &D2SAdminOperationAudit{}))

	for _, ticket := range []*D2SSupportTicket{
		nil,
		{UserID: 0, ContactEmail: "user@example.com", Subject: "Help", Message: "More than ten characters"},
		{UserID: 1, ContactEmail: "user@example.com", Subject: " ", Message: "More than ten characters"},
		{UserID: 1, ContactEmail: "user@example.com", Subject: "Help", Message: "short"},
	} {
		assert.Error(t, CreateD2SSupportTicket(ticket))
	}

	ticket := &D2SSupportTicket{
		UserID: 11, Category: "PAYMENT", ContactEmail: "user@example.com",
		Subject: "Payment not reflected", Message: "My payment succeeded but the account does not show the order.",
	}
	require.NoError(t, CreateD2SSupportTicket(ticket))
	assert.False(t, ValidD2SSupportReplyLength(strings.Repeat("x", 5001)))
	assert.Error(t, func() error {
		_, err := UpdateD2SSupportTicketWithAudit(ticket.ID, "UNKNOWN", "reply", 7, "admin@example.com", "192.0.2.7", "{}")
		return err
	}())

	require.NoError(t, DB.Exec("CREATE TRIGGER fail_support_audit BEFORE INSERT ON d2s_admin_operation_audits BEGIN SELECT RAISE(ABORT, 'forced support audit failure'); END;").Error)
	_, err := UpdateD2SSupportTicketWithAudit(ticket.ID, D2SSupportResolved, "Resolved", 7, "admin@example.com", "192.0.2.7", "{}")
	require.Error(t, err)
	var stored D2SSupportTicket
	require.NoError(t, DB.First(&stored, "id = ?", ticket.ID).Error)
	assert.Equal(t, D2SSupportOpen, stored.Status)
	assert.Empty(t, stored.AdminReply)

	require.NoError(t, DB.Exec("DROP TRIGGER fail_support_audit").Error)
	updated, err := UpdateD2SSupportTicketWithAudit(ticket.ID, D2SSupportResolved, "Resolved", 7, "ADMIN@example.com", "192.0.2.7", `{"status":"RESOLVED"}`)
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
	assert.Equal(t, "d2s_support_tickets:"+ticket.ID, audit.TargetResource)
	assert.Equal(t, "192.0.2.7", audit.SourceIP)
	assert.Equal(t, "SUCCESS", audit.Result)
}
