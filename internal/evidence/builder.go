package evidence

import (
	"bytes"
	"context"
	"encoding/json"

	"cave-microclimate-clearance/internal/domain"
	"cave-microclimate-clearance/internal/store"
)

type Repository interface {
	Get(context.Context, string) (*domain.ClearanceTrial, error)
	Timeline(context.Context, string) ([]domain.AuditEvent, error)
	Verify(context.Context, string) error
	SetPermitEvidenceDigest(context.Context, string, string) error
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func calculate(t domain.ClearanceTrial, events []domain.AuditEvent) string {
	return domain.EvidenceContentDigest(t, events)
}

func (s *Service) Build(ctx context.Context, id string) (Package, error) {
	if err := s.repo.Verify(ctx, id); err != nil {
		return Package{}, err
	}
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return Package{}, err
	}
	if !t.Terminal() {
		return Package{}, domain.InvalidState("试验尚未终结，不能生成冻结证据包")
	}
	events, err := s.repo.Timeline(ctx, id)
	if err != nil {
		return Package{}, err
	}
	digest := calculate(*t, events)
	if t.Status == domain.StatusPermitted {
		if err := s.repo.SetPermitEvidenceDigest(ctx, id, digest); err != nil {
			return Package{}, err
		}
		t, err = s.repo.Get(ctx, id)
		if err != nil {
			return Package{}, err
		}
	}
	return Package{FormatVersion: FormatVersion, TrialID: id, GeneratedAt: t.FinalizedAt.UTC(), Trial: *t, AuditEvents: events, AuditChainDigest: store.ChainDigest(events), ContentDigest: digest}, nil
}

func Marshal(p Package) ([]byte, error) { return json.MarshalIndent(p, "", "  ") }

func Unmarshal(data []byte) (Package, error) {
	var p Package
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return Package{}, err
	}
	return p, nil
}
