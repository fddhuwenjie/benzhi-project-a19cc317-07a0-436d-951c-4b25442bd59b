package domain

import (
	"fmt"
	"strings"
	"time"
)

const PermitDerivationRuleVersion = "safe-stage-paired-limits/v1"

type ReviewInput struct {
	ReviewerID            string
	Approved              bool
	Checks                ReviewChecks
	RejectionReasons      []RejectionReason
	MaxConcurrentVisitors int
	MaxStayMinutes        int
	ValidFrom             time.Time
	ValidUntil            time.Time
	PermitID              string
	Now                   time.Time
}

func (t *ClearanceTrial) reconcile(checks ReviewChecks, now time.Time) ReviewReconciliation {
	stagesComplete := len(t.Observations) == len(PlannedStages)
	if stagesComplete {
		for i, o := range t.Observations {
			if o.Stage != PlannedStages[i] || !o.Coverage.Complete {
				stagesComplete = false
				break
			}
		}
	}
	calibrationsValid := true
	validTo := map[string]time.Time{}
	for _, b := range t.Baseline.Sensors {
		x, err := time.Parse(time.RFC3339, b.CalibrationValidTo)
		if err != nil {
			calibrationsValid = false
		} else {
			validTo[b.SensorID] = x
		}
	}
	for _, o := range t.Observations {
		for _, s := range o.Samples {
			if end, ok := validTo[s.SensorID]; !ok || s.SampledAt.After(end) {
				calibrationsValid = false
			}
		}
	}
	assessmentVerified := t.Assessment != nil && t.Assessment.InputDigest == assessmentInputDigest(t) && t.Assessment.RuleSummary == ThresholdSummary(t.Thresholds, t.Baseline.ThresholdProfileVersion)
	recoveryVerified := t.Assessment != nil && !t.Assessment.RecoveryRequired
	if t.Assessment != nil && t.Assessment.RecoveryRequired {
		recoveryVerified = t.Recovery != nil && t.Recovery.Passed && len(t.RecoveryAttempts) > 0
	}
	values := []struct {
		code               string
		declared, computed bool
		refs               []string
	}{
		{"stages_complete", checks.StagesComplete, stagesComplete, []string{"observations", "observations[].coverage"}},
		{"calibrations_valid", checks.CalibrationsValid, calibrationsValid, []string{"baseline.sensors[].calibration_valid_to", "observations[].samples[].sampled_at"}},
		{"assessment_verified", checks.AssessmentVerified, assessmentVerified, []string{"assessment.input_digest", "assessment.rule_summary", "observations"}},
		{"recovery_verified", checks.RecoveryVerified, recoveryVerified, []string{"assessment.recovery_required", "recovery_attempts", "recovery.passed"}},
	}
	r := ReviewReconciliation{AllFactsSatisfied: true, AllDeclarationsMatch: true, ComputedAt: now.UTC()}
	for _, v := range values {
		item := ReconciliationItem{Code: v.code, Declared: v.declared, Computed: v.computed, Matched: v.declared == v.computed, EvidenceReferences: v.refs}
		if !v.computed {
			r.AllFactsSatisfied = false
		}
		if !item.Matched {
			r.AllDeclarationsMatch = false
		}
		r.Items = append(r.Items, item)
	}
	return r
}

func (t *ClearanceTrial) safeStage() (Stage, *LoadStageObservation, []MetricResult) {
	if t.Assessment == nil {
		return "", nil, nil
	}
	cutoff := len(t.Assessment.StageResults)
	if t.Assessment.FirstExceededStage != nil {
		cutoff = stageIndex(*t.Assessment.FirstExceededStage)
	}
	for i := cutoff - 1; i >= 0; i-- {
		sr := t.Assessment.StageResults[i]
		if !sr.AllPassed {
			continue
		}
		var margins []MetricResult
		for metricIndex := 0; metricIndex < 3; metricIndex++ {
			var chosen MetricResult
			set := false
			for _, sensor := range sr.Sensors {
				r := sensor.Metrics[metricIndex]
				if !set || r.Observed > chosen.Observed {
					chosen = r
					set = true
				}
			}
			if set {
				margins = append(margins, chosen)
			}
		}
		return sr.Stage, &t.Observations[i], margins
	}
	return "", nil, nil
}

func (t *ClearanceTrial) ReviewTrial(in ReviewInput) error {
	if t.Status != StatusReadyReview {
		return InvalidState("状态 %s 不允许复核", t.Status)
	}
	if strings.TrimSpace(in.ReviewerID) == "" {
		return Validation("reviewer_id", "复核人员不能为空")
	}
	if in.ReviewerID == t.LeadObserverID {
		return Validation("reviewer_id", "责任监测员不得自我复核")
	}
	for _, o := range t.Observations {
		if o.ObserverID == in.ReviewerID {
			return Validation("reviewer_id", "参与采样的人员不得复核")
		}
	}
	for _, r := range t.RecoveryAttempts {
		if r.ObserverID == in.ReviewerID || r.IsolationExecutorID == in.ReviewerID {
			return Validation("reviewer_id", "参与隔离或恢复采样的人员不得复核")
		}
	}
	reconciliation := t.reconcile(in.Checks, in.Now)
	if in.Approved {
		for _, item := range reconciliation.Items {
			if !item.Matched {
				return ValidationDetails("checks."+item.Code, item, "复核声明与冻结事实不一致：检查项 %s，声明值 %t，计算值 %t，证据 %v", item.Code, item.Declared, item.Computed, item.EvidenceReferences)
			}
		}
		if !reconciliation.AllFactsSatisfied {
			return Validation("checks", "冻结事实未全部满足，不能批准")
		}
		if len(in.RejectionReasons) != 0 {
			return Validation("rejection_reasons", "批准时不得携带拒绝原因")
		}
		if in.MaxConcurrentVisitors <= 0 || in.MaxStayMinutes <= 0 {
			return Validation("permit", "许可人数和停留时长必须为正数")
		}
		if in.ValidFrom.Before(in.Now.Add(-time.Minute)) || !in.ValidUntil.After(in.ValidFrom) {
			return Validation("valid_until", "许可有效期无效")
		}
		basis, observation, margins := t.safeStage()
		if observation == nil {
			return Validation("permit", "不存在全部指标通过的安全阶段，只能拒绝")
		}
		if in.MaxConcurrentVisitors > observation.VisitorCount || in.MaxStayMinutes > observation.DurationMinutes {
			return ValidationDetails("permit", map[string]any{"basis_stage": basis, "max_concurrent_visitors": observation.VisitorCount, "max_stay_minutes": observation.DurationMinutes, "requested_concurrent_visitors": in.MaxConcurrentVisitors, "requested_stay_minutes": in.MaxStayMinutes}, "许可上限必须成对取自安全阶段 %s：最多 %d 人、停留 %d 分钟；申请为 %d 人、%d 分钟", basis, observation.VisitorCount, observation.DurationMinutes, in.MaxConcurrentVisitors, in.MaxStayMinutes)
		}
		t.Permit = &ClearancePermit{PermitID: in.PermitID, ReviewerID: in.ReviewerID, MaxConcurrentVisitors: in.MaxConcurrentVisitors, MaxStayMinutes: in.MaxStayMinutes, StopThresholds: t.Thresholds, BasisStage: basis, BasisSafetyMargins: margins, DerivationRuleVersion: PermitDerivationRuleVersion, ValidFrom: in.ValidFrom.UTC(), ValidUntil: in.ValidUntil.UTC()}
		t.Status = StatusPermitted
	} else {
		if len(in.RejectionReasons) == 0 {
			return Validation("rejection_reasons", "拒绝必须提供结构化原因")
		}
		for i, x := range in.RejectionReasons {
			if strings.TrimSpace(x.Code) == "" || strings.TrimSpace(x.Detail) == "" {
				return Validation("rejection_reasons", fmt.Sprintf("第 %d 个拒绝原因必须包含非空 code 和 detail", i+1))
			}
		}
		t.RejectionReasons = append([]RejectionReason(nil), in.RejectionReasons...)
		t.Status = StatusRejected
	}
	t.Review = &ReviewDecision{ReviewerID: in.ReviewerID, Approved: in.Approved, Checks: in.Checks, Reconciliation: reconciliation, RejectionReasons: append([]RejectionReason(nil), in.RejectionReasons...), ReviewedAt: in.Now.UTC()}
	final := in.Now.UTC()
	t.FinalizedAt = &final
	return nil
}
