package activity

import (
	"context"
	"fmt"
	"log"

	"github.com/argus-platform/argus/internal/types"
)

// NotifyReviewerActivity 通知审核人有新 IoC 待审核
func NotifyReviewerActivity(ctx context.Context, data types.EnrichedData) error {
	log.Printf("[activity] notify reviewer: IoC %s (confidence=%d, iocs=%d)",
		data.ID, data.Confidence, len(data.IOCs))
	// TODO: send email / Slack / DingTalk
	return nil
}

// PushToMISPActivity 推送 IoC 到 MISP
func PushToMISPActivity(ctx context.Context, data types.EnrichedData) error {
	log.Printf("[activity] push to MISP: %s (confidence=%d)", data.ID, data.Confidence)
	// TODO: actual MISP API call
	return nil
}

// PushToMISPWithTagActivity 推送 IoC 到 MISP 并附带标签
func PushToMISPWithTagActivity(ctx context.Context, data types.EnrichedData, tag string) error {
	log.Printf("[activity] push to MISP with tag %q: %s", tag, data.ID)
	// TODO: actual MISP API call with tag
	return nil
}

// PushToOpenCTIActivity 推送 IoC 到 OpenCTI
func PushToOpenCTIActivity(ctx context.Context, data types.EnrichedData) error {
	log.Printf("[activity] push to OpenCTI: %s (confidence=%d)", data.ID, data.Confidence)
	// TODO: actual OpenCTI GraphQL call
	return nil
}

// EscalateReviewActivity 升级通知：催审
func EscalateReviewActivity(ctx context.Context, data types.EnrichedData) error {
	log.Printf("[activity] escalate review: %s (confidence=%d) — 24h passed, re-notify reviewer",
		data.ID, data.Confidence)
	// TODO: urgent notification (email/Slack/DingTalk with higher priority)
	return nil
}

// LogAuditActivity 记录审计日志
func LogAuditActivity(ctx context.Context, workflowID string, status string, data types.EnrichedData) error {
	log.Printf("[activity] audit: workflow=%s status=%s id=%s", workflowID, status, data.ID)
	// TODO: write to audit log (ES / DB)
	return nil
}

// CompositeActivityError 返回友好的错误信息
func CompositeActivityError(activity string, err error) error {
	return fmt.Errorf("[activity %s] %w", activity, err)
}
