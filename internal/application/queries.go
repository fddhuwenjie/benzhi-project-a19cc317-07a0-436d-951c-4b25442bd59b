package application

import (
	"context"

	"cave-microclimate-clearance/internal/domain"
)

type TrialDetail struct {
	Trial    *domain.ClearanceTrial `json:"trial"`
	Writable bool                   `json:"writable"`
}

type trialQuery struct {
	detail TrialDetail
	err    error
}

type Timeline struct {
	TrialID string              `json:"trial_id"`
	Events  []domain.AuditEvent `json:"events"`
	Valid   bool                `json:"valid"`
}

func (s *Service) GetTrial(ctx context.Context, id string) (TrialDetail, error) {
	s.queryMu.Lock()
	if s.currentQuery != nil {
		current := s.currentQuery
		s.queryMu.Unlock()
		return current.detail, current.err
	}
	s.queryMu.Unlock()

	t, err := s.repo.Get(ctx, id)
	current := &trialQuery{err: err}
	if err == nil {
		current.detail = TrialDetail{Trial: t, Writable: !t.Terminal()}
	}

	s.queryMu.Lock()
	s.currentQuery = current
	s.queryMu.Unlock()
	return current.detail, current.err
}

func (s *Service) GetTimeline(ctx context.Context, id string) (Timeline, error) {
	events, err := s.repo.Timeline(ctx, id)
	if err != nil {
		return Timeline{}, err
	}
	return Timeline{TrialID: id, Events: events, Valid: true}, nil
}

func (s *Service) Repository() interface {
	SetPermitEvidenceDigest(context.Context, string, string) error
	Get(context.Context, string) (*domain.ClearanceTrial, error)
	Timeline(context.Context, string) ([]domain.AuditEvent, error)
	Verify(context.Context, string) error
} {
	return s.repo
}
