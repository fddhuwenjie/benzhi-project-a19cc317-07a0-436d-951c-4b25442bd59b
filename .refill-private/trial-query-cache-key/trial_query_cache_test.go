package trial_query_cache_key_test

import (
	"context"
	"errors"
	"testing"

	"cave-microclimate-clearance/internal/application"
	"cave-microclimate-clearance/internal/domain"
	"cave-microclimate-clearance/internal/store"
)

type trialRepository struct {
	trials map[string]*domain.ClearanceTrial
}

func (r *trialRepository) Get(_ context.Context, id string) (*domain.ClearanceTrial, error) {
	trial, ok := r.trials[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	copy := *trial
	return &copy, nil
}

func (r *trialRepository) Create(context.Context, *domain.ClearanceTrial, string, string, string, string, any) (store.TransactionResult, error) {
	return store.TransactionResult{}, errors.New("unexpected Create")
}

func (r *trialRepository) Transact(context.Context, string, string, string, int64, func(*domain.ClearanceTrial) (store.Mutation, error)) (store.TransactionResult, error) {
	return store.TransactionResult{}, errors.New("unexpected Transact")
}

func (r *trialRepository) Timeline(context.Context, string) ([]domain.AuditEvent, error) {
	return nil, errors.New("unexpected Timeline")
}

func (r *trialRepository) Verify(context.Context, string) error {
	return errors.New("unexpected Verify")
}

func (r *trialRepository) SetPermitEvidenceDigest(context.Context, string, string) error {
	return errors.New("unexpected SetPermitEvidenceDigest")
}

func TestTrialDetailCacheIsScopedByTrialID(t *testing.T) {
	repo := &trialRepository{trials: map[string]*domain.ClearanceTrial{
		"trial-a": {TrialID: "trial-a", Status: domain.StatusSampling},
		"trial-b": {TrialID: "trial-b", Status: domain.StatusReadyAssess},
	}}
	service := application.NewService(repo)

	first, err := service.GetTrial(context.Background(), "trial-a")
	if err != nil {
		t.Fatalf("first query failed: %v", err)
	}
	if first.Trial == nil || first.Trial.TrialID != "trial-a" {
		t.Fatalf("first query returned the wrong trial")
	}

	second, err := service.GetTrial(context.Background(), "trial-b")
	if err != nil {
		t.Fatalf("second query failed: %v", err)
	}
	if second.Trial == nil {
		t.Fatalf("second query returned an empty trial")
	}
	if second.Trial.TrialID != "trial-b" {
		t.Fatalf("query for trial-b returned cached identity %s", second.Trial.TrialID)
	}
}
