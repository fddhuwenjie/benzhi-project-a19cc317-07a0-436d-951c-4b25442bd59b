package postcommit_evidence_recheck_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cave-microclimate-clearance/internal/application"
	"cave-microclimate-clearance/internal/domain"
	"cave-microclimate-clearance/internal/evidence"
	"cave-microclimate-clearance/internal/httpapi"
	"cave-microclimate-clearance/internal/store"
)

type cancelAfterTimelineRepository struct {
	cancel    context.CancelFunc
	committed bool
	timeline  int
}

func terminalTrial(trialID string) *domain.ClearanceTrial {
	finalizedAt := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	return &domain.ClearanceTrial{TrialID: trialID, Status: domain.StatusRejected, FinalizedAt: &finalizedAt}
}

func (r *cancelAfterTimelineRepository) Create(context.Context, *domain.ClearanceTrial, string, string, string, string, any) (store.TransactionResult, error) {
	panic("unexpected Create")
}

func (r *cancelAfterTimelineRepository) Transact(_ context.Context, trialID, _, _ string, _ int64, _ func(*domain.ClearanceTrial) (store.Mutation, error)) (store.TransactionResult, error) {
	r.committed = true
	trial := terminalTrial(trialID)
	return store.TransactionResult{
		Trial:        trial,
		ResponseJSON: []byte(`{"trial_id":"trial-postcommit","status":"rejected","revision":6}`),
	}, nil
}

func (r *cancelAfterTimelineRepository) Get(ctx context.Context, trialID string) (*domain.ClearanceTrial, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return terminalTrial(trialID), nil
}

func (r *cancelAfterTimelineRepository) Timeline(ctx context.Context, trialID string) ([]domain.AuditEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.timeline++
	if r.timeline == 1 {
		r.cancel()
	}
	return []domain.AuditEvent{{TrialID: trialID, Sequence: 1}}, nil
}

func (r *cancelAfterTimelineRepository) Verify(ctx context.Context, _ string) error {
	return ctx.Err()
}

func (r *cancelAfterTimelineRepository) SetPermitEvidenceDigest(context.Context, string, string) error {
	panic("unexpected SetPermitEvidenceDigest")
}

func TestReviewCommitIsNotReclassifiedByCanceledEvidenceRecheck(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	repo := &cancelAfterTimelineRepository{cancel: cancel}
	api := httpapi.New(application.NewService(repo), evidence.NewService(repo)).Handler()
	body := `{"request_id":"review-once","expected_revision":5,"actor_id":"independent-reviewer","approved":false,"checks":{},"rejection_reasons":[{"code":"hold","detail":"保护性暂缓"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trials/trial-postcommit/reviews", strings.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	api.ServeHTTP(response, req)

	if !repo.committed {
		t.Fatal("复核事务未进入已提交状态")
	}
	if response.Code != http.StatusOK {
		t.Fatalf("终态事务已经提交且首次证据构建完成，不应被随后取消的重复校验改写为 HTTP %d: %s", response.Code, response.Body.String())
	}
}
