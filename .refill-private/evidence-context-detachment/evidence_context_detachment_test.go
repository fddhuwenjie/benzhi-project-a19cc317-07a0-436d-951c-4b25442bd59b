package evidencecontextdetachment_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"cave-microclimate-clearance/internal/domain"
	"cave-microclimate-clearance/internal/evidence"
)

var errDetachedContext = errors.New("repository received detached context")

type cancelPoint string

const (
	cancelAtVerify     cancelPoint = "Verify"
	cancelAtInitialGet cancelPoint = "initial Get"
	cancelAtTimeline   cancelPoint = "Timeline"
	cancelAtSetDigest  cancelPoint = "SetPermitEvidenceDigest"
	cancelAtFinalGet   cancelPoint = "final Get"
	cancelAtVerifyAPI  cancelPoint = "VerifyCurrent"
)

type cancellationRepository struct {
	point    cancelPoint
	cancel   context.CancelFunc
	getCalls int
	trial    domain.ClearanceTrial
}

func canceledOrDetached(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return errDetachedContext
}

func (r *cancellationRepository) Verify(ctx context.Context, _ string) error {
	if r.point == cancelAtVerify {
		r.cancel()
		return canceledOrDetached(ctx)
	}
	if r.point == cancelAtInitialGet {
		r.cancel()
	}
	if r.point == cancelAtVerifyAPI {
		return canceledOrDetached(ctx)
	}
	return nil
}

func (r *cancellationRepository) Get(ctx context.Context, _ string) (*domain.ClearanceTrial, error) {
	r.getCalls++
	if r.getCalls == 1 {
		if r.point == cancelAtInitialGet {
			return nil, canceledOrDetached(ctx)
		}
		if r.point == cancelAtTimeline {
			r.cancel()
		}
	} else if r.point == cancelAtFinalGet {
		return nil, canceledOrDetached(ctx)
	}
	trial := r.trial
	return &trial, nil
}

func (r *cancellationRepository) Timeline(ctx context.Context, _ string) ([]domain.AuditEvent, error) {
	if r.point == cancelAtTimeline {
		return nil, canceledOrDetached(ctx)
	}
	if r.point == cancelAtSetDigest {
		r.cancel()
	}
	return []domain.AuditEvent{}, nil
}

func (r *cancellationRepository) SetPermitEvidenceDigest(ctx context.Context, _, _ string) error {
	if r.point == cancelAtSetDigest {
		return canceledOrDetached(ctx)
	}
	if r.point == cancelAtFinalGet {
		r.cancel()
	}
	return nil
}

func TestEvidenceOperationsPreserveCallerCancellation(t *testing.T) {
	points := []cancelPoint{
		cancelAtVerify,
		cancelAtInitialGet,
		cancelAtTimeline,
		cancelAtSetDigest,
		cancelAtFinalGet,
		cancelAtVerifyAPI,
	}
	for _, point := range points {
		t.Run(string(point), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			finalizedAt := time.Unix(1_700_000_000, 0).UTC()
			repo := &cancellationRepository{
				point:  point,
				cancel: cancel,
				trial: domain.ClearanceTrial{
					TrialID:     "permitted-trial",
					Status:      domain.StatusPermitted,
					FinalizedAt: &finalizedAt,
				},
			}
			service := evidence.NewService(repo)
			var err error
			if point == cancelAtVerifyAPI {
				cancel()
				_, err = service.VerifyCurrent(ctx, repo.trial.TrialID)
			} else {
				_, err = service.Build(ctx, repo.trial.TrialID)
			}
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("在 %s 阶段取消后应返回 context.Canceled，实际错误: %v", point, err)
			}
		})
	}
}
