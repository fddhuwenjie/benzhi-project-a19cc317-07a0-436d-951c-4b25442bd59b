package domain

import (
	"fmt"
	"time"
)

func ValidateIntegrity(t *ClearanceTrial) error {
	if t == nil {
		return corrupt("试验快照为空")
	}
	if t.TrialID == "" || t.CaveSectionID == "" || t.LeadObserverID == "" {
		return corrupt("试验身份字段缺失")
	}
	if t.Revision < 1 {
		return corrupt("快照修订必须为正数")
	}
	if !t.TestWindowEnd.After(t.TestWindowStart) || t.CreatedAt.IsZero() {
		return corrupt("试验窗口或创建时间无效")
	}
	if err := ValidateThresholds(t.Thresholds); err != nil {
		return corrupt("区段阈值不满足内部约束")
	}
	frozen, err := freezeBaseline(t.Baseline, t.TestWindowStart, t.Thresholds)
	if err != nil || digestValue(frozen) != digestValue(t.Baseline) {
		return corrupt("冻结基线原始序列或稳定性摘要不一致")
	}
	if len(t.Observations) > len(PlannedStages) {
		return corrupt("负荷阶段数量超过规划")
	}
	observers := map[string]bool{t.LeadObserverID: true}
	for i, o := range t.Observations {
		if o.Stage != PlannedStages[i] {
			return corrupt(fmt.Sprintf("第 %d 个负荷阶段顺序错误", i+1))
		}
		if o.ObserverID == "" || !o.EndedAt.After(o.StartedAt) {
			return corrupt("阶段观察人员或时间范围无效")
		}
		if o.DurationMinutes <= 0 || o.DurationMinutes > MaxDurationMinutes {
			return corrupt("阶段声明时长超出安全换算范围")
		}
		if time.Duration(o.DurationMinutes)*time.Minute < o.EndedAt.Sub(o.StartedAt) {
			return corrupt("阶段声明时长短于观测时长")
		}
		if i > 0 {
			p := t.Observations[i-1]
			if o.VisitorCount <= p.VisitorCount || o.DurationMinutes < p.DurationMinutes {
				return corrupt("阶段访客负荷未严格递进")
			}
			if o.StartedAt.Sub(p.EndedAt) < time.Duration(t.Thresholds.MinStageRestMinutes)*time.Minute {
				return corrupt("阶段间静置间隔不足")
			}
		}
		expected, err := coverageFor(t, o)
		if err != nil || digestValue(expected) != digestValue(o.Coverage) {
			return corrupt("阶段采样覆盖摘要不一致")
		}
		observers[o.ObserverID] = true
	}
	if err := validateStateShape(t); err != nil {
		return err
	}
	if t.Assessment != nil {
		if t.Assessment.RuleVersion != t.Baseline.ThresholdProfileVersion || t.Assessment.RuleSummary != ThresholdSummary(t.Thresholds, t.Baseline.ThresholdProfileVersion) || t.Assessment.InputDigest != assessmentInputDigest(t) || t.Assessment.ExposureRuleVersion != ExposureRuleVersion || t.Assessment.ExposureInputDigest != assessmentInputDigest(t) {
			return corrupt("阈值判定规则或输入摘要发生漂移")
		}
		clone := *t
		clone.Status = StatusReadyAssess
		clone.Assessment = nil
		expected, err := clone.Assess("integrity", t.Assessment.EvaluatedAt)
		if err != nil {
			return corrupt("阈值判定无法重算")
		}
		actualFacts := struct {
			Metrics   []MetricResult
			Stages    []StageThresholdResult
			Exposures []StageExposure
			Trends    []ExposureTrend
			First     *Stage
			Last      *Stage
			Stop      bool
		}{t.Assessment.MetricResults, t.Assessment.StageResults, t.Assessment.StageExposures, t.Assessment.ExposureTrends, t.Assessment.FirstExceededStage, t.Assessment.LastAllPassedStage, t.Assessment.StopRequired}
		expectedFacts := struct {
			Metrics   []MetricResult
			Stages    []StageThresholdResult
			Exposures []StageExposure
			Trends    []ExposureTrend
			First     *Stage
			Last      *Stage
			Stop      bool
		}{expected.MetricResults, expected.StageResults, expected.StageExposures, expected.ExposureTrends, expected.FirstExceededStage, expected.LastAllPassedStage, expected.StopRequired}
		if digestValue(actualFacts) != digestValue(expectedFacts) || t.Assessment.StopRequired != t.Assessment.RecoveryRequired {
			return corrupt("阈值判定派生结论发生漂移")
		}
	}
	for _, r := range t.RecoveryAttempts {
		if r.ObserverID == "" || r.MeasureCompletedAt.IsZero() || r.VerifiedAt.IsZero() {
			return corrupt("恢复尝试身份或时间缺失")
		}
		observers[r.ObserverID] = true
		observers[r.IsolationExecutorID] = true
	}
	if t.Recovery != nil {
		if len(t.RecoveryAttempts) == 0 || digestValue(*t.Recovery) != digestValue(t.RecoveryAttempts[len(t.RecoveryAttempts)-1]) {
			return corrupt("最新恢复结论与尝试历史不一致")
		}
	}
	if t.Review != nil {
		if observers[t.Review.ReviewerID] {
			return corrupt("独立复核人与现场人员未分离")
		}
		recomputed := t.reconcile(t.Review.Checks, t.Review.Reconciliation.ComputedAt)
		if digestValue(recomputed) != digestValue(t.Review.Reconciliation) {
			return corrupt("复核自动对账结果发生漂移")
		}
	}
	if t.Permit != nil {
		stage, observation, _ := t.safeStage()
		if observation == nil || stage != t.Permit.BasisStage || t.Permit.MaxConcurrentVisitors > observation.VisitorCount || t.Permit.MaxStayMinutes > observation.DurationMinutes || t.Permit.DerivationRuleVersion != PermitDerivationRuleVersion {
			return corrupt("许可安全阶段推导或成对上限无效")
		}
	}
	return nil
}

func validateStateShape(t *ClearanceTrial) error {
	switch t.Status {
	case StatusBaselineFrozen:
		if len(t.Observations) != 0 || t.Assessment != nil {
			return corrupt("基线冻结状态包含后续事实")
		}
	case StatusSampling:
		if len(t.Observations) < 1 || len(t.Observations) >= 3 || t.Assessment != nil {
			return corrupt("采样状态与阶段数量不一致")
		}
	case StatusReadyAssess:
		if len(t.Observations) != 3 || t.Assessment != nil {
			return corrupt("待判定状态缺少三级观测")
		}
	case StatusPaused:
		if t.Assessment == nil || !t.Assessment.StopRequired {
			return corrupt("暂停状态缺少停止判定")
		}
		if t.Recovery != nil && t.Recovery.Passed {
			return corrupt("暂停状态错误包含通过恢复结论")
		}
	case StatusReadyReview:
		if t.Assessment == nil || len(t.Observations) != 3 {
			return corrupt("待复核状态证据不完整")
		}
		if t.Assessment.RecoveryRequired && (t.Recovery == nil || !t.Recovery.Passed) {
			return corrupt("待复核状态缺少合格恢复证据")
		}
	case StatusPermitted:
		if t.Review == nil || !t.Review.Approved || t.Permit == nil || t.FinalizedAt == nil {
			return corrupt("许可终态字段不完整")
		}
		if len(t.RejectionReasons) != 0 {
			return corrupt("许可终态携带拒绝原因")
		}
	case StatusRejected:
		if t.Review == nil || t.Review.Approved || t.Permit != nil || t.FinalizedAt == nil || len(t.RejectionReasons) == 0 {
			return corrupt("拒绝终态字段不完整")
		}
	default:
		return corrupt("未知试验状态")
	}
	return nil
}

func corrupt(message string) error { return &Error{Code: CodeCorrupt, Message: message} }
