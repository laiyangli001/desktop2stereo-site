package model

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestD2SExportApprovalRequiresDifferentReviewerAndIsSingleUse(t *testing.T) {
	useD2STestDB(t)
	require.NoError(t, DB.AutoMigrate(&D2SAdminExportApproval{}, &D2SAdminOperationAudit{}))
	approval := &D2SAdminExportApproval{
		ExportType: D2SExportApprovalTypeUserActionLogs, FilterHash: strings.Repeat("a", 64),
		FilterSnapshot: `{"start_time":100,"end_time":200}`, RequestReason: "Regulatory request R-123",
		RequestedByID: 7, RequestedByEmail: "auditor@example.com", RequestedAt: 1000, ExpiresAt: 2800,
	}
	requestAudit := &D2SAdminOperationAudit{
		OperatorID: 7, OperatorEmail: "auditor@example.com", OperationType: "REQUEST_EXPORT_APPROVAL",
		TargetResource: "USER_ACTION_LOGS", Result: "SUCCESS",
	}
	require.NoError(t, CreateD2SAdminExportApprovalWithAudit(approval, requestAudit))

	selfApprovalAudit := &D2SAdminOperationAudit{
		OperatorID: 7, OperatorEmail: "auditor@example.com", OperationType: "APPROVE_EXPORT",
		TargetResource: approval.ID, Result: "SUCCESS",
	}
	_, err := DecideD2SAdminExportApprovalWithAudit(approval.ID, 7, "auditor@example.com", D2SExportApprovalApproved, "reviewed", 1100, selfApprovalAudit)
	require.ErrorIs(t, err, ErrD2SExportApprovalSelfApproval)

	reviewAudit := &D2SAdminOperationAudit{
		OperatorID: 8, OperatorEmail: "security@example.com", OperationType: "APPROVE_EXPORT",
		TargetResource: approval.ID, Result: "SUCCESS",
	}
	updated, err := DecideD2SAdminExportApprovalWithAudit(approval.ID, 8, "security@example.com", D2SExportApprovalApproved, "Approved request R-123", 1200, reviewAudit)
	require.NoError(t, err)
	require.Equal(t, D2SExportApprovalApproved, updated.Status)
	require.Equal(t, 8, updated.ReviewedByID)

	useAudit := &D2SAdminOperationAudit{
		OperatorID: 7, OperatorEmail: "auditor@example.com", OperationType: "USE_EXPORT_APPROVAL",
		TargetResource: approval.ID, Result: "SUCCESS",
	}
	err = ConsumeD2SAdminExportApprovalWithAudit(approval.ID, D2SExportApprovalTypeUserActionLogs, strings.Repeat("b", 64), 7, 1300, useAudit)
	require.ErrorIs(t, err, ErrD2SExportApprovalFilterMismatch)
	require.NoError(t, ConsumeD2SAdminExportApprovalWithAudit(approval.ID, D2SExportApprovalTypeUserActionLogs, strings.Repeat("a", 64), 7, 1300, useAudit))
	require.ErrorIs(t, ConsumeD2SAdminExportApprovalWithAudit(approval.ID, D2SExportApprovalTypeUserActionLogs, strings.Repeat("a", 64), 7, 1301, useAudit), ErrD2SExportApprovalNotApproved)

	var persisted D2SAdminExportApproval
	require.NoError(t, DB.Where("id = ?", approval.ID).First(&persisted).Error)
	require.Equal(t, D2SExportApprovalConsumed, persisted.Status)
	require.Equal(t, 7, persisted.ConsumedByID)
	var auditCount int64
	require.NoError(t, DB.Model(&D2SAdminOperationAudit{}).Count(&auditCount).Error)
	require.EqualValues(t, 3, auditCount)
}

func TestD2SExportApprovalRejectsExpiredAndPendingConsumption(t *testing.T) {
	useD2STestDB(t)
	require.NoError(t, DB.AutoMigrate(&D2SAdminExportApproval{}, &D2SAdminOperationAudit{}))
	approval := &D2SAdminExportApproval{
		ExportType: D2SExportApprovalTypeRegistrations, FilterHash: strings.Repeat("c", 64),
		FilterSnapshot: `{}`, RequestReason: "Compliance review", RequestedByID: 9,
		RequestedByEmail: "reader@example.com", RequestedAt: 100, ExpiresAt: 200,
	}
	audit := &D2SAdminOperationAudit{OperatorID: 9, OperatorEmail: "reader@example.com", OperationType: "REQUEST_EXPORT_APPROVAL", Result: "SUCCESS"}
	require.NoError(t, CreateD2SAdminExportApprovalWithAudit(approval, audit))
	useAudit := &D2SAdminOperationAudit{OperatorID: 9, OperatorEmail: "reader@example.com", OperationType: "USE_EXPORT_APPROVAL", Result: "SUCCESS"}
	require.ErrorIs(t, ConsumeD2SAdminExportApprovalWithAudit(approval.ID, D2SExportApprovalTypeRegistrations, strings.Repeat("c", 64), 9, 150, useAudit), ErrD2SExportApprovalNotApproved)
	require.ErrorIs(t, ConsumeD2SAdminExportApprovalWithAudit(approval.ID, D2SExportApprovalTypeRegistrations, strings.Repeat("c", 64), 9, 200, useAudit), ErrD2SExportApprovalExpired)
	_, err := DecideD2SAdminExportApprovalWithAudit(approval.ID, 10, "security@example.com", D2SExportApprovalApproved, "late", 200, &D2SAdminOperationAudit{
		OperatorID: 10, OperatorEmail: "security@example.com", OperationType: "APPROVE_EXPORT", Result: "SUCCESS",
	})
	require.True(t, errors.Is(err, ErrD2SExportApprovalExpired))
}
