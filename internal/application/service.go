package application

import (
	"context"
	"encoding/json"
	"time"

	"cave-microclimate-clearance/internal/domain"
	"cave-microclimate-clearance/internal/store"
)

type Service struct {
	repo store.Repository
	now  func() time.Time
}

func NewService(repo store.Repository) *Service { return &Service{repo: repo, now: time.Now} }

func result(r store.TransactionResult) CommandResult {
	return CommandResult{Trial: r.Trial, ResponseJSON: r.ResponseJSON, Replayed: r.Replayed}
}

func (s *Service) CreateTrial(ctx context.Context, c CreateTrialCommand) (CommandResult, error) {
	if err := validateMeta(c.CommandMeta, true); err != nil {
		return CommandResult{}, err
	}
	if c.TrialID == "" {
		c.TrialID = stableID("trial", c.ActorID+"\x00"+c.RequestID)
	}
	if c.Baseline.BaselineID == "" {
		c.Baseline.BaselineID = stableID("baseline", c.ActorID+"\x00"+c.RequestID)
	}
	if c.LeadObserverID != c.ActorID {
		return CommandResult{}, domain.Validation("lead_observer_id", "lead_observer_id 必须与建档 actor_id 一致")
	}
	thresholds := domain.DefaultThresholds()
	if c.Thresholds != nil {
		thresholds = *c.Thresholds
	}
	now := s.now().UTC()
	t, err := domain.NewTrial(domain.CreateInput{TrialID: c.TrialID, CaveSectionID: c.CaveSectionID, WindowStart: c.TestWindowStart, WindowEnd: c.TestWindowEnd, LeadObserverID: c.LeadObserverID, Baseline: c.Baseline, Thresholds: thresholds, Now: now})
	if err != nil {
		return CommandResult{}, err
	}
	response := CommandResponse{TrialID: t.TrialID, Status: t.Status, Revision: 1, ResourceID: t.Baseline.BaselineID}
	createFingerprint := fingerprint("create_trial", struct {
		Command         CreateTrialCommand
		Synchronization domain.BaselineSynchronizationSummary
	}{c, t.Baseline.Synchronization})
	r, err := s.repo.Create(ctx, t, c.RequestID, createFingerprint, domain.CreationAuditPayload(t), c.ActorID, response)
	if err != nil {
		return CommandResult{}, err
	}
	return result(r), nil
}

func (s *Service) AddObservation(ctx context.Context, trialID string, c ObservationCommand) (CommandResult, error) {
	if err := validateMeta(c.CommandMeta, false); err != nil {
		return CommandResult{}, err
	}
	if c.Observation.ObservationID == "" {
		c.Observation.ObservationID = stableID("observation", trialID+"\x00"+c.RequestID)
	}
	if c.Observation.ObserverID == "" {
		c.Observation.ObserverID = c.ActorID
	}
	if c.Observation.ObserverID != c.ActorID {
		return CommandResult{}, domain.Validation("observation.observer_id", "observer_id 必须与写请求 actor_id 一致")
	}
	fp := fingerprint("add_observation", c)
	r, err := s.repo.Transact(ctx, trialID, c.RequestID, fp, c.ExpectedRevision, func(t *domain.ClearanceTrial) (store.Mutation, error) {
		if err := t.AddObservation(c.Observation); err != nil {
			return store.Mutation{}, err
		}
		resp := CommandResponse{TrialID: trialID, Status: t.Status, Revision: c.ExpectedRevision + 1, ResourceID: c.Observation.ObservationID, Coverage: &t.Observations[len(t.Observations)-1].Coverage}
		payload := domain.ObservationAuditPayload(t.Observations[len(t.Observations)-1])
		return store.Mutation{EventType: "load_stage_observed_" + string(c.Observation.Stage), ActorID: c.ActorID, PayloadDigest: payload, Response: resp}, nil
	})
	if err != nil {
		return CommandResult{}, err
	}
	return result(r), nil
}

func (s *Service) Assess(ctx context.Context, trialID string, c AssessmentCommand) (CommandResult, error) {
	if err := validateMeta(c.CommandMeta, false); err != nil {
		return CommandResult{}, err
	}
	fp := fingerprint("assess", c)
	r, err := s.repo.Transact(ctx, trialID, c.RequestID, fp, c.ExpectedRevision, func(t *domain.ClearanceTrial) (store.Mutation, error) {
		if c.ActorID != t.LeadObserverID {
			return store.Mutation{}, domain.Validation("actor_id", "阈值判定 actor_id 必须是冻结的责任人员")
		}
		a, err := t.Assess(newID("assessment"), s.now())
		if err != nil {
			return store.Mutation{}, err
		}
		resp := CommandResponse{TrialID: trialID, Status: t.Status, Revision: c.ExpectedRevision + 1, ResourceID: a.AssessmentID}
		return store.Mutation{EventType: "threshold_assessed", ActorID: c.ActorID, PayloadDigest: domain.AssessmentAuditPayload(*a), Response: resp}, nil
	})
	if err != nil {
		return CommandResult{}, err
	}
	return result(r), nil
}

func (s *Service) VerifyRecovery(ctx context.Context, trialID string, c RecoveryCommand) (CommandResult, error) {
	if err := validateMeta(c.CommandMeta, false); err != nil {
		return CommandResult{}, err
	}
	fp := fingerprint("verify_recovery", c)
	r, err := s.repo.Transact(ctx, trialID, c.RequestID, fp, c.ExpectedRevision, func(t *domain.ClearanceTrial) (store.Mutation, error) {
		executor := c.IsolationExecutorID
		if executor == "" {
			executor = c.ActorID
			if len(t.RecoveryAttempts) > 0 {
				executor = t.RecoveryAttempts[0].IsolationExecutorID
			}
		}
		record := domain.RecoveryRecord{AttemptID: stableID("recovery", trialID+"\x00"+c.RequestID), IsolationMeasures: c.IsolationMeasures, IsolationExecutorID: executor, MeasureCompletedAt: c.MeasureCompletedAt, Samples: c.Samples, ObserverID: c.ActorID}
		if err := t.VerifyRecovery(record, s.now()); err != nil {
			return store.Mutation{}, err
		}
		latest := t.RecoveryAttempts[len(t.RecoveryAttempts)-1]
		resp := CommandResponse{TrialID: trialID, Status: t.Status, Revision: c.ExpectedRevision + 1, ResourceID: latest.AttemptID, RecoveryAttempt: &latest}
		event := "recovery_attempt_failed"
		if latest.Passed {
			event = "recovery_window_verified"
		}
		return store.Mutation{EventType: event, ActorID: c.ActorID, PayloadDigest: domain.RecoveryAuditPayload(latest), Response: resp}, nil
	})
	if err != nil {
		return CommandResult{}, err
	}
	return result(r), nil
}

func (s *Service) Review(ctx context.Context, trialID string, c ReviewCommand) (CommandResult, error) {
	if err := validateMeta(c.CommandMeta, false); err != nil {
		return CommandResult{}, err
	}
	fp := fingerprint("review", c)
	r, err := s.repo.Transact(ctx, trialID, c.RequestID, fp, c.ExpectedRevision, func(t *domain.ClearanceTrial) (store.Mutation, error) {
		permitID := ""
		if c.Approved {
			permitID = newID("permit")
		}
		in := domain.ReviewInput{ReviewerID: c.ActorID, Approved: c.Approved, Checks: c.Checks, RejectionReasons: c.RejectionReasons, MaxConcurrentVisitors: c.MaxConcurrentVisitors, MaxStayMinutes: c.MaxStayMinutes, ValidFrom: c.ValidFrom, ValidUntil: c.ValidUntil, PermitID: permitID, Now: s.now()}
		if err := t.ReviewTrial(in); err != nil {
			return store.Mutation{}, err
		}
		resp := CommandResponse{TrialID: trialID, Status: t.Status, Revision: c.ExpectedRevision + 1, ResourceID: permitID, Reconciliation: &t.Review.Reconciliation}
		event := "review_rejected"
		if c.Approved {
			event = "permit_issued"
		}
		return store.Mutation{EventType: event, ActorID: c.ActorID, PayloadDigest: domain.TerminalAuditPayload(t), Response: resp}, nil
	})
	if err != nil {
		return CommandResult{}, err
	}
	return result(r), nil
}

func DecodeResponse(raw []byte) (CommandResponse, error) {
	var r CommandResponse
	err := json.Unmarshal(raw, &r)
	return r, err
}
