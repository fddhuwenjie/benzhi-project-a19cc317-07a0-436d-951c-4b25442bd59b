package evidencecachealias_test

import (
	"context"
	"testing"
	"time"

	"cave-microclimate-clearance/internal/domain"
	"cave-microclimate-clearance/internal/evidence"
)

type frozenRepository struct {
	trial    domain.ClearanceTrial
	events   []domain.AuditEvent
	getCalls int
}

func (r *frozenRepository) Verify(context.Context, string) error { return nil }

func (r *frozenRepository) Get(context.Context, string) (*domain.ClearanceTrial, error) {
	r.getCalls++
	t := r.trial
	t.RejectionReasons = append([]domain.RejectionReason(nil), r.trial.RejectionReasons...)
	return &t, nil
}

func (r *frozenRepository) Timeline(context.Context, string) ([]domain.AuditEvent, error) {
	return append([]domain.AuditEvent(nil), r.events...), nil
}

func (r *frozenRepository) SetPermitEvidenceDigest(context.Context, string, string) error {
	return nil
}

func TestEvidencePackageCacheIsolatedFromCallerMutation(t *testing.T) {
	finalized := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	repo := &frozenRepository{
		trial: domain.ClearanceTrial{
			TrialID:          "cache-alias-trial",
			Status:           domain.StatusRejected,
			FinalizedAt:      &finalized,
			RejectionReasons: []domain.RejectionReason{{Code: "conservation_hold", Detail: "原始冻结结论"}},
		},
		events: []domain.AuditEvent{{TrialID: "cache-alias-trial", Sequence: 1, ActorID: "independent-reviewer"}},
	}
	service := evidence.NewService(repo)

	first, err := service.Build(context.Background(), "cache-alias-trial")
	if err != nil {
		t.Fatal(err)
	}
	first.Trial.RejectionReasons[0].Detail = "调用方篡改内容"
	first.AuditEvents[0].ActorID = "field-observer"

	second, err := service.Build(context.Background(), "cache-alias-trial")
	if err != nil {
		t.Fatal(err)
	}
	if repo.getCalls != 1 {
		t.Fatalf("第二次构建应复用终态缓存，实际读取仓库 %d 次", repo.getCalls)
	}
	if got := second.Trial.RejectionReasons[0].Detail; got != "原始冻结结论" {
		t.Fatalf("缓存返回了被调用方污染的拒绝结论: %q", got)
	}
	if got := second.AuditEvents[0].ActorID; got != "independent-reviewer" {
		t.Fatalf("缓存返回了被调用方污染的审计身份: %q", got)
	}
}
