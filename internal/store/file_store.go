package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"cave-microclimate-clearance/internal/domain"
)

type requestRecord struct {
	Fingerprint  string `json:"fingerprint"`
	ResponseJSON []byte `json:"response_json"`
	Revision     int64  `json:"revision"`
}

type envelope struct {
	FormatVersion  int                      `json:"format_version"`
	Trial          domain.ClearanceTrial    `json:"trial"`
	Requests       map[string]requestRecord `json:"requests"`
	Events         []domain.AuditEvent      `json:"events"`
	SnapshotDigest string                   `json:"snapshot_digest"`
}

type FileStore struct {
	root  string
	mu    sync.Mutex
	locks map[string]*sync.Mutex
	now   func() time.Time
}

var safeID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func NewFileStore(root string) (*FileStore, error) {
	if root == "" {
		return nil, errors.New("数据目录不能为空")
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, fmt.Errorf("创建数据目录: %w", err)
	}
	return &FileStore{root: root, locks: map[string]*sync.Mutex{}, now: time.Now}, nil
}

func (s *FileStore) lock(id string) (*sync.Mutex, error) {
	if !safeID.MatchString(id) {
		return nil, domain.Validation("trial_id", "trial_id 格式无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	l := s.locks[id]
	if l == nil {
		l = &sync.Mutex{}
		s.locks[id] = l
	}
	return l, nil
}

func (s *FileStore) path(id string) string { return filepath.Join(s.root, id+".json") }

func snapshotDigest(t domain.ClearanceTrial) string {
	b, _ := json.Marshal(t)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func (s *FileStore) load(id string) (*envelope, error) {
	b, err := os.ReadFile(s.path(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var e envelope
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, &CorruptError{"JSON 截断或格式错误"}
	}
	if e.FormatVersion != 1 || e.Trial.TrialID != id {
		return nil, &CorruptError{"快照版本或身份无效"}
	}
	if e.Trial.Revision != int64(len(e.Events)) {
		return nil, &CorruptError{"快照修订与事件数量不一致"}
	}
	if snapshotDigest(e.Trial) != e.SnapshotDigest {
		return nil, &CorruptError{"快照摘要不匹配"}
	}
	if err := domain.ValidateIntegrity(&e.Trial); err != nil {
		return nil, &CorruptError{"聚合内部不变量校验失败: " + err.Error()}
	}
	if err := verifyEvents(id, e.Events); err != nil {
		return nil, err
	}
	if e.Trial.Terminal() {
		expected := domain.EvidenceContentDigest(e.Trial, e.Events)
		if e.Trial.Status == domain.StatusPermitted && (e.Trial.Permit == nil || e.Trial.Permit.EvidenceDigest != expected) {
			return nil, &CorruptError{"许可终态证据摘要锚点不匹配"}
		}
		if e.Trial.Status == domain.StatusRejected && e.Trial.RejectionEvidenceDigest != expected {
			return nil, &CorruptError{"拒绝终态证据摘要锚点不匹配"}
		}
	}
	if e.Requests == nil {
		return nil, &CorruptError{"幂等索引缺失"}
	}
	return &e, nil
}

func (s *FileStore) save(id string, e *envelope) error {
	e.FormatVersion = 1
	e.SnapshotDigest = snapshotDigest(e.Trial)
	b, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.root, ".trial-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	ok := false
	defer func() {
		tmp.Close()
		if !ok {
			os.Remove(name)
		}
	}()
	if err := tmp.Chmod(0o640); err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, s.path(id)); err != nil {
		return err
	}
	d, err := os.Open(s.root)
	if err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	ok = true
	return nil
}

func marshalResponse(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("序列化幂等响应: %w", err)
	}
	return b, nil
}

func appendEvent(e *envelope, eventType, actor, requestID, payload string, now time.Time) {
	seq := int64(len(e.Events) + 1)
	prev := chainDigest(e.Events)
	ev := domain.AuditEvent{EventID: fmt.Sprintf("evt-%s-%06d", e.Trial.TrialID, seq), TrialID: e.Trial.TrialID, Sequence: seq, EventType: eventType, ActorID: actor, RequestID: requestID, PayloadDigest: payload, PreviousDigest: prev, OccurredAt: now.UTC()}
	ev.EventDigest = eventDigest(ev)
	e.Events = append(e.Events, ev)
	e.Trial.Revision = seq
}

func (s *FileStore) Create(ctx context.Context, trial *domain.ClearanceTrial, requestID, fingerprint, eventPayloadDigest, actorID string, response any) (TransactionResult, error) {
	if err := ctx.Err(); err != nil {
		return TransactionResult{}, err
	}
	l, err := s.lock(trial.TrialID)
	if err != nil {
		return TransactionResult{}, err
	}
	l.Lock()
	defer l.Unlock()
	if existing, err := s.load(trial.TrialID); err == nil {
		r, ok := existing.Requests[requestID]
		if ok && r.Fingerprint == fingerprint {
			copy := existing.Trial
			return TransactionResult{&copy, append([]byte(nil), r.ResponseJSON...), true}, nil
		}
		if ok {
			return TransactionResult{}, &IdempotencyConflict{}
		}
		return TransactionResult{}, domain.InvalidState("trial_id 已存在")
	} else if !errors.Is(err, ErrNotFound) {
		return TransactionResult{}, err
	}
	b, err := marshalResponse(response)
	if err != nil {
		return TransactionResult{}, err
	}
	e := &envelope{Trial: *trial, Requests: map[string]requestRecord{}, Events: []domain.AuditEvent{}}
	appendEvent(e, "trial_created_baseline_frozen", actorID, requestID, eventPayloadDigest, trial.CreatedAt)
	e.Requests[requestID] = requestRecord{Fingerprint: fingerprint, ResponseJSON: b, Revision: e.Trial.Revision}
	if err := s.save(trial.TrialID, e); err != nil {
		return TransactionResult{}, err
	}
	copy := e.Trial
	return TransactionResult{&copy, b, false}, nil
}

func (s *FileStore) Transact(ctx context.Context, id, requestID, fingerprint string, expected int64, fn func(*domain.ClearanceTrial) (Mutation, error)) (TransactionResult, error) {
	if err := ctx.Err(); err != nil {
		return TransactionResult{}, err
	}
	l, err := s.lock(id)
	if err != nil {
		return TransactionResult{}, err
	}
	l.Lock()
	defer l.Unlock()
	e, err := s.load(id)
	if err != nil {
		return TransactionResult{}, err
	}
	if old, ok := e.Requests[requestID]; ok {
		if old.Fingerprint != fingerprint {
			return TransactionResult{}, &IdempotencyConflict{}
		}
		copy := e.Trial
		return TransactionResult{&copy, append([]byte(nil), old.ResponseJSON...), true}, nil
	}
	if expected != e.Trial.Revision {
		return TransactionResult{}, &RevisionConflict{expected, e.Trial.Revision}
	}
	if e.Trial.Terminal() {
		return TransactionResult{}, domain.InvalidState("终态试验禁止继续修改")
	}
	copy := e.Trial
	m, err := fn(&copy)
	if err != nil {
		return TransactionResult{}, err
	}
	b, err := marshalResponse(m.Response)
	if err != nil {
		return TransactionResult{}, err
	}
	e.Trial = copy
	appendEvent(e, m.EventType, m.ActorID, requestID, m.PayloadDigest, s.now())
	if e.Trial.Status == domain.StatusPermitted && e.Trial.Permit != nil {
		e.Trial.Permit.EvidenceDigest = domain.EvidenceContentDigest(e.Trial, e.Events)
	}
	if e.Trial.Status == domain.StatusRejected {
		e.Trial.RejectionEvidenceDigest = domain.EvidenceContentDigest(e.Trial, e.Events)
	}
	e.Requests[requestID] = requestRecord{Fingerprint: fingerprint, ResponseJSON: b, Revision: e.Trial.Revision}
	if err := s.save(id, e); err != nil {
		return TransactionResult{}, err
	}
	result := e.Trial
	return TransactionResult{&result, b, false}, nil
}

func (s *FileStore) Get(ctx context.Context, id string) (*domain.ClearanceTrial, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	l, err := s.lock(id)
	if err != nil {
		return nil, err
	}
	l.Lock()
	defer l.Unlock()
	e, err := s.load(id)
	if err != nil {
		return nil, err
	}
	copy := e.Trial
	return &copy, nil
}

func (s *FileStore) Timeline(ctx context.Context, id string) ([]domain.AuditEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	l, err := s.lock(id)
	if err != nil {
		return nil, err
	}
	l.Lock()
	defer l.Unlock()
	e, err := s.load(id)
	if err != nil {
		return nil, err
	}
	return append([]domain.AuditEvent(nil), e.Events...), nil
}

func (s *FileStore) Verify(ctx context.Context, id string) error {
	_, err := s.Get(ctx, id)
	return err
}

func (s *FileStore) SetPermitEvidenceDigest(ctx context.Context, id, digest string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	l, err := s.lock(id)
	if err != nil {
		return err
	}
	l.Lock()
	defer l.Unlock()
	e, err := s.load(id)
	if err != nil {
		return err
	}
	if e.Trial.Status != domain.StatusPermitted || e.Trial.Permit == nil {
		return domain.InvalidState("仅已许可试验可设置证据摘要")
	}
	if e.Trial.Permit.EvidenceDigest != "" && e.Trial.Permit.EvidenceDigest != digest {
		return domain.InvalidState("终态证据摘要已冻结")
	}
	e.Trial.Permit.EvidenceDigest = digest
	return s.save(id, e)
}
