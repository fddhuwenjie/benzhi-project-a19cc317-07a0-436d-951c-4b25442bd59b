package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"sort"
	"strings"
	"time"
)

func digestValue(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func assessmentInputDigest(t *ClearanceTrial) string {
	return digestValue(struct {
		B BaselineProfile
		O []LoadStageObservation
		T Thresholds
	}{t.Baseline, t.Observations, t.Thresholds})
}

type metricCandidate struct {
	value  float64
	stage  Stage
	sensor string
	at     time.Time
}

func resultFromCandidate(name string, c metricCandidate, limit float64) MetricResult {
	observed := round3(c.value)
	margin := round3(limit - observed)
	passed := observed <= limit
	conclusion := "within_limit"
	if !passed {
		conclusion = "stop_threshold_exceeded"
	}
	return MetricResult{Metric: name, Observed: observed, Limit: limit, Passed: passed, Conclusion: conclusion, DecisiveStage: c.stage, DecisiveSensorID: c.sensor, DecisiveSampledAt: c.at.UTC(), AbsoluteMargin: margin, PercentMargin: round3(margin * 100 / limit)}
}

func maxCandidate(samples []SensorSample, stage Stage, sensor string, value func(SensorSample) float64) metricCandidate {
	c := metricCandidate{stage: stage, sensor: sensor}
	for i, s := range samples {
		v := value(s)
		if i == 0 || v > c.value {
			c.value = v
			c.at = s.SampledAt
		}
	}
	return c
}

func better(a, b metricCandidate) metricCandidate {
	if b.value > a.value {
		return b
	}
	return a
}

func (t *ClearanceTrial) Assess(id string, now time.Time) (*ThresholdAssessment, error) {
	if t.Status != StatusReadyAssess {
		return nil, InvalidState("状态 %s 不允许阈值判定", t.Status)
	}
	if len(t.Observations) != 3 {
		return nil, Validation("observations", "三级负荷观测不完整")
	}
	bases := baselineMap(t)
	var global [3]metricCandidate
	globalSet := false
	stageResults := make([]StageThresholdResult, 0, 3)
	var firstExceeded, lastAllPassed *Stage
	for _, o := range t.Observations {
		grouped := map[string][]SensorSample{}
		for _, s := range o.Samples {
			grouped[s.SensorID] = append(grouped[s.SensorID], s)
		}
		ids := make([]string, 0, len(grouped))
		for id := range grouped {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		sr := StageThresholdResult{Stage: o.Stage, AllPassed: true}
		for _, id := range ids {
			samples := grouped[id]
			base := bases[id]
			cs := [3]metricCandidate{
				maxCandidate(samples, o.Stage, id, func(s SensorSample) float64 { return s.TemperatureC - base.TemperatureC }),
				maxCandidate(samples, o.Stage, id, func(s SensorSample) float64 { return s.RelativeHumidity - base.RelativeHumidity }),
				maxCandidate(samples, o.Stage, id, func(s SensorSample) float64 { return s.CO2PPM }),
			}
			results := []MetricResult{
				resultFromCandidate("temperature_delta_c", cs[0], t.Thresholds.MaxTemperatureDeltaC),
				resultFromCandidate("relative_humidity_delta_pct", cs[1], t.Thresholds.MaxHumidityDeltaPct),
				resultFromCandidate("co2_peak_ppm", cs[2], t.Thresholds.MaxCO2PPM),
			}
			all := true
			for _, r := range results {
				if !r.Passed {
					all = false
					sr.AllPassed = false
				}
			}
			sr.Sensors = append(sr.Sensors, SensorThresholdResult{SensorID: id, Metrics: results, AllPassed: all})
			if !globalSet {
				global = cs
				globalSet = true
			} else {
				for i := range global {
					global[i] = better(global[i], cs[i])
				}
			}
		}
		stageResults = append(stageResults, sr)
		stage := o.Stage
		if sr.AllPassed {
			x := stage
			lastAllPassed = &x
		} else if firstExceeded == nil {
			x := stage
			firstExceeded = &x
		}
	}
	results := []MetricResult{
		resultFromCandidate("temperature_delta_c", global[0], t.Thresholds.MaxTemperatureDeltaC),
		resultFromCandidate("relative_humidity_delta_pct", global[1], t.Thresholds.MaxHumidityDeltaPct),
		resultFromCandidate("co2_peak_ppm", global[2], t.Thresholds.MaxCO2PPM),
	}
	stop := false
	for _, r := range results {
		if !r.Passed {
			stop = true
		}
	}
	stageExposures, exposureTrends := exposureAnalysis(t)
	inputDigest := assessmentInputDigest(t)
	a := &ThresholdAssessment{AssessmentID: id, RuleVersion: t.Baseline.ThresholdProfileVersion, RuleSummary: ThresholdSummary(t.Thresholds, t.Baseline.ThresholdProfileVersion), MetricResults: results, StageResults: stageResults, StageExposures: stageExposures, ExposureTrends: exposureTrends, ExposureRuleVersion: ExposureRuleVersion, ExposureInputDigest: inputDigest, FirstExceededStage: firstExceeded, LastAllPassedStage: lastAllPassed, StopRequired: stop, RecoveryRequired: stop, InputDigest: inputDigest, EvaluatedAt: now.UTC()}
	t.Assessment = a
	if stop {
		t.Status = StatusPaused
	} else {
		t.Status = StatusReadyReview
	}
	return a, nil
}

func normalizedMeasures(values []string) ([]string, error) {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			return nil, Validation("isolation_measures", "隔离措施不得为空")
		}
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	sort.Strings(result)
	if len(result) == 0 {
		return nil, Validation("isolation_measures", "必须记录现场隔离措施")
	}
	return result, nil
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (t *ClearanceTrial) VerifyRecovery(r RecoveryRecord, now time.Time) error {
	if t.Status != StatusPaused || t.Assessment == nil || !t.Assessment.RecoveryRequired {
		return InvalidState("当前试验不需要恢复验证")
	}
	measures, err := normalizedMeasures(r.IsolationMeasures)
	if err != nil {
		return err
	}
	if r.ObserverID == "" {
		return Validation("observer_id", "恢复观察人员不能为空")
	}
	if r.IsolationExecutorID == "" {
		return Validation("isolation_executor_id", "隔离措施执行人员不能为空")
	}
	if r.MeasureCompletedAt.IsZero() {
		return Validation("measure_completed_at", "必须提供隔离措施完成时间")
	}
	if r.MeasureCompletedAt.After(now) {
		return Validation("measure_completed_at", "隔离措施完成时间不得晚于提交时间")
	}
	if len(t.RecoveryAttempts) > 0 {
		first := t.RecoveryAttempts[0]
		if !r.MeasureCompletedAt.Equal(first.MeasureCompletedAt) || !equalStrings(measures, first.IsolationMeasures) || r.IsolationExecutorID != first.IsolationExecutorID {
			return Validation("isolation_measures", "首次冻结的隔离措施、执行人员和完成时间不得删除或替换")
		}
	}
	bases := baselineMap(t)
	grouped := map[string][]SensorSample{}
	last := map[string]time.Time{}
	for _, s := range r.Samples {
		if _, ok := bases[s.SensorID]; !ok {
			return Validation("samples.sensor_id", "恢复传感器 %s 不在基线集合", s.SensorID)
		}
		if s.SampledAt.Before(r.MeasureCompletedAt) {
			return Validation("samples.sampled_at", "恢复窗口必须从隔离措施完成后开始")
		}
		if s.SampledAt.After(now) {
			return Validation("samples.sampled_at", "恢复采样时间不得晚于提交时间")
		}
		if p, ok := last[s.SensorID]; ok && !s.SampledAt.After(p) {
			return Validation("samples.sampled_at", "传感器 %s 恢复采样时间必须严格递增", s.SensorID)
		}
		if !readingWithinPhysicalRange(s.TemperatureC, s.RelativeHumidity, s.CO2PPM) {
			return Validation("samples", "恢复读数超出物理范围")
		}
		last[s.SensorID] = s.SampledAt
		grouped[s.SensorID] = append(grouped[s.SensorID], s)
	}
	r.IsolationMeasures = measures
	r.VerifiedAt = now.UTC()
	r.Passed = true
	ids := make([]string, 0, len(bases))
	for id := range bases {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		points := grouped[id]
		base := bases[id]
		current := []SensorSample{}
		previousDistance := 0.0
		for _, s := range points {
			distance := math.Abs(s.TemperatureC-base.TemperatureC)/t.Thresholds.RecoveryTempDeltaC + math.Abs(s.RelativeHumidity-base.RelativeHumidity)/t.Thresholds.RecoveryHumidityDelta
			if base.CO2PPM < t.Thresholds.RecoveryCO2PPM {
				distance += math.Max(0, s.CO2PPM-base.CO2PPM) / (t.Thresholds.RecoveryCO2PPM - base.CO2PPM)
			}
			within := math.Abs(s.TemperatureC-base.TemperatureC) <= t.Thresholds.RecoveryTempDeltaC && math.Abs(s.RelativeHumidity-base.RelativeHumidity) <= t.Thresholds.RecoveryHumidityDelta && s.CO2PPM <= t.Thresholds.RecoveryCO2PPM
			continuous := len(current) == 0 || s.SampledAt.Sub(current[len(current)-1].SampledAt) <= time.Duration(t.Thresholds.RecoveryMaxGapSeconds)*time.Second
			improving := len(current) == 0 || distance <= previousDistance+0.000001
			if !within || !continuous || !improving {
				current = nil
			}
			if within {
				current = append(current, s)
				previousDistance = distance
			}
		}
		span := time.Duration(0)
		if len(current) > 1 {
			span = current[len(current)-1].SampledAt.Sub(current[0].SampledAt)
		}
		c := RecoverySensorConclusion{SensorID: id, ReceivedPoints: len(points), ContinuousPoints: len(current), ObservedSpanSeconds: int64(span / time.Second), Trend: "stable_or_improving", Passed: true}
		if len(current) < t.Thresholds.RecoveryPoints {
			c.Passed = false
			c.FailureReasons = append(c.FailureReasons, "continuous_points_insufficient")
		}
		if span < time.Duration(t.Thresholds.RecoveryMinMinutes)*time.Minute {
			c.Passed = false
			c.FailureReasons = append(c.FailureReasons, "continuous_span_insufficient")
		}
		if len(current) == 0 && len(points) > 0 {
			c.Trend = "interrupted_or_worsening"
			c.FailureReasons = append(c.FailureReasons, "threshold_or_trend_not_met")
			c.Passed = false
		}
		if len(points) == 0 {
			c.Trend = "missing"
			c.FailureReasons = append(c.FailureReasons, "sensor_missing")
			c.Passed = false
		}
		if !c.Passed {
			r.Passed = false
			for _, reason := range c.FailureReasons {
				r.FailureReasons = append(r.FailureReasons, id+":"+reason)
			}
		}
		r.SensorConclusions = append(r.SensorConclusions, c)
	}
	t.RecoveryAttempts = append(t.RecoveryAttempts, r)
	latest := r
	t.Recovery = &latest
	if r.Passed {
		t.Status = StatusReadyReview
	}
	return nil
}
