package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"cave-microclimate-clearance/internal/domain"
)

type auditMaterial struct {
	EventID        string `json:"event_id"`
	TrialID        string `json:"trial_id"`
	Sequence       int64  `json:"sequence"`
	EventType      string `json:"event_type"`
	ActorID        string `json:"actor_id"`
	RequestID      string `json:"request_id"`
	PayloadDigest  string `json:"payload_digest"`
	PreviousDigest string `json:"previous_digest"`
	OccurredAt     string `json:"occurred_at"`
}

func eventDigest(e domain.AuditEvent) string {
	m := auditMaterial{e.EventID, e.TrialID, e.Sequence, e.EventType, e.ActorID, e.RequestID, e.PayloadDigest, e.PreviousDigest, e.OccurredAt.UTC().Format("2006-01-02T15:04:05.000000000Z")}
	b, _ := json.Marshal(m)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func verifyEvents(trialID string, events []domain.AuditEvent) error {
	previous := ""
	for i, e := range events {
		if e.TrialID != trialID {
			return &CorruptError{fmt.Sprintf("事件 %d trial_id 不匹配", i+1)}
		}
		if e.Sequence != int64(i+1) {
			return &CorruptError{fmt.Sprintf("事件序号在 %d 处不连续", i+1)}
		}
		if e.PreviousDigest != previous {
			return &CorruptError{fmt.Sprintf("事件 %d 前序摘要不匹配", i+1)}
		}
		if eventDigest(e) != e.EventDigest {
			return &CorruptError{fmt.Sprintf("事件 %d 内容摘要不匹配", i+1)}
		}
		previous = e.EventDigest
	}
	return nil
}

func chainDigest(events []domain.AuditEvent) string {
	if len(events) == 0 {
		return ""
	}
	return events[len(events)-1].EventDigest
}

func ChainDigest(events []domain.AuditEvent) string { return chainDigest(events) }
