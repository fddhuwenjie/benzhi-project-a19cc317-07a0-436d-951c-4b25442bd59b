package application

import (
	"time"

	"cave-microclimate-clearance/internal/domain"
)

type CommandMeta struct {
	RequestID        string `json:"request_id"`
	ExpectedRevision int64  `json:"expected_revision"`
	ActorID          string `json:"actor_id"`
}

type CreateTrialCommand struct {
	CommandMeta
	TrialID         string                 `json:"trial_id"`
	CaveSectionID   string                 `json:"cave_section_id"`
	TestWindowStart time.Time              `json:"test_window_start"`
	TestWindowEnd   time.Time              `json:"test_window_end"`
	LeadObserverID  string                 `json:"lead_observer_id"`
	Baseline        domain.BaselineProfile `json:"baseline"`
	Thresholds      *domain.Thresholds     `json:"thresholds,omitempty"`
}

type ObservationCommand struct {
	CommandMeta
	Observation domain.LoadStageObservation `json:"observation"`
}
type AssessmentCommand struct{ CommandMeta }
type RecoveryCommand struct {
	CommandMeta
	IsolationMeasures   []string              `json:"isolation_measures"`
	IsolationExecutorID string                `json:"isolation_executor_id,omitempty"`
	MeasureCompletedAt  time.Time             `json:"measure_completed_at"`
	Samples             []domain.SensorSample `json:"samples"`
}
type ReviewCommand struct {
	CommandMeta
	Approved              bool                     `json:"approved"`
	Checks                domain.ReviewChecks      `json:"checks"`
	RejectionReasons      []domain.RejectionReason `json:"rejection_reasons,omitempty"`
	MaxConcurrentVisitors int                      `json:"max_concurrent_visitors,omitempty"`
	MaxStayMinutes        int                      `json:"max_stay_minutes,omitempty"`
	ValidFrom             time.Time                `json:"valid_from,omitempty"`
	ValidUntil            time.Time                `json:"valid_until,omitempty"`
}

type CommandResponse struct {
	TrialID         string                              `json:"trial_id"`
	Status          domain.Status                       `json:"status"`
	Revision        int64                               `json:"revision"`
	ResourceID      string                              `json:"resource_id,omitempty"`
	Coverage        *domain.SamplingCompletenessSummary `json:"coverage,omitempty"`
	RecoveryAttempt *domain.RecoveryRecord              `json:"recovery_attempt,omitempty"`
	Reconciliation  *domain.ReviewReconciliation        `json:"reconciliation,omitempty"`
}

type CommandResult struct {
	Trial        *domain.ClearanceTrial
	ResponseJSON []byte
	Replayed     bool
}
