package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

type ExpectedAuditEvent struct {
	EventType     string
	ActorID       string
	FactReference string
	PayloadDigest string
}

func AuditPayloadDigest(operation string, value any) string {
	b, _ := json.Marshal(struct {
		Operation string `json:"operation"`
		Value     any    `json:"value"`
	}{operation, value})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func ObservationAuditPayload(o LoadStageObservation) string {
	return AuditPayloadDigest("observation_fact", o)
}

type creationAuditFact struct {
	TrialID         string          `json:"trial_id"`
	CaveSectionID   string          `json:"cave_section_id"`
	TestWindowStart string          `json:"test_window_start"`
	TestWindowEnd   string          `json:"test_window_end"`
	LeadObserverID  string          `json:"lead_observer_id"`
	Baseline        BaselineProfile `json:"baseline"`
	Thresholds      Thresholds      `json:"thresholds"`
}

func CreationAuditPayload(t *ClearanceTrial) string {
	return AuditPayloadDigest("creation_fact", creationAuditFact{TrialID: t.TrialID, CaveSectionID: t.CaveSectionID, TestWindowStart: t.TestWindowStart.UTC().Format(time.RFC3339Nano), TestWindowEnd: t.TestWindowEnd.UTC().Format(time.RFC3339Nano), LeadObserverID: t.LeadObserverID, Baseline: t.Baseline, Thresholds: t.Thresholds})
}

func AssessmentAuditPayload(a ThresholdAssessment) string {
	return AuditPayloadDigest("assessment_fact", a)
}

func RecoveryAuditPayload(r RecoveryRecord) string {
	return AuditPayloadDigest("recovery_fact", r)
}

type terminalAuditFact struct {
	Status           Status            `json:"status"`
	Review           *ReviewDecision   `json:"review"`
	Permit           *ClearancePermit  `json:"permit,omitempty"`
	RejectionReasons []RejectionReason `json:"rejection_reasons,omitempty"`
}

func TerminalAuditPayload(t *ClearanceTrial) string {
	var permit *ClearancePermit
	if t.Permit != nil {
		copy := *t.Permit
		copy.EvidenceDigest = ""
		permit = &copy
	}
	return AuditPayloadDigest("terminal_fact", terminalAuditFact{Status: t.Status, Review: t.Review, Permit: permit, RejectionReasons: t.RejectionReasons})
}

func ExpectedAuditSequence(t *ClearanceTrial) []ExpectedAuditEvent {
	if t == nil {
		return nil
	}
	expected := []ExpectedAuditEvent{{EventType: "trial_created_baseline_frozen", ActorID: t.LeadObserverID, FactReference: "baseline.synchronization", PayloadDigest: CreationAuditPayload(t)}}
	for i, observation := range t.Observations {
		expected = append(expected, ExpectedAuditEvent{
			EventType:     "load_stage_observed_" + string(observation.Stage),
			ActorID:       observation.ObserverID,
			FactReference: fmt.Sprintf("observations[%d]:%s", i, observation.ObservationID),
			PayloadDigest: ObservationAuditPayload(observation),
		})
	}
	if t.Assessment != nil {
		expected = append(expected, ExpectedAuditEvent{EventType: "threshold_assessed", ActorID: t.LeadObserverID, FactReference: "assessment:" + t.Assessment.AssessmentID, PayloadDigest: AssessmentAuditPayload(*t.Assessment)})
	}
	for i, recovery := range t.RecoveryAttempts {
		eventType := "recovery_attempt_failed"
		if recovery.Passed {
			eventType = "recovery_window_verified"
		}
		expected = append(expected, ExpectedAuditEvent{EventType: eventType, ActorID: recovery.ObserverID, FactReference: fmt.Sprintf("recovery_attempts[%d]:%s;isolation_executor:%s", i, recovery.AttemptID, recovery.IsolationExecutorID), PayloadDigest: RecoveryAuditPayload(recovery)})
	}
	if t.Review != nil {
		eventType := "review_rejected"
		fact := "review:rejected"
		if t.Status == StatusPermitted {
			eventType = "permit_issued"
			if t.Permit != nil {
				fact = "permit:" + t.Permit.PermitID
			}
		}
		expected = append(expected, ExpectedAuditEvent{EventType: eventType, ActorID: t.Review.ReviewerID, FactReference: fact, PayloadDigest: TerminalAuditPayload(t)})
	}
	return expected
}
