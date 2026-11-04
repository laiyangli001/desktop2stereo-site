package model

import (
	"errors"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const D2SPurchasesEnabledOption = "D2SPurchasesEnabled"

// A saved setting overrides the legacy environment default on every node.
func D2SPurchasesEnabled() (bool, error) {
	if DB == nil {
		return false, errors.New("database is unavailable")
	}
	var option Option
	err := DB.Where(&Option{Key: D2SPurchasesEnabledOption}).First(&option).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return common.GetEnvOrDefaultBool("D2S_PURCHASES_ENABLED", false), nil
	}
	if err != nil {
		return false, err
	}
	if option.Value != "true" && option.Value != "false" {
		return false, errors.New("invalid purchase setting")
	}
	return option.Value == "true", nil
}

func SetD2SPurchasesEnabled(enabled bool, audit *D2SAdminOperationAudit) error {
	if DB == nil {
		return errors.New("database is unavailable")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		option := Option{Key: D2SPurchasesEnabledOption, Value: strconv.FormatBool(enabled)}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "key"}},
			DoUpdates: clause.AssignmentColumns([]string{"value"}),
		}).Create(&option).Error; err != nil {
			return err
		}
		return RecordD2SAdminOperationAuditTx(tx, audit)
	})
}
