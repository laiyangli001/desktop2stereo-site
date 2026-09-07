package model

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

type D2SReconciliationMismatch struct {
	OrderID         string `json:"order_id,omitempty"`
	ProviderEventID string `json:"provider_event_id,omitempty"`
	Kind            string `json:"kind"`
}

type D2SReconciliationReport struct {
	StartAt          int64                       `json:"start_at"`
	EndAt            int64                       `json:"end_at"`
	Orders           int                         `json:"orders"`
	PaymentEvents    int                         `json:"payment_events"`
	PaidOrders       int                         `json:"paid_orders"`
	ChargebackOrders int                         `json:"chargeback_orders"`
	PendingExpired   int                         `json:"pending_expired"`
	Mismatches       []D2SReconciliationMismatch `json:"mismatches"`
}

// ReconcileD2SPayments checks the durable order/event ledger for a UTC window.
// Provider settlement files remain the external source of truth; this report
// identifies local records that require an operator comparison or replay.
func ReconcileD2SPayments(startAt, endAt int64) (*D2SReconciliationReport, error) {
	if startAt <= 0 || endAt <= startAt {
		return nil, errors.New("invalid reconciliation window")
	}
	var orders []D2SOrder
	if err := DB.Where("created_at >= ? AND created_at < ?", startAt, endAt).Find(&orders).Error; err != nil {
		return nil, err
	}
	report := &D2SReconciliationReport{StartAt: startAt, EndAt: endAt, Orders: len(orders), Mismatches: make([]D2SReconciliationMismatch, 0)}
	orderByID := make(map[string]D2SOrder, len(orders))
	createdInWindow := make(map[string]bool, len(orders))
	for _, order := range orders {
		orderByID[order.ID] = order
		createdInWindow[order.ID] = true
		switch order.Status {
		case D2SOrderPaid:
			report.PaidOrders++
		case D2SOrderChargeback:
			report.ChargebackOrders++
		}
		if order.Status == D2SOrderPending && order.ExpiresAt > 0 && order.ExpiresAt < endAt {
			report.PendingExpired++
			report.Mismatches = append(report.Mismatches, D2SReconciliationMismatch{OrderID: order.ID, Kind: "pending_expired"})
		}
	}

	var events []D2SPaymentEvent
	if err := DB.Where("processed_at >= ? AND processed_at < ?", startAt, endAt).Find(&events).Error; err != nil {
		return nil, err
	}
	report.PaymentEvents = len(events)
	paidExternalEvents := make(map[string]bool)
	for _, event := range events {
		order, ok := orderByID[event.OrderID]
		if !ok {
			var found D2SOrder
			if err := DB.Where("id = ?", event.OrderID).First(&found).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					report.Mismatches = append(report.Mismatches, D2SReconciliationMismatch{OrderID: event.OrderID, ProviderEventID: event.ProviderEventID, Kind: "orphan_payment_event"})
					continue
				}
				return nil, err
			}
			order = found
			orderByID[order.ID] = order
			report.Orders++
			switch order.Status {
			case D2SOrderPaid:
				report.PaidOrders++
			case D2SOrderChargeback:
				report.ChargebackOrders++
			}
		}
		if event.AmountMinor != order.GatewayMinor || event.Currency != order.Currency || event.Provider != order.Provider {
			report.Mismatches = append(report.Mismatches, D2SReconciliationMismatch{OrderID: order.ID, ProviderEventID: event.ProviderEventID, Kind: "payment_order_mismatch"})
		}
		switch event.EventType {
		case "paid":
			if order.Status != D2SOrderPaid && order.Status != D2SOrderChargeback {
				report.Mismatches = append(report.Mismatches, D2SReconciliationMismatch{OrderID: order.ID, ProviderEventID: event.ProviderEventID, Kind: "paid_order_not_settled"})
			}
		case "canceled", "failed":
			if order.Status != D2SOrderCanceled {
				report.Mismatches = append(report.Mismatches, D2SReconciliationMismatch{OrderID: order.ID, ProviderEventID: event.ProviderEventID, Kind: "cancel_event_not_settled"})
			}
		case "chargeback", "reversed":
			if order.Status != D2SOrderChargeback {
				report.Mismatches = append(report.Mismatches, D2SReconciliationMismatch{OrderID: order.ID, ProviderEventID: event.ProviderEventID, Kind: "reversal_not_settled"})
			}
		}
		if event.EventType == "paid" && order.GatewayMinor > 0 {
			paidExternalEvents[order.ID] = true
		}
	}
	for _, order := range orders {
		if createdInWindow[order.ID] && order.Status == D2SOrderPaid && order.GatewayMinor > 0 && !paidExternalEvents[order.ID] {
			report.Mismatches = append(report.Mismatches, D2SReconciliationMismatch{OrderID: order.ID, Kind: "paid_order_without_payment_event"})
		}
	}
	return report, nil
}

func D2SPreviousUTCWindow(now time.Time) (int64, int64) {
	utc := now.UTC()
	end := time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
	return end.Add(-24 * time.Hour).Unix(), end.Unix()
}
