package workflow

import (
	"time"

	"go.temporal.io/sdk/workflow"
	"github.com/argus-platform/argus/internal/types"
	"github.com/argus-platform/argus/internal/activity"
)

const (
	ReviewQueue        = "IOC_REVIEW_QUEUE"
	ReviewTimeout      = 24 * time.Hour
	ReviewEscalateTTL  = 48 * time.Hour
	SignalApprove      = "signal-approve"
	SignalReject       = "signal-reject"
	SignalReviewer     = "signal-reviewer"
)

type ReviewState struct {
	Data         types.EnrichedData `json:"data"`
	Confidence   int    `json:"confidence"`
	Status       string `json:"status"` // pending, approved, rejected, auto-pushed, escalated, discarded
	Reviewer     string `json:"reviewer,omitempty"`
	Escalated    bool   `json:"escalated"`
	RejectReason string `json:"reject_reason,omitempty"`
}

// IoCReviewWorkflow 人工审核工作流
// 1. 等待审核 24h
// 2. 收到 approve/reject signal → 处理
// 3. 超时 → 按置信度分级处理
func IoCReviewWorkflow(ctx workflow.Context, data types.EnrichedData) (*ReviewState, error) {
	logger := workflow.GetLogger(ctx)
	state := &ReviewState{
		Data:       data,
		Confidence: data.Confidence,
		Status:     "pending",
	}

	logger.Info("IoC review workflow started", "id", data.ID, "confidence", data.Confidence)

	// 1. 通知审核人
	ctxNotify := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 10 * time.Second,
		TaskQueue:           ReviewQueue,
	})
	err := workflow.ExecuteActivity(ctxNotify, activity.NotifyReviewerActivity, data).Get(ctx, nil)
	if err != nil {
		logger.Warn("notify reviewer failed", "error", err)
	}

	// 2. 等待 signal 或超时
	selector := workflow.NewSelector(ctx)
	var approved bool
	var rejected bool
	var reviewer string

	selector.AddReceive(workflow.GetSignalChannel(ctx, SignalApprove), func(c workflow.ReceiveChannel, _ bool) {
		c.Receive(ctx, &reviewer)
		approved = true
	})
	selector.AddReceive(workflow.GetSignalChannel(ctx, SignalReject), func(c workflow.ReceiveChannel, _ bool) {
		var reason string
		c.Receive(ctx, &reason)
		rejected = true
		state.RejectReason = reason
	})
	selector.AddReceive(workflow.GetSignalChannel(ctx, SignalReviewer), func(c workflow.ReceiveChannel, _ bool) {
		c.Receive(ctx, &reviewer)
	})

	// 等待 24h 或 signal
	timerCtx, cancel := workflow.WithCancel(ctx)
	defer cancel()
	timerFuture := workflow.NewTimer(timerCtx, ReviewTimeout)

	selector.AddFuture(timerFuture, func(f workflow.Future) {})
	selector.Select(ctx) // blocks until signal or timeout

	if approved {
		state.Status = "approved"
		state.Reviewer = reviewer

		// 推送 MISP + OpenCTI
		ctxPush := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
			StartToCloseTimeout: 30 * time.Second,
			TaskQueue:           ReviewQueue,
		})
		err := workflow.ExecuteActivity(ctxPush, activity.PushToMISPActivity, data).Get(ctx, nil)
		if err != nil {
			logger.Error("push to MISP failed", "error", err)
		}
		err = workflow.ExecuteActivity(ctxPush, activity.PushToOpenCTIActivity, data).Get(ctx, nil)
		if err != nil {
			logger.Error("push to OpenCTI failed", "error", err)
		}

		logger.Info("IoC review approved", "id", data.ID, "reviewer", reviewer)
		return state, nil
	}

	if rejected {
		state.Status = "rejected"
		state.Reviewer = reviewer
		logger.Info("IoC review rejected", "id", data.ID, "reason", state.RejectReason)
		return state, nil
	}

	// ── 3. 超时处理 ──
	ctxTimeout := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		TaskQueue:           ReviewQueue,
	})

	switch {
	case state.Confidence >= 80:
		state.Status = "auto-pushed"
		// 高置信度：自动推入并标注待核实
		err := workflow.ExecuteActivity(ctxTimeout, activity.PushToMISPWithTagActivity, data, "pending_verification").Get(ctx, nil)
		if err != nil {
			logger.Error("auto push to MISP failed", "error", err)
		}
		err = workflow.ExecuteActivity(ctxTimeout, activity.PushToOpenCTIActivity, data).Get(ctx, nil)
		if err != nil {
			logger.Error("auto push to OpenCTI failed", "error", err)
		}
		logger.Info("IoC auto-pushed (high confidence)", "id", data.ID, "confidence", state.Confidence)

	case state.Confidence >= 60:
		state.Status = "escalated"
		state.Escalated = true
		// 中置信度：升级通知，二次等待
		err := workflow.ExecuteActivity(ctxTimeout, activity.EscalateReviewActivity, data).Get(ctx, nil)
		if err != nil {
			logger.Warn("escalate failed", "error", err)
		}

		// 再等 24h（总共 48h）
		escalateTimer := workflow.NewTimer(ctx, ReviewEscalateTTL-ReviewTimeout)
		selector2 := workflow.NewSelector(ctx)
		var approved2 bool
		var rejected2 bool
		selector2.AddReceive(workflow.GetSignalChannel(ctx, SignalApprove), func(c workflow.ReceiveChannel, _ bool) {
			c.Receive(ctx, &reviewer)
			approved2 = true
		})
		selector2.AddReceive(workflow.GetSignalChannel(ctx, SignalReject), func(c workflow.ReceiveChannel, _ bool) {
			var reason string
			c.Receive(ctx, &reason)
			rejected2 = true
		})
		selector2.AddFuture(escalateTimer, func(f workflow.Future) {})
		selector2.Select(ctx)

		if approved2 {
			state.Status = "approved"
			state.Reviewer = reviewer
			workflow.ExecuteActivity(ctxTimeout, activity.PushToMISPActivity, data).Get(ctx, nil)
			workflow.ExecuteActivity(ctxTimeout, activity.PushToOpenCTIActivity, data).Get(ctx, nil)
		} else if rejected2 {
			state.Status = "rejected"
		} else {
			// 二次超时 → 丢弃
			state.Status = "discarded"
		}

	default:
		state.Status = "discarded"
		logger.Info("IoC discarded (low confidence)", "id", data.ID, "confidence", state.Confidence)
	}

	return state, nil
}
