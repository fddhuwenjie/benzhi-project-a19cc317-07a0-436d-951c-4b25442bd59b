package evidence_error_chain_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cave-microclimate-clearance/internal/application"
	"cave-microclimate-clearance/internal/domain"
	"cave-microclimate-clearance/internal/evidence"
	"cave-microclimate-clearance/internal/httpapi"
	"cave-microclimate-clearance/internal/store"
)

type failureStage string

const (
	failVerify   failureStage = "verify"
	failFirstGet failureStage = "first-get"
	failTimeline failureStage = "timeline"
	failSet      failureStage = "set-digest"
	failFinalGet failureStage = "final-get"
)

type corruptingRepository struct {
	stage    failureStage
	getCalls int
	trial    domain.ClearanceTrial
}

func newCorruptingRepository(stage failureStage) *corruptingRepository {
	finalized := time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC)
	return &corruptingRepository{
		stage: stage,
		trial: domain.ClearanceTrial{
			TrialID:     "terminal-trial",
			Status:      domain.StatusPermitted,
			FinalizedAt: &finalized,
			Permit:      &domain.ClearancePermit{PermitID: "permit-1"},
		},
	}
}

func corrupt() error { return &store.CorruptError{Reason: "受控介质损坏"} }

func (r *corruptingRepository) Create(context.Context, *domain.ClearanceTrial, string, string, string, string, any) (store.TransactionResult, error) {
	panic("unexpected Create")
}

func (r *corruptingRepository) Transact(context.Context, string, string, string, int64, func(*domain.ClearanceTrial) (store.Mutation, error)) (store.TransactionResult, error) {
	panic("unexpected Transact")
}

func (r *corruptingRepository) Verify(context.Context, string) error {
	if r.stage == failVerify {
		return corrupt()
	}
	return nil
}

func (r *corruptingRepository) Get(context.Context, string) (*domain.ClearanceTrial, error) {
	r.getCalls++
	if (r.stage == failFirstGet && r.getCalls == 1) || (r.stage == failFinalGet && r.getCalls == 2) {
		return nil, corrupt()
	}
	trial := r.trial
	return &trial, nil
}

func (r *corruptingRepository) Timeline(context.Context, string) ([]domain.AuditEvent, error) {
	if r.stage == failTimeline {
		return nil, corrupt()
	}
	return []domain.AuditEvent{}, nil
}

func (r *corruptingRepository) SetPermitEvidenceDigest(_ context.Context, _ string, digest string) error {
	if r.stage == failSet {
		return corrupt()
	}
	r.trial.Permit.EvidenceDigest = digest
	return nil
}

func requestProblem(t *testing.T, stage failureStage, path string) (int, string) {
	t.Helper()
	repo := newCorruptingRepository(stage)
	api := httpapi.New(application.NewService(repo), evidence.NewService(repo))
	recorder := httptest.NewRecorder()
	api.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是问题 JSON: %v", err)
	}
	return recorder.Code, body.Code
}

func TestEvidenceRepositoryErrorsRetainStorageClassification(t *testing.T) {
	cases := []struct {
		name  string
		stage failureStage
		path  string
	}{
		{"BuildVerify", failVerify, "/api/v1/trials/terminal-trial/evidence"},
		{"BuildFirstGet", failFirstGet, "/api/v1/trials/terminal-trial/evidence"},
		{"BuildTimeline", failTimeline, "/api/v1/trials/terminal-trial/evidence"},
		{"BuildSetDigest", failSet, "/api/v1/trials/terminal-trial/evidence"},
		{"BuildFinalGet", failFinalGet, "/api/v1/trials/terminal-trial/evidence"},
		{"VerifyCurrent", failVerify, "/api/v1/trials/terminal-trial/evidence/verification"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, code := requestProblem(t, tc.stage, tc.path)
			if status != http.StatusServiceUnavailable || code != "storage_corrupt" {
				t.Fatalf("仓库存储损坏必须保留错误链并映射为 503/storage_corrupt，实际为 %d/%s", status, code)
			}
		})
	}
}
