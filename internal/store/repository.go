package store

import (
	"context"
	"errors"

	"cave-microclimate-clearance/internal/domain"
)

var ErrNotFound = errors.New("trial not found")

type Mutation struct {
	EventType     string
	ActorID       string
	PayloadDigest string
	Response      any
}

type TransactionResult struct {
	Trial        *domain.ClearanceTrial
	ResponseJSON []byte
	Replayed     bool
}

type Repository interface {
	Create(ctx context.Context, trial *domain.ClearanceTrial, requestID, fingerprint, eventPayloadDigest, actorID string, response any) (TransactionResult, error)
	Transact(ctx context.Context, trialID, requestID, fingerprint string, expectedRevision int64, fn func(*domain.ClearanceTrial) (Mutation, error)) (TransactionResult, error)
	Get(ctx context.Context, trialID string) (*domain.ClearanceTrial, error)
	Timeline(ctx context.Context, trialID string) ([]domain.AuditEvent, error)
	Verify(ctx context.Context, trialID string) error
	SetPermitEvidenceDigest(ctx context.Context, trialID, digest string) error
}

type RevisionConflict struct{ Expected, Actual int64 }

func (e *RevisionConflict) Error() string { return "expected_revision 与当前修订不一致" }

type IdempotencyConflict struct{}

func (e *IdempotencyConflict) Error() string { return "request_id 已用于不同请求载荷" }

type CorruptError struct{ Reason string }

func (e *CorruptError) Error() string { return "持久化数据损坏: " + e.Reason }
