package service

import (
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const d2sArtifactCleanupInterval = time.Hour

// StartD2SArtifactCleanup removes expired, non-authoritative runtime artifacts.
// Only the master node runs the periodic sweep in a multi-node deployment.
func StartD2SArtifactCleanup() {
	if !common.IsMasterNode {
		return
	}
	go func() {
		cleanupD2SArtifacts()
		ticker := time.NewTicker(d2sArtifactCleanupInterval)
		defer ticker.Stop()
		for range ticker.C {
			cleanupD2SArtifacts()
		}
	}()
}

func StartD2SSupportTicketRetentionCleanup() {
	if !common.IsMasterNode {
		return
	}
	go func() {
		cleanupD2SSupportTickets()
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			cleanupD2SSupportTickets()
		}
	}()
}

func cleanupD2SSupportTickets() {
	deleted, err := model.PruneD2SSupportTickets(time.Now().UTC().Unix())
	if err != nil {
		common.SysError("failed to apply Desktop2Stereo support ticket retention: " + err.Error())
		return
	}
	if deleted > 0 {
		common.SysLog("removed expired Desktop2Stereo support tickets: " + strconv.FormatInt(deleted, 10))
	}
}

func cleanupD2SArtifacts() {
	if err := model.ExpireD2SPendingOrders(time.Now().Unix()); err != nil {
		common.SysError("failed to expire Desktop2Stereo pending orders: " + err.Error())
	}
	if err := model.DeleteExpiredD2SRuntimeArtifacts(time.Now().Unix()); err != nil {
		common.SysError("failed to delete expired Desktop2Stereo device codes and online leases: " + err.Error())
	}
}
