package service

import (
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

func cleanupD2SArtifacts() {
	if err := model.DeleteExpiredD2SRuntimeArtifacts(time.Now().Unix()); err != nil {
		common.SysError("failed to delete expired Desktop2Stereo device codes and online leases: " + err.Error())
	}
}
