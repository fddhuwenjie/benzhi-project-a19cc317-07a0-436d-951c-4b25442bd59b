package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"cave-microclimate-clearance/internal/domain"
	"cave-microclimate-clearance/internal/store"
)

func ReconcileSemantics(t domain.ClearanceTrial, events []domain.AuditEvent) SemanticReconciliation {
	expected := domain.ExpectedAuditSequence(&t)
	result := SemanticReconciliation{Valid: true, Issues: []SemanticIssue{}}
	expectedCounts, actualCounts := map[string]int{}, map[string]int{}
	for _, event := range expected {
		expectedCounts[event.EventType]++
	}
	for _, event := range events {
		actualCounts[event.EventType]++
	}
	allTypes := map[string]bool{}
	for eventType := range expectedCounts {
		allTypes[eventType] = true
	}
	for eventType := range actualCounts {
		allTypes[eventType] = true
	}
	types := make([]string, 0, len(allTypes))
	for eventType := range allTypes {
		types = append(types, eventType)
	}
	sort.Strings(types)
	for _, eventType := range types {
		result.EventCounts = append(result.EventCounts, SemanticEventCount{EventType: eventType, Expected: expectedCounts[eventType], Actual: actualCounts[eventType]})
	}
	for expectedIndex, actualIndex := 0, 0; expectedIndex < len(expected) || actualIndex < len(events); {
		if expectedIndex >= len(expected) {
			actual := events[actualIndex]
			result.Issues = append(result.Issues, SemanticIssue{Code: "unexpected_event", EventSequence: actual.Sequence, FactReference: "audit_events", ActualEventType: actual.EventType, ActualActorID: actual.ActorID, Message: "审计时间线包含冻结聚合无法对应的额外事件"})
			actualIndex++
			continue
		}
		exp := expected[expectedIndex]
		if actualIndex >= len(events) {
			result.Issues = append(result.Issues, SemanticIssue{Code: "missing_event", EventSequence: int64(actualIndex + 1), FactReference: exp.FactReference, ExpectedEventType: exp.EventType, ExpectedActorID: exp.ActorID, Message: "冻结业务事实缺少对应审计事件"})
			expectedIndex++
			continue
		}
		actual := events[actualIndex]
		if actual.EventType != exp.EventType {
			found := false
			for i := actualIndex + 1; i < len(events); i++ {
				if events[i].EventType == exp.EventType {
					found = true
					break
				}
			}
			if !found {
				result.Issues = append(result.Issues, SemanticIssue{Code: "missing_event", EventSequence: int64(actualIndex + 1), FactReference: exp.FactReference, ExpectedEventType: exp.EventType, ActualEventType: actual.EventType, ExpectedActorID: exp.ActorID, ActualActorID: actual.ActorID, Message: "冻结业务事实缺少对应类型的审计事件"})
				expectedIndex++
				continue
			}
			result.Issues = append(result.Issues, SemanticIssue{Code: "event_out_of_order", EventSequence: actual.Sequence, FactReference: exp.FactReference, ExpectedEventType: exp.EventType, ActualEventType: actual.EventType, ExpectedActorID: exp.ActorID, ActualActorID: actual.ActorID, Message: "审计事件顺序与冻结业务流程不一致"})
			actualIndex++
			continue
		}
		if actual.ActorID != exp.ActorID {
			result.Issues = append(result.Issues, SemanticIssue{Code: "actor_identity_mismatch", EventSequence: actual.Sequence, FactReference: exp.FactReference, ExpectedEventType: exp.EventType, ActualEventType: actual.EventType, ExpectedActorID: exp.ActorID, ActualActorID: actual.ActorID, Message: "事件操作者与冻结业务身份不一致"})
		}
		if exp.PayloadDigest != "" && actual.PayloadDigest != exp.PayloadDigest {
			result.Issues = append(result.Issues, SemanticIssue{Code: "fact_digest_mismatch", EventSequence: actual.Sequence, FactReference: exp.FactReference, ExpectedEventType: exp.EventType, ActualEventType: actual.EventType, Message: "事件载荷摘要与冻结资源事实不一致"})
		}
		if exp.PayloadDigest == "" && actual.PayloadDigest == "" {
			result.Issues = append(result.Issues, SemanticIssue{Code: "fact_digest_missing", EventSequence: actual.Sequence, FactReference: exp.FactReference, ExpectedEventType: exp.EventType, ActualEventType: actual.EventType, Message: "事件未冻结业务载荷摘要"})
		}
		expectedIndex++
		actualIndex++
	}
	result.Valid = len(result.Issues) == 0
	return result
}

func eventHash(e domain.AuditEvent) string {
	m := struct {
		EventID        string `json:"event_id"`
		TrialID        string `json:"trial_id"`
		Sequence       int64  `json:"sequence"`
		EventType      string `json:"event_type"`
		ActorID        string `json:"actor_id"`
		RequestID      string `json:"request_id"`
		PayloadDigest  string `json:"payload_digest"`
		PreviousDigest string `json:"previous_digest"`
		OccurredAt     string `json:"occurred_at"`
	}{e.EventID, e.TrialID, e.Sequence, e.EventType, e.ActorID, e.RequestID, e.PayloadDigest, e.PreviousDigest, e.OccurredAt.UTC().Format("2006-01-02T15:04:05.000000000Z")}
	b, _ := json.Marshal(m)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func verifyChain(id string, events []domain.AuditEvent) bool {
	prev := ""
	for i, e := range events {
		if e.TrialID != id || e.Sequence != int64(i+1) || e.PreviousDigest != prev || eventHash(e) != e.EventDigest {
			return false
		}
		prev = e.EventDigest
	}
	return len(events) > 0
}

func VerifyPackage(p Package) Verification {
	v := Verification{TrialID: p.TrialID, ContentDigestMatches: false, AuditChainValid: false, TerminalDigestMatches: false, Problems: []string{}}
	if p.FormatVersion != FormatVersion {
		v.Problems = append(v.Problems, "证据包格式版本不受支持")
	}
	if p.Trial.TrialID != p.TrialID {
		v.Problems = append(v.Problems, "试验身份字段漂移")
	}
	v.AuditChainValid = verifyChain(p.TrialID, p.AuditEvents) && store.ChainDigest(p.AuditEvents) == p.AuditChainDigest
	if !v.AuditChainValid {
		v.Problems = append(v.Problems, "审计事件缺失或摘要链无效")
	}
	v.ContentDigestMatches = calculate(p.Trial, p.AuditEvents) == p.ContentDigest
	if !v.ContentDigestMatches {
		v.Problems = append(v.Problems, "证据内容摘要不匹配")
	}
	if p.Trial.Status == domain.StatusPermitted && p.Trial.Permit != nil {
		v.TerminalDigestMatches = p.Trial.Permit.EvidenceDigest == p.ContentDigest
	} else if p.Trial.Status == domain.StatusRejected {
		v.TerminalDigestMatches = p.Trial.RejectionEvidenceDigest == p.ContentDigest
	} else {
		v.Problems = append(v.Problems, "证据包不是冻结终态")
	}
	if !v.TerminalDigestMatches {
		v.Problems = append(v.Problems, "终态证据摘要锚点不一致")
	}
	v.SemanticReconciliation = ReconcileSemantics(p.Trial, p.AuditEvents)
	if !v.SemanticReconciliation.Valid {
		v.Problems = append(v.Problems, "审计事件与冻结业务事实语义对账失败")
	}
	v.Valid = v.ContentDigestMatches && v.AuditChainValid && v.TerminalDigestMatches && v.SemanticReconciliation.Valid && len(v.Problems) == 0
	return v
}

func (s *Service) VerifyCurrent(ctx context.Context, id string) (Verification, error) {
	p, err := s.Build(ctx, id)
	if err != nil {
		return Verification{}, fmt.Errorf("构建待校验证据包: %w", err)
	}
	return VerifyPackage(p), nil
}
