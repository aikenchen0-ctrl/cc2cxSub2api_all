package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/Wei-Shaw/agentapi/payment"
	payprovider "github.com/Wei-Shaw/agentapi/payment/provider"
)

type stationRefund struct {
	ProviderKey  string
	ConfigCipher string
	TradeNo      string
	RefundID     string
	CreditCents  int64
	State        string
	UpdatedAt    int64
}

func (s *Store) stationRefund(agentID string, id int64) (stationRefund, error) {
	var refund stationRefund
	err := s.db.QueryRow(`SELECT provider_key, config_cipher, trade_no, refund_id, credit_cents, state, updated_at FROM station_payment_refunds WHERE agent_id=? AND order_id=?`, agentID, id).Scan(&refund.ProviderKey, &refund.ConfigCipher, &refund.TradeNo, &refund.RefundID, &refund.CreditCents, &refund.State, &refund.UpdatedAt)
	return refund, err
}

// Reserve the credited wallet funds and record the attempt before contacting a
// provider. An ambiguous network failure must never trigger another refund.
func (s *Store) beginStationRefund(agentID string, order StationPaymentOrder, provider StationPaymentProvider) error {
	cipher, err := s.encryptConfig(provider.Config)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var tradeNo string
	if err := tx.QueryRow(`SELECT provider_trade_no FROM station_payment_orders WHERE agent_id=? AND id=?`, agentID, order.ID).Scan(&tradeNo); err != nil {
		return err
	}
	credit := int64(math.Round(order.Amount * 100))
	var credited int64
	if err := tx.QueryRow(`SELECT COALESCE(SUM(amount_cents),0) FROM wallet_ledger WHERE agent_id=? AND main_user_id=? AND kind='user_recharge' AND request_id=?`, agentID, order.MainUserID, "recharge:"+order.OutTradeNo).Scan(&credited); err != nil {
		return err
	}
	if credit <= 0 || credited != credit {
		return fmt.Errorf("order wallet credit has not been fulfilled")
	}
	now := s.clock().UTC().Unix()
	result, err := tx.Exec(`UPDATE station_payment_orders SET status='REFUNDING', updated_at=? WHERE agent_id=? AND id=? AND status IN ('COMPLETED','REFUND_REQUESTED')`, now, agentID, order.ID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errRechargeStateConflict
	}
	result, err = tx.Exec(`UPDATE agent_user_wallets SET balance_cents=balance_cents-?, version=version+1, updated_at=? WHERE agent_id=? AND main_user_id=? AND balance_cents>=?`, credit, now, agentID, order.MainUserID, credit)
	if err != nil {
		return err
	}
	rows, err = result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errInsufficientBalance
	}
	_, err = tx.Exec(`INSERT INTO station_payment_refunds(agent_id,order_id,provider_key,config_cipher,trade_no,credit_cents,state,updated_at) VALUES (?,?,?,?,?,?,'REFUNDING',?)`, agentID, order.ID, provider.ProviderKey, cipher, tradeNo, credit, now)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO wallet_ledger(agent_id,main_user_id,kind,amount_cents,balance_after_cents,request_id,order_id,note,created_at) SELECT ?,?,'payment_refund',?,balance_cents,?,?,?,? FROM agent_user_wallets WHERE agent_id=? AND main_user_id=?`, agentID, order.MainUserID, -credit, "payment-refund:"+order.OutTradeNo, order.OutTradeNo, "payment refund reserved", now, agentID, order.MainUserID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) finishStationRefund(agentID string, order StationPaymentOrder, result *payment.RefundResponse) error {
	state := "REFUND_PENDING"
	if result != nil {
		switch result.Status {
		case payment.ProviderStatusSuccess, payment.ProviderStatusRefunded:
			state = "REFUNDED"
		case payment.ProviderStatusFailed:
			state = "REFUND_FAILED"
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var current string
	var credit int64
	if err := tx.QueryRow(`SELECT state, credit_cents FROM station_payment_refunds WHERE agent_id=? AND order_id=?`, agentID, order.ID).Scan(&current, &credit); err != nil {
		return err
	}
	if current == "REFUNDED" || current == "REFUND_FAILED" {
		return tx.Commit()
	}
	now := s.clock().UTC().Unix()
	if state == "REFUND_FAILED" {
		if _, err := tx.Exec(`UPDATE agent_user_wallets SET balance_cents=balance_cents+?,version=version+1,updated_at=? WHERE agent_id=? AND main_user_id=?`, credit, now, agentID, order.MainUserID); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO wallet_ledger(agent_id,main_user_id,kind,amount_cents,balance_after_cents,request_id,order_id,note,created_at) SELECT ?,?,'payment_refund_reversal',?,balance_cents,?,?,?,? FROM agent_user_wallets WHERE agent_id=? AND main_user_id=?`, agentID, order.MainUserID, credit, "payment-refund-reversal:"+order.OutTradeNo, order.OutTradeNo, "provider rejected refund", now, agentID, order.MainUserID); err != nil {
			return err
		}
	}
	refundID := ""
	if result != nil {
		refundID = result.RefundID
	}
	if _, err := tx.Exec(`UPDATE station_payment_refunds SET state=?,refund_id=CASE WHEN ?<>'' THEN ? ELSE refund_id END,updated_at=? WHERE agent_id=? AND order_id=?`, state, refundID, refundID, now, agentID, order.ID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE station_payment_orders SET status=?,updated_at=? WHERE agent_id=? AND id=?`, state, now, agentID, order.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Server) refundStationOrder(w http.ResponseWriter, r *http.Request, requestID string, tenant TenantContext, order StationPaymentOrder) {
	if order.Status == "REFUNDED" {
		s.writeData(w, http.StatusOK, requestID, map[string]string{"message": "order refunded", "status": "REFUNDED"})
		return
	}
	isNew := order.Status == "COMPLETED" || order.Status == "REFUND_REQUESTED"
	if !isNew && order.Status != "REFUNDING" && order.Status != "REFUND_PENDING" {
		s.writeError(w, http.StatusConflict, requestID, "ORDER_STATE_CONFLICT", "order is not refundable")
		return
	}
	if isNew {
		id, _ := strconv.ParseInt(order.ProviderInstanceID, 10, 64)
		provider, err := s.store.StationProvider(tenant.AgentID, id, true)
		if err != nil || !provider.RefundEnabled {
			s.writeError(w, http.StatusForbidden, requestID, "REFUND_NOT_ALLOWED", "provider refunds are disabled")
			return
		}
		provider.Config["notifyUrl"] = requestOrigin(r) + stationWebhookPath(tenant.AgentID, provider.ProviderKey)
		provider.Config["returnUrl"] = requestOrigin(r) + "/payment/result"
		if _, err := payprovider.CreateProvider(provider.ProviderKey, order.ProviderInstanceID, provider.Config); err != nil {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_PROVIDER", "provider refund configuration is invalid")
			return
		}
		if err := s.store.beginStationRefund(tenant.AgentID, order, provider); err != nil {
			code := "REFUND_STATE_CONFLICT"
			if errors.Is(err, errInsufficientBalance) {
				code = "INSUFFICIENT_BALANCE"
			}
			s.writeError(w, http.StatusConflict, requestID, code, "refund cannot reserve the credited wallet funds")
			return
		}
	}
	refund, err := s.store.stationRefund(tenant.AgentID, order.ID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load refund attempt")
		return
	}
	if !isNew && refund.State == "REFUNDING" && s.store.clock().Sub(time.Unix(refund.UpdatedAt, 0)) < 5*time.Minute {
		s.writeData(w, http.StatusAccepted, requestID, map[string]string{"message": "refund request is in progress", "status": "REFUNDING"})
		return
	}
	configJSON, err := s.store.decrypt(refund.ConfigCipher)
	config := map[string]string{}
	if err != nil || json.Unmarshal([]byte(configJSON), &config) != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load refund configuration")
		return
	}
	provider, err := payprovider.CreateProvider(refund.ProviderKey, order.ProviderInstanceID, config)
	if err != nil {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "INVALID_PROVIDER", "refund provider is unavailable")
		return
	}
	var result *payment.RefundResponse
	amount := fmt.Sprintf("%.2f", order.PayAmount)
	if isNew {
		result, err = provider.Refund(r.Context(), payment.RefundRequest{TradeNo: refund.TradeNo, OrderID: order.OutTradeNo, Amount: amount, Reason: "station admin refund"})
	} else if query, ok := provider.(payment.RefundQueryProvider); ok {
		result, err = query.QueryRefund(r.Context(), payment.RefundQueryRequest{TradeNo: refund.TradeNo, OrderID: order.OutTradeNo, RefundID: refund.RefundID, Amount: amount})
	} else {
		s.writeData(w, http.StatusAccepted, requestID, map[string]string{"message": "refund is awaiting provider confirmation", "status": order.Status})
		return
	}
	if err != nil {
		result = nil
	}
	if saveErr := s.store.finishStationRefund(tenant.AgentID, order, result); saveErr != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to persist refund result")
		return
	}
	updated, _ := s.store.StationOrder(tenant.AgentID, order.ID)
	s.recordTenantAudit(tenant.AgentID, "agent_admin", tenant.MainUserID, "billing.order.refund", "station_order", strconv.FormatInt(order.ID, 10), requestID, updated.Status, "")
	code := http.StatusOK
	if updated.Status == "REFUND_PENDING" {
		code = http.StatusAccepted
	}
	s.writeData(w, code, requestID, map[string]string{"message": "refund processed", "status": updated.Status})
}
