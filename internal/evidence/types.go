package evidence

import (
	"time"

	"cave-microclimate-clearance/internal/domain"
)

const FormatVersion = domain.EvidenceFormatVersion

type Package struct {
	FormatVersion    string                `json:"format_version"`
	TrialID          string                `json:"trial_id"`
	GeneratedAt      time.Time             `json:"generated_at"`
	Trial            domain.ClearanceTrial `json:"trial"`
	AuditEvents      []domain.AuditEvent   `json:"audit_events"`
	AuditChainDigest string                `json:"audit_chain_digest"`
	ContentDigest    string                `json:"content_digest"`
}

type Verification struct {
	TrialID                string                 `json:"trial_id"`
	Valid                  bool                   `json:"valid"`
	ContentDigestMatches   bool                   `json:"content_digest_matches"`
	AuditChainValid        bool                   `json:"audit_chain_valid"`
	TerminalDigestMatches  bool                   `json:"terminal_digest_matches"`
	SemanticReconciliation SemanticReconciliation `json:"semantic_reconciliation"`
	Problems               []string               `json:"problems"`
}

type SemanticEventCount struct {
	EventType string `json:"event_type"`
	Expected  int    `json:"expected"`
	Actual    int    `json:"actual"`
}

type SemanticIssue struct {
	Code              string `json:"code"`
	EventSequence     int64  `json:"event_sequence,omitempty"`
	FactReference     string `json:"fact_reference"`
	ExpectedEventType string `json:"expected_event_type,omitempty"`
	ActualEventType   string `json:"actual_event_type,omitempty"`
	ExpectedActorID   string `json:"expected_actor_id,omitempty"`
	ActualActorID     string `json:"actual_actor_id,omitempty"`
	Message           string `json:"message"`
}

type SemanticReconciliation struct {
	Valid       bool                 `json:"valid"`
	EventCounts []SemanticEventCount `json:"event_counts"`
	Issues      []SemanticIssue      `json:"issues"`
}
