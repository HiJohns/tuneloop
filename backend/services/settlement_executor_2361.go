package services

// #2361（#2268 子任务）：SettlementExecutor 执行器。
//
// 职责边界（v2.2 §9/§13）：
//   - 锁：订单行 SELECT … FOR UPDATE 串行化同一订单的最终结算；
//   - 计算：只调用 SettlementCalculator（#2359），本文件不做任何金额推导；
//   - 落库：#2360 的 settlement_calculations（版本/状态/分账户）+ settlement_action_ledger；
//   - 执行：每个子动作预生成 idempotency_key（settlement_id + action_type +
//     source_transaction_id），已成功的动作绝不重放；
//   - 部分成功：按子动作重试，状态 partially_succeeded；禁止整单重放；
//   - 冲正/差额：已执行版本不覆盖，旧行 status→superseded（金额不变），
//     新行 calculated + supersedes=旧 id（唯一索引不冲突）。
//
// 计算/执行分离：所有金额来自 *SettlementResult（rent_refund / deposit_refund /
// rent_shortfall / other_fee_shortfall / gift_points_refunded / net_cash），
// 本文件只做「结果字段 → 子动作」的映射与合计校验。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"tuneloop-backend/models"
)

// 结算行状态机（#2360/#2361）。
const (
	SettlementStatusCalculated         = "calculated"
	SettlementStatusConfirmed          = "confirmed"
	SettlementStatusProcessing         = "processing"
	SettlementStatusSucceeded          = "succeeded"
	SettlementStatusPartiallySucceeded = "partially_succeeded"
	SettlementStatusFailedRetryable    = "failed_retryable"
	SettlementStatusSuperseded         = "superseded"
)

// 子动作状态。
const (
	SettlementActionPending   = "pending"
	SettlementActionExecuting = "executing"
	SettlementActionSucceeded = "succeeded"
	SettlementActionFailed    = "failed"
)

// 子动作类型（与 settlement_action_ledger.action_type 注释一致，另加净额动作）。
const (
	ActionRentRefund    = "rent_refund"
	ActionDepositRefund = "deposit_refund"
	ActionFeeCharge     = "fee_charge"
	ActionPointsRestore = "points_restore"
	ActionNetRefund     = "net_refund"
	ActionNetCharge     = "net_charge"
)

const (
	actionDirectionCredit = "credit"
	actionDirectionDebit  = "debit"
)

var (
	// ErrSettlementExecutorInProgress 表示另一个执行器正在处理同一订单。
	ErrSettlementExecutorInProgress = errors.New("settlement executor: another executor is processing this settlement")
	// ErrSettlementActionFailed 表示至少一个子动作失败（状态已落库，可重试）。
	ErrSettlementActionFailed = errors.New("settlement executor: one or more actions failed")
)

// ActiveSettlementStatuses 是订单级唯一最终结算的状态集合（与迁移
// 20261116001 的 partial unique index 完全一致；superseded/calculated 不参与）。
var ActiveSettlementStatuses = []string{
	SettlementStatusConfirmed,
	SettlementStatusProcessing,
	SettlementStatusSucceeded,
	SettlementStatusPartiallySucceeded,
	SettlementStatusFailedRetryable,
}

// SettlementActionPlan 是从计算结果映射出的一个子动作（不含金额推导）。
type SettlementActionPlan struct {
	Type        string `json:"type"`
	Direction   string `json:"direction"` // credit（退款）| debit（扣收）
	AmountCents int64  `json:"amount_cents"`
	Points      int64  `json:"points"`
}

// SettlementExecutionRequest 是执行入口参数。
type SettlementExecutionRequest struct {
	OrderID      string          `json:"order_id"`
	Input        SettlementInput `json:"input"`
	AllowNetting bool            `json:"allow_netting"` // 渠道/资金来源/协议均允许时才净额（D3 默认分笔）
}

// SettlementActionRequest 是交给渠道适配器的子动作（幂等键已预生成）。
type SettlementActionRequest struct {
	SettlementID        string `json:"settlement_id"`
	OrderID             string `json:"order_id"`
	SettlementVersion   int64  `json:"settlement_version"`
	ActionType          string `json:"action_type"`
	Direction           string `json:"direction"`
	AmountCents         int64  `json:"amount_cents"`
	Points              int64  `json:"points"`
	IdempotencyKey      string `json:"idempotency_key"`
	SourceTransactionID string `json:"source_transaction_id"`
}

// SettlementActionRunner 是资金动作端口（微信退款/乐币恢复/扣收）。
// 实现方必须保证同一 IdempotencyKey 只产生一次资金动作。
type SettlementActionRunner interface {
	Execute(ctx context.Context, req SettlementActionRequest) (channelTxnID string, err error)
}

// SettlementActionRunnerFunc 便于测试/组合。
type SettlementActionRunnerFunc func(ctx context.Context, req SettlementActionRequest) (string, error)

func (f SettlementActionRunnerFunc) Execute(ctx context.Context, req SettlementActionRequest) (string, error) {
	return f(ctx, req)
}

// SettlementExecutorHooks 仅用于故障注入测试（生产留空）。
type SettlementExecutorHooks struct {
	// AfterAction 在子动作成功落库后调用；返回 error 模拟「动作已成功但进程中断」。
	AfterAction func(ledger models.SettlementActionLedger) error
}

// SettlementExecutor 串行化并执行最终结算。
type SettlementExecutor struct {
	DB         *gorm.DB
	Calculator *SettlementCalculator
	Runner     SettlementActionRunner
	Hooks      SettlementExecutorHooks
	Now        func() time.Time

	PollInterval time.Duration
	PollTimeout  time.Duration
}

// NewSettlementExecutor 构造执行器（Calculator 默认 #2359 实现）。
func NewSettlementExecutor(db *gorm.DB, runner SettlementActionRunner) *SettlementExecutor {
	return &SettlementExecutor{
		DB:           db,
		Calculator:   NewSettlementCalculator(),
		Runner:       runner,
		Now:          time.Now,
		PollInterval: 10 * time.Millisecond,
		PollTimeout:  5 * time.Second,
	}
}

// SettlementExecutionResult 是执行结果（含子动作明细）。
type SettlementExecutionResult struct {
	SettlementID string                    `json:"settlement_id"`
	OrderID      string                    `json:"order_id"`
	Version      int64                     `json:"settlement_version"`
	Status       string                    `json:"status"`
	NetCashCents int64                     `json:"net_cash_cents"`
	Actions      []SettlementActionOutcome `json:"actions"`
	Supersedes   string                    `json:"supersedes,omitempty"`
}

// SettlementActionOutcome 是单个子动作的终态。
type SettlementActionOutcome struct {
	Type           string `json:"type"`
	Status         string `json:"status"`
	AmountCents    int64  `json:"amount_cents"`
	Points         int64  `json:"points"`
	IdempotencyKey string `json:"idempotency_key"`
	Error          string `json:"error,omitempty"`
}

// PlanSettlementActions 把计算结果映射为子动作；只读结果字段，不重新推导金额。
//
// 分账户（D3 默认）：rent_refund / deposit_refund / fee_charge / points_restore；
// 净额（渠道/资金来源/协议均允许）：单个 net_refund/net_charge + points_restore。
// 现金动作合计（credit − debit）必须等于 net_cash，否则返回守恒错误（不静默）。
func PlanSettlementActions(res *SettlementResult, allowNetting bool) ([]SettlementActionPlan, error) {
	if res == nil {
		return nil, fmt.Errorf("%w: nil settlement result", ErrSettlementInvalidInput)
	}
	plans := make([]SettlementActionPlan, 0, 4)
	if allowNetting && res.NetCashCents != 0 {
		planType := ActionNetRefund
		direction := actionDirectionCredit
		amount := res.NetCashCents
		if amount < 0 {
			planType = ActionNetCharge
			direction = actionDirectionDebit
			amount = -amount
		}
		plans = append(plans, SettlementActionPlan{Type: planType, Direction: direction, AmountCents: amount})
	} else {
		if res.RentRefundCents > 0 {
			plans = append(plans, SettlementActionPlan{Type: ActionRentRefund, Direction: actionDirectionCredit, AmountCents: res.RentRefundCents})
		}
		if res.DepositRefundCents > 0 {
			plans = append(plans, SettlementActionPlan{Type: ActionDepositRefund, Direction: actionDirectionCredit, AmountCents: res.DepositRefundCents})
		}
		charge := res.RentShortfallCents + res.OtherFeeShortfallCents
		if charge > 0 {
			plans = append(plans, SettlementActionPlan{Type: ActionFeeCharge, Direction: actionDirectionDebit, AmountCents: charge})
		}
	}
	if res.GiftPointsRefunded > 0 {
		plans = append(plans, SettlementActionPlan{Type: ActionPointsRestore, Direction: actionDirectionCredit, Points: res.GiftPointsRefunded})
	}

	// 现金守恒：Σ credit − Σ debit == net_cash。
	var sum int64
	for _, p := range plans {
		if p.Direction == actionDirectionCredit {
			sum += p.AmountCents
		} else {
			sum -= p.AmountCents
		}
	}
	if sum != res.NetCashCents {
		return nil, fmt.Errorf("%w: actions cash sum %d != net_cash %d", ErrSettlementInvariantViolation, sum, res.NetCashCents)
	}
	return plans, nil
}

// Execute 执行（或幂等重试）同一订单的最终结算。
func (e *SettlementExecutor) Execute(ctx context.Context, req SettlementExecutionRequest) (*SettlementExecutionResult, error) {
	if e == nil || e.DB == nil {
		return nil, errors.New("settlement executor: DB is required")
	}
	if e.Runner == nil {
		return nil, errors.New("settlement executor: action runner is required")
	}
	if req.OrderID == "" {
		return nil, fmt.Errorf("%w: order_id is required", ErrSettlementInvalidInput)
	}
	calc := e.Calculator
	if calc == nil {
		calc = NewSettlementCalculator()
	}
	if req.Input.SettlementVersion <= 0 {
		req.Input.SettlementVersion = 1
	}
	if req.Input.OrderID == "" {
		req.Input.OrderID = req.OrderID
	}

	res, err := calc.Calculate(req.Input)
	if err != nil {
		return nil, err
	}
	plans, err := PlanSettlementActions(res, req.AllowNetting)
	if err != nil {
		return nil, err
	}

	row, err := e.ensureSettlement(ctx, req.OrderID, res, plans)
	if err != nil {
		return nil, err
	}

	switch row.Status {
	case SettlementStatusSucceeded:
		// 已完成：返回幂等结果，绝不重放任何动作。
		return e.resultFromRow(ctx, row)
	case SettlementStatusSuperseded:
		return nil, fmt.Errorf("%w: settlement version %d is superseded", ErrSettlementDuplicateActive, row.SettlementVersion)
	}
	// calculated / confirmed / processing / partially_succeeded / failed_retryable：
	// 继续执行——runActions 跳过已成功动作，只重试 pending/failed 子动作。

	if err := e.runActions(ctx, row, plans); err != nil && !errors.Is(err, ErrSettlementActionFailed) {
		return nil, err
	}

	finalRow, err := e.finalizeSettlement(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	result, err := e.resultFromRow(ctx, finalRow)
	if err != nil {
		return nil, err
	}
	if finalRow.Status == SettlementStatusFailedRetryable {
		return result, ErrSettlementActionFailed
	}
	return result, nil
}

// ensureSettlement 在订单行锁内创建/复用结算行与 pending 子动作。
func (e *SettlementExecutor) ensureSettlement(ctx context.Context, orderID string, res *SettlementResult, plans []SettlementActionPlan) (*models.SettlementCalculation, error) {
	tx := e.DB.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	defer func() { _ = tx.Rollback() }()

	var order models.Order
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", orderID).First(&order).Error; err != nil {
		return nil, fmt.Errorf("settlement executor: lock order %s: %w", orderID, err)
	}

	var existing models.SettlementCalculation
	err := tx.Where("order_id = ? AND settlement_version = ?", orderID, res.SettlementVersion).
		First(&existing).Error
	if err == nil {
		if existing.Status == SettlementStatusSuperseded {
			return nil, fmt.Errorf("%w: settlement version %d is superseded", ErrSettlementDuplicateActive, res.SettlementVersion)
		}
		if err := tx.Commit().Error; err != nil {
			return nil, err
		}
		return &existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	var active models.SettlementCalculation
	if err := tx.Where("order_id = ? AND status IN ?", orderID, ActiveSettlementStatuses).
		First(&active).Error; err == nil {
		return nil, fmt.Errorf("%w: order %s already has active settlement version %d",
			ErrSettlementDuplicateActive, orderID, active.SettlementVersion)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	now := e.now()
	row := models.SettlementCalculation{
		ID:                   uuid.New().String(),
		OrderID:              orderID,
		TenantID:             order.TenantID,
		Trigger:              "confirm",
		ActualDays:           res.ActualDays,
		LedgerVersion:        res.SettlementVersion,
		SettlementVersion:    res.SettlementVersion,
		Status:               SettlementStatusConfirmed,
		AlgorithmVersion:     res.AlgorithmVersion,
		RatioSource:          res.RatioSource,
		CalculatedAt:         &now,
		PricingPolicyVersion: res.PricingPolicyVersion,

		RentCashPaidNetCents:         models.Cents(res.RentCashPaidNetCents),
		DepositCashCollectedNetCents: models.Cents(res.DepositCashCollectedNetCents),
		DepositHeldCents:             models.Cents(res.DepositHeldCents),
		OverduePenaltyCents:          models.Cents(res.OverduePenaltyCents),
		DamageFeeCents:               models.Cents(res.DamageFeeCents),
		LogisticsFeeCents:            models.Cents(res.LogisticsFeeCents),
		LegacyMarker:                 res.LegacyMarker,
	}
	if res.RatioNum != 0 || res.RatioDen != 0 {
		num, den := int(res.RatioNum), int(res.RatioDen)
		row.RatioNum, row.RatioDen = &num, &den
	}
	if b, err := json.Marshal(res); err == nil {
		s := string(b)
		row.Result = &s
		row.AccountsJSON = &s
	}
	if b, err := json.Marshal(map[string]interface{}{
		"settlement_id": res.SettlementID, "order_id": res.OrderID,
		"rent_cash_paid_net_cents":         res.RentCashPaidNetCents,
		"deposit_cash_collected_net_cents": res.DepositCashCollectedNetCents,
		"deposit_held_cents":               res.DepositHeldCents,
	}); err == nil {
		s := string(b)
		row.InputSnapshot = &s
	}

	if err := tx.Create(&row).Error; err != nil {
		return nil, fmt.Errorf("settlement executor: create settlement: %w", err)
	}
	// 状态机：calculated/confirmed → processing（执行开始）。
	if err := tx.Model(&row).Update("status", SettlementStatusProcessing).Error; err != nil {
		return nil, err
	}
	row.Status = SettlementStatusProcessing

	for _, plan := range plans {
		ledger := models.SettlementActionLedger{
			ID:                  uuid.New().String(),
			SettlementID:        row.ID,
			OrderID:             orderID,
			SettlementVersion:   res.SettlementVersion,
			ActionType:          plan.Type,
			IdempotencyKey:      settlementActionKey(row.ID, plan.Type, orderID),
			ExecutedAmountCents: models.Cents(plan.AmountCents),
			ExecutedPoints:      plan.Points,
			Status:              SettlementActionPending,
			CreatedAt:           now,
			UpdatedAt:           now,
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&ledger).Error; err != nil {
			return nil, fmt.Errorf("settlement executor: create action %s: %w", plan.Type, err)
		}
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// runActions 逐个子动作执行；已成功跳过，pending/failed 先抢占再执行。
func (e *SettlementExecutor) runActions(ctx context.Context, row *models.SettlementCalculation, plans []SettlementActionPlan) error {
	var firstErr error
	for _, plan := range plans {
		key := settlementActionKey(row.ID, plan.Type, row.OrderID)
		ledger, err := e.loadAction(ctx, key)
		if err != nil {
			return err
		}
		if ledger.Status == SettlementActionSucceeded {
			continue
		}

		claimed, err := e.claimAction(ctx, ledger.ID)
		if err != nil {
			return err
		}
		if !claimed {
			ledger, err = e.waitAction(ctx, ledger.ID)
			if err != nil {
				return err
			}
			if ledger.Status == SettlementActionSucceeded {
				continue
			}
			if ledger.Status == SettlementActionExecuting {
				return ErrSettlementExecutorInProgress
			}
			// 其它执行器已失败：本执行器重试一次（幂等键相同）。
			claimed, err = e.claimAction(ctx, ledger.ID)
			if err != nil {
				return err
			}
			if !claimed {
				firstErr = fmt.Errorf("settlement executor: action %s is not claimable", plan.Type)
				continue
			}
		}

		txnID, execErr := e.Runner.Execute(ctx, SettlementActionRequest{
			SettlementID:        row.ID,
			OrderID:             row.OrderID,
			SettlementVersion:   row.SettlementVersion,
			ActionType:          plan.Type,
			Direction:           plan.Direction,
			AmountCents:         plan.AmountCents,
			Points:              plan.Points,
			IdempotencyKey:      key,
			SourceTransactionID: "order:" + row.OrderID,
		})
		if execErr != nil {
			msg := execErr.Error()
			_ = e.DB.WithContext(ctx).Model(&models.SettlementActionLedger{}).
				Where("id = ?", ledger.ID).
				Updates(map[string]interface{}{
					"status":        SettlementActionFailed,
					"error_message": msg,
					"retry_count":   gorm.Expr("retry_count + 1"),
					"updated_at":    e.now(),
				}).Error
			if firstErr == nil {
				firstErr = fmt.Errorf("%w: %s: %v", ErrSettlementActionFailed, plan.Type, execErr)
			}
			continue
		}

		updates := map[string]interface{}{
			"status":     SettlementActionSucceeded,
			"updated_at": e.now(),
		}
		if txnID != "" {
			updates["channel_txn_id"] = txnID
		}
		if err := e.DB.WithContext(ctx).Model(&models.SettlementActionLedger{}).
			Where("id = ?", ledger.ID).Updates(updates).Error; err != nil {
			return err
		}
		ledger.Status = SettlementActionSucceeded
		ledger.ChannelTxnID = nil
		if txnID != "" {
			ledger.ChannelTxnID = &txnID
		}
		if e.Hooks.AfterAction != nil {
			if hookErr := e.Hooks.AfterAction(*ledger); hookErr != nil {
				return hookErr
			}
		}
	}
	return firstErr
}

// finalizeSettlement 依据子动作账本推导终态并落库。
func (e *SettlementExecutor) finalizeSettlement(ctx context.Context, settlementID string) (*models.SettlementCalculation, error) {
	var ledgers []models.SettlementActionLedger
	if err := e.DB.WithContext(ctx).Where("settlement_id = ?", settlementID).Find(&ledgers).Error; err != nil {
		return nil, err
	}
	succeeded, failed := 0, 0
	for _, l := range ledgers {
		switch l.Status {
		case SettlementActionSucceeded:
			succeeded++
		case SettlementActionFailed:
			failed++
		default:
			failed++
		}
	}
	status := SettlementStatusSucceeded
	if failed > 0 {
		if succeeded > 0 {
			status = SettlementStatusPartiallySucceeded
		} else {
			status = SettlementStatusFailedRetryable
		}
	}
	if err := e.DB.WithContext(ctx).Model(&models.SettlementCalculation{}).
		Where("id = ?", settlementID).Update("status", status).Error; err != nil {
		return nil, err
	}
	var row models.SettlementCalculation
	if err := e.DB.WithContext(ctx).Where("id = ?", settlementID).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// Supersede 冲正：旧 succeeded/partially_succeeded 行 → superseded（金额不变），
// 新行 calculated + supersedes=旧 id（不执行资金动作）。
func (e *SettlementExecutor) Supersede(ctx context.Context, orderID string, in SettlementInput) (*SettlementExecutionResult, error) {
	if e == nil || e.DB == nil {
		return nil, errors.New("settlement executor: DB is required")
	}
	tx := e.DB.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	defer func() { _ = tx.Rollback() }()

	var order models.Order
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", orderID).First(&order).Error; err != nil {
		return nil, err
	}
	var active models.SettlementCalculation
	if err := tx.Where("order_id = ? AND status IN ?", orderID, ActiveSettlementStatuses).
		Order("settlement_version DESC").First(&active).Error; err != nil {
		return nil, fmt.Errorf("settlement executor: no active settlement for order %s: %w", orderID, err)
	}
	if in.SettlementVersion <= active.SettlementVersion {
		in.SettlementVersion = active.SettlementVersion + 1
	}
	if in.OrderID == "" {
		in.OrderID = orderID
	}
	calc := e.Calculator
	if calc == nil {
		calc = NewSettlementCalculator()
	}
	res, err := calc.Calculate(in)
	if err != nil {
		return nil, err
	}
	now := e.now()
	oldID := active.ID
	row := models.SettlementCalculation{
		ID:                           uuid.New().String(),
		OrderID:                      orderID,
		TenantID:                     order.TenantID,
		Trigger:                      "confirm",
		ActualDays:                   res.ActualDays,
		LedgerVersion:                res.SettlementVersion,
		SettlementVersion:            res.SettlementVersion,
		Status:                       SettlementStatusCalculated,
		AlgorithmVersion:             res.AlgorithmVersion,
		RatioSource:                  res.RatioSource,
		CalculatedAt:                 &now,
		Supersedes:                   &oldID,
		RentCashPaidNetCents:         models.Cents(res.RentCashPaidNetCents),
		DepositCashCollectedNetCents: models.Cents(res.DepositCashCollectedNetCents),
		DepositHeldCents:             models.Cents(res.DepositHeldCents),
		OverduePenaltyCents:          models.Cents(res.OverduePenaltyCents),
		DamageFeeCents:               models.Cents(res.DamageFeeCents),
		LogisticsFeeCents:            models.Cents(res.LogisticsFeeCents),
		PricingPolicyVersion:         res.PricingPolicyVersion,
		LegacyMarker:                 res.LegacyMarker,
	}
	if res.RatioNum != 0 || res.RatioDen != 0 {
		num, den := int(res.RatioNum), int(res.RatioDen)
		row.RatioNum, row.RatioDen = &num, &den
	}
	if b, err := json.Marshal(res); err == nil {
		s := string(b)
		row.Result = &s
		row.AccountsJSON = &s
	}
	// 先插新行（calculated 不参与 active 唯一索引），再降级旧行。
	if err := tx.Create(&row).Error; err != nil {
		return nil, err
	}
	if err := tx.Model(&models.SettlementCalculation{}).Where("id = ?", active.ID).
		Update("status", SettlementStatusSuperseded).Error; err != nil {
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return &SettlementExecutionResult{
		SettlementID: row.ID,
		OrderID:      orderID,
		Version:      row.SettlementVersion,
		Status:       row.Status,
		NetCashCents: res.NetCashCents,
		Supersedes:   oldID,
	}, nil
}

func (e *SettlementExecutor) resultFromRow(ctx context.Context, row *models.SettlementCalculation) (*SettlementExecutionResult, error) {
	var ledgers []models.SettlementActionLedger
	if err := e.DB.WithContext(ctx).Where("settlement_id = ?", row.ID).
		Order("created_at ASC").Find(&ledgers).Error; err != nil {
		return nil, err
	}
	out := &SettlementExecutionResult{
		SettlementID: row.ID,
		OrderID:      row.OrderID,
		Version:      row.SettlementVersion,
		Status:       row.Status,
		Actions:      make([]SettlementActionOutcome, 0, len(ledgers)),
	}
	if row.Supersedes != nil {
		out.Supersedes = *row.Supersedes
	}
	if row.AccountsJSON != nil {
		var res SettlementResult
		if json.Unmarshal([]byte(*row.AccountsJSON), &res) == nil {
			out.NetCashCents = res.NetCashCents
		}
	}
	for _, l := range ledgers {
		outcome := SettlementActionOutcome{
			Type:           l.ActionType,
			Status:         l.Status,
			AmountCents:    int64(l.ExecutedAmountCents),
			Points:         l.ExecutedPoints,
			IdempotencyKey: l.IdempotencyKey,
		}
		if l.ErrorMessage != nil {
			outcome.Error = *l.ErrorMessage
		}
		out.Actions = append(out.Actions, outcome)
	}
	return out, nil
}

func (e *SettlementExecutor) loadAction(ctx context.Context, key string) (*models.SettlementActionLedger, error) {
	var ledger models.SettlementActionLedger
	if err := e.DB.WithContext(ctx).Where("idempotency_key = ?", key).First(&ledger).Error; err != nil {
		return nil, fmt.Errorf("settlement executor: load action %s: %w", key, err)
	}
	return &ledger, nil
}

func (e *SettlementExecutor) claimAction(ctx context.Context, id string) (bool, error) {
	res := e.DB.WithContext(ctx).Model(&models.SettlementActionLedger{}).
		Where("id = ? AND status IN ?", id, []string{SettlementActionPending, SettlementActionFailed}).
		Updates(map[string]interface{}{"status": SettlementActionExecuting, "updated_at": e.now()})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

func (e *SettlementExecutor) waitAction(ctx context.Context, id string) (*models.SettlementActionLedger, error) {
	deadline := time.Now().Add(e.pollTimeout())
	for {
		var ledger models.SettlementActionLedger
		if err := e.DB.WithContext(ctx).Where("id = ?", id).First(&ledger).Error; err != nil {
			return nil, err
		}
		if ledger.Status != SettlementActionExecuting {
			return &ledger, nil
		}
		if time.Now().After(deadline) {
			return &ledger, nil
		}
		time.Sleep(e.pollInterval())
	}
}

func (e *SettlementExecutor) pollInterval() time.Duration {
	if e.PollInterval > 0 {
		return e.PollInterval
	}
	return 10 * time.Millisecond
}

func (e *SettlementExecutor) pollTimeout() time.Duration {
	if e.PollTimeout > 0 {
		return e.PollTimeout
	}
	return 5 * time.Second
}

func (e *SettlementExecutor) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

// settlementActionKey 是预生成的稳定幂等键：settlement_id + action_type + source_transaction_id。
func settlementActionKey(settlementID, actionType, orderID string) string {
	return fmt.Sprintf("%s:%s:order:%s", settlementID, actionType, orderID)
}
