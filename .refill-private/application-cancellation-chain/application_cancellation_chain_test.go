package application_cancellation_chain_test

import (
	"context"
	"errors"
	"testing"

	"cave-microclimate-clearance/internal/application"
	"cave-microclimate-clearance/internal/domain"
	"cave-microclimate-clearance/internal/store"
)

type canceledRepository struct{}

func (canceledRepository) Create(ctx context.Context, _ *domain.ClearanceTrial, _, _, _, _ string, _ any) (store.TransactionResult, error) {
	return store.TransactionResult{}, ctx.Err()
}

func (canceledRepository) Transact(ctx context.Context, _, _, _ string, _ int64, _ func(*domain.ClearanceTrial) (store.Mutation, error)) (store.TransactionResult, error) {
	return store.TransactionResult{}, ctx.Err()
}

func (canceledRepository) Get(ctx context.Context, _ string) (*domain.ClearanceTrial, error) {
	return nil, ctx.Err()
}

func (canceledRepository) Timeline(ctx context.Context, _ string) ([]domain.AuditEvent, error) {
	return nil, ctx.Err()
}

func (canceledRepository) Verify(ctx context.Context, _ string) error {
	return ctx.Err()
}

func (canceledRepository) SetPermitEvidenceDigest(ctx context.Context, _, _ string) error {
	return ctx.Err()
}

func TestApplicationOperationsRetainCancellationIdentity(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	service := application.NewService(canceledRepository{})
	meta := application.CommandMeta{RequestID: "cancel-request", ExpectedRevision: 1, ActorID: "observer"}
	operations := []struct {
		name string
		run  func() error
	}{
		{
			name: "add_observation",
			run: func() error {
				_, err := service.AddObservation(ctx, "trial-cancel", application.ObservationCommand{
					CommandMeta: meta,
					Observation: domain.LoadStageObservation{ObserverID: "observer"},
				})
				return err
			},
		},
		{
			name: "assess",
			run: func() error {
				_, err := service.Assess(ctx, "trial-cancel", application.AssessmentCommand{CommandMeta: meta})
				return err
			},
		},
		{
			name: "verify_recovery",
			run: func() error {
				_, err := service.VerifyRecovery(ctx, "trial-cancel", application.RecoveryCommand{CommandMeta: meta})
				return err
			},
		},
		{
			name: "review",
			run: func() error {
				_, err := service.Review(ctx, "trial-cancel", application.ReviewCommand{CommandMeta: meta})
				return err
			},
		},
		{
			name: "get_trial",
			run: func() error {
				_, err := service.GetTrial(ctx, "trial-cancel")
				return err
			},
		},
		{
			name: "get_timeline",
			run: func() error {
				_, err := service.GetTimeline(ctx, "trial-cancel")
				return err
			},
		},
	}

	for _, operation := range operations {
		if err := operation.run(); !errors.Is(err, context.Canceled) {
			t.Errorf("%s 丢失 context.Canceled 错误链: %v", operation.name, err)
		}
	}
}
