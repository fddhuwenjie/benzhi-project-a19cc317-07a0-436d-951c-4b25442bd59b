package application

import (
	"context"
	"errors"
	"fmt"

	"cave-microclimate-clearance/internal/domain"
)

type TrialDetail struct {
	Trial    *domain.ClearanceTrial `json:"trial"`
	Writable bool                   `json:"writable"`
}
type Timeline struct {
	TrialID string              `json:"trial_id"`
	Events  []domain.AuditEvent `json:"events"`
	Valid   bool                `json:"valid"`
}

func (s *Service) GetTrial(ctx context.Context, id string) (TrialDetail, error) {
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return TrialDetail{}, fmt.Errorf("读取试验详情失败: %v", err)
		}
		return TrialDetail{}, err
	}
	return TrialDetail{Trial: t, Writable: !t.Terminal()}, nil
}

func (s *Service) GetTimeline(ctx context.Context, id string) (Timeline, error) {
	events, err := s.repo.Timeline(ctx, id)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return Timeline{}, fmt.Errorf("读取审计时间线失败: %v", err)
		}
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
