package evidence

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"

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
	repo  Repository
	mu    sync.RWMutex
	cache map[string]Package
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, cache: make(map[string]Package)}
}

func (s *Service) cached(id string) (Package, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.cache[id]
	if !ok {
		return Package{}, false
	}
	c, err := clonePackage(p)
	if err != nil {
		return Package{}, false
	}
	return c, true
}

func (s *Service) remember(id string, p Package) {
	cloned, err := clonePackage(p)
	if err != nil {
		// 一份已序列化成功并落盘的证据包必然可被 JSON 复制；
		// 若克隆失败说明证据包本身已不可用，不写入缓存以免污染后续构建。
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache[id] = cloned
}

// clonePackage 返回证据包的深度副本。
// Package 内部的 Trial（含 RejectionReasons、Observations、Review、Permit 等
// 引用类型字段）与 AuditEvents 切片共享底层数组，调用方修改返回值会直接污染
// 缓存。这里通过 JSON 序列化/反序列化完整复制其可达对象图，确保后续构建始终返回
// 仓库冻结时的事实，证据摘要和语义对账不受其他调用方状态影响。
func clonePackage(p Package) (Package, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return Package{}, err
	}
	var c Package
	if err := json.Unmarshal(data, &c); err != nil {
		return Package{}, err
	}
	return c, nil
}

func calculate(t domain.ClearanceTrial, events []domain.AuditEvent) string {
	return domain.EvidenceContentDigest(t, events)
}

func (s *Service) Build(ctx context.Context, id string) (Package, error) {
	if err := ctx.Err(); err != nil {
		return Package{}, err
	}
	if p, ok := s.cached(id); ok {
		return p, nil
	}
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
	p := Package{FormatVersion: FormatVersion, TrialID: id, GeneratedAt: t.FinalizedAt.UTC(), Trial: *t, AuditEvents: events, AuditChainDigest: store.ChainDigest(events), ContentDigest: digest}
	s.remember(id, p)
	return p, nil
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
