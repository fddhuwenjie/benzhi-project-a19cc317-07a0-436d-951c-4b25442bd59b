package domain

import (
	"errors"
	"testing"
	"time"
)

func testTrial(t *testing.T) (*ClearanceTrial, time.Time) {
	t.Helper()
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	start := now.Add(time.Hour)
	end := start.Add(2 * time.Hour)
	readings := []BaselineReading{
		{SampledAt: now.Add(-10 * time.Minute), TemperatureC: 13.9, RelativeHumidity: 69.5, CO2PPM: 490},
		{SampledAt: now.Add(-5 * time.Minute), TemperatureC: 14, RelativeHumidity: 70, CO2PPM: 500},
		{SampledAt: now, TemperatureC: 14.1, RelativeHumidity: 70.5, CO2PPM: 510},
	}
	trial, err := NewTrial(CreateInput{TrialID: "trial-test", CaveSectionID: "section-A", WindowStart: start, WindowEnd: end, LeadObserverID: "lead", Baseline: BaselineProfile{BaselineID: "base", ThresholdProfileVersion: "rules/v1", Sensors: []SensorBaseline{{SensorID: "s1", CalibrationRef: "cal-1", CalibrationValidTo: end.Add(time.Hour).Format(time.RFC3339), Readings: readings}}}, Thresholds: DefaultThresholds(), Now: now})
	if err != nil {
		t.Fatal(err)
	}
	return trial, start
}

func addThree(t *testing.T, trial *ClearanceTrial, start time.Time, exceed bool) {
	t.Helper()
	for i, stage := range PlannedStages {
		co2 := float64(700 + i*100)
		temp := 14.2 + float64(i)*.2
		rh := 71 + float64(i)
		if exceed && stage == StageHigh {
			co2 = 1400
			temp = 16
			rh = 77
		}
		offsets := []int{0, 20, 45}
		begin := start.Add(time.Duration(offsets[i]) * time.Minute)
		var samples []SensorSample
		duration := 15 + i*5
		for minute := 0; minute <= duration; minute++ {
			fraction := float64(minute) / float64(duration)
			samples = append(samples, SensorSample{SensorID: "s1", SampledAt: begin.Add(time.Duration(minute) * time.Minute), TemperatureC: temp - .1 + .1*fraction, RelativeHumidity: rh - .2 + .2*fraction, CO2PPM: co2 - 20 + 20*fraction})
		}
		o := LoadStageObservation{ObservationID: string(stage), Stage: stage, VisitorCount: (i + 1) * 3, DurationMinutes: duration, SamplingIntervalSeconds: 60, ObserverID: "sampler", StartedAt: begin, EndedAt: begin.Add(time.Duration(duration) * time.Minute), Samples: samples}
		if err := trial.AddObservation(o); err != nil {
			t.Fatalf("阶段 %s: %v", stage, err)
		}
	}
}

func TestAbnormalRecoveryApprovalWorkflow(t *testing.T) {
	trial, start := testTrial(t)
	addThree(t, trial, start, true)
	a, err := trial.Assess("assessment", start.Add(70*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if !a.StopRequired || trial.Status != StatusPaused {
		t.Fatalf("应进入暂停状态: %+v", a)
	}
	if a.MetricResults[2].DecisiveStage != StageHigh || a.LastAllPassedStage == nil || *a.LastAllPassedStage != StageMedium {
		t.Fatalf("阈值贡献或安全阶段定位错误: %+v", a)
	}
	if len(a.StageExposures) != 3 || len(a.ExposureTrends) != 3 || a.ExposureRuleVersion != ExposureRuleVersion {
		t.Fatalf("阶段暴露分析未完整冻结: %+v", a)
	}
	for _, trend := range a.ExposureTrends {
		if trend.Conclusion != "persistent_deterioration" || trend.DecisiveSensorID != "s1" {
			t.Fatalf("逐级恶化趋势或决定性来源错误: %+v", trend)
		}
	}
	samples := []SensorSample{{SensorID: "s1", SampledAt: start.Add(80 * time.Minute), TemperatureC: 14.3, RelativeHumidity: 71.5, CO2PPM: 650}, {SensorID: "s1", SampledAt: start.Add(81 * time.Minute), TemperatureC: 14.2, RelativeHumidity: 71, CO2PPM: 620}, {SensorID: "s1", SampledAt: start.Add(82 * time.Minute), TemperatureC: 14.1, RelativeHumidity: 70.5, CO2PPM: 580}}
	completedAt := start.Add(79 * time.Minute)
	failed := []SensorSample{{SensorID: "s1", SampledAt: start.Add(80 * time.Minute), TemperatureC: 15, RelativeHumidity: 75, CO2PPM: 900}}
	if err := trial.VerifyRecovery(RecoveryRecord{AttemptID: "recovery-1", ObserverID: "recovery", IsolationExecutorID: "recovery", MeasureCompletedAt: completedAt, IsolationMeasures: []string{"关闭入口", "关闭入口"}, Samples: failed}, start.Add(81*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if trial.Status != StatusPaused || trial.Recovery == nil || trial.Recovery.Passed || len(trial.RecoveryAttempts) != 1 {
		t.Fatalf("未通过恢复尝试未被保留: %+v", trial.Recovery)
	}
	if err := trial.VerifyRecovery(RecoveryRecord{AttemptID: "recovery-2", ObserverID: "recovery", IsolationExecutorID: "recovery", MeasureCompletedAt: completedAt, IsolationMeasures: []string{"关闭入口"}, Samples: samples}, start.Add(83*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := trial.ReviewTrial(ReviewInput{ReviewerID: "reviewer", Approved: true, Checks: ReviewChecks{true, true, true, true}, MaxConcurrentVisitors: 6, MaxStayMinutes: 20, ValidFrom: start.Add(90 * time.Minute), ValidUntil: start.Add(24 * time.Hour), PermitID: "permit", Now: start.Add(85 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if trial.Status != StatusPermitted || trial.Permit == nil {
		t.Fatalf("许可未签发: %+v", trial)
	}
	if trial.Permit.BasisStage != StageMedium {
		t.Fatalf("异常试验应以 medium 为许可依据: %+v", trial.Permit)
	}
	trial.Revision = 7
	if err := ValidateIntegrity(trial); err != nil {
		t.Fatal(err)
	}
}

func TestBaselineSynchronizationCoverageAndStaleness(t *testing.T) {
	now := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	start, end := now.Add(time.Hour), now.Add(3*time.Hour)
	makeReadings := func(first, last time.Time) []BaselineReading {
		return []BaselineReading{
			{SampledAt: first, TemperatureC: 14, RelativeHumidity: 70, CO2PPM: 500},
			{SampledAt: first.Add(last.Sub(first) / 2), TemperatureC: 14.1, RelativeHumidity: 70.5, CO2PPM: 510},
			{SampledAt: last, TemperatureC: 14, RelativeHumidity: 70, CO2PPM: 505},
		}
	}
	sensors := []SensorBaseline{
		{SensorID: "s1", CalibrationRef: "cal-1", CalibrationValidTo: end.Add(time.Hour).Format(time.RFC3339), Readings: makeReadings(start.Add(-12*time.Minute), start.Add(-time.Minute))},
		{SensorID: "s2", CalibrationRef: "cal-2", CalibrationValidTo: end.Add(time.Hour).Format(time.RFC3339), Readings: makeReadings(start.Add(-11*time.Minute), start.Add(-30*time.Second))},
	}
	trial, err := NewTrial(CreateInput{TrialID: "sync", CaveSectionID: "section", WindowStart: start, WindowEnd: end, LeadObserverID: "lead", Baseline: BaselineProfile{Sensors: sensors}, Thresholds: DefaultThresholds(), Now: now})
	if err != nil {
		t.Fatal(err)
	}
	sync := trial.Baseline.Synchronization
	if sync.CommonCoverageSeconds != 600 || sync.MaximumStalenessSeconds != 60 || sync.AlignmentDeviationSeconds != 30 || sync.CoverageStartDecisiveSensorID != "s2" || sync.StalenessDecisiveSensorID != "s1" {
		t.Fatalf("基线同步摘要错误: %+v", sync)
	}

	nonOverlapping := append([]SensorBaseline(nil), sensors...)
	nonOverlapping[0].Readings = makeReadings(start.Add(-40*time.Minute), start.Add(-30*time.Minute))
	nonOverlapping[1].Readings = makeReadings(start.Add(-20*time.Minute), start.Add(-10*time.Minute))
	_, err = NewTrial(CreateInput{TrialID: "no-overlap", CaveSectionID: "section", WindowStart: start, WindowEnd: end, LeadObserverID: "lead", Baseline: BaselineProfile{Sensors: nonOverlapping}, Thresholds: DefaultThresholds(), Now: now})
	var validation *Error
	if !errors.As(err, &validation) || validation.Field != "baseline.synchronization.common_coverage" || validation.Details == nil {
		t.Fatalf("无共同覆盖应返回逐传感器边界: %#v", err)
	}

	limits := DefaultThresholds()
	limits.MaxBaselineStalenessMinutes = 5
	_, err = NewTrial(CreateInput{TrialID: "stale", CaveSectionID: "section", WindowStart: start, WindowEnd: end, LeadObserverID: "lead", Baseline: BaselineProfile{Sensors: []SensorBaseline{nonOverlapping[1]}}, Thresholds: limits, Now: now})
	if !errors.As(err, &validation) || validation.Field != "baseline.synchronization.staleness" {
		t.Fatalf("陈旧基线应返回字段级错误: %#v", err)
	}
}

func TestShortCO2PeakStopsWhileExposureTrendReverses(t *testing.T) {
	trial, start := testTrial(t)
	for stageIndex, stage := range PlannedStages {
		begin := start.Add(time.Duration(stageIndex*20) * time.Minute)
		samples := make([]SensorSample, 0, 16)
		for minute := 0; minute <= 15; minute++ {
			co2 := []float64{600, 800, 500}[stageIndex]
			if stage == StageHigh && minute == 15 {
				co2 = 1400
			}
			samples = append(samples, SensorSample{SensorID: "s1", SampledAt: begin.Add(time.Duration(minute) * time.Minute), TemperatureC: 14.1, RelativeHumidity: 70, CO2PPM: co2})
		}
		err := trial.AddObservation(LoadStageObservation{ObservationID: string(stage), Stage: stage, VisitorCount: stageIndex + 1, DurationMinutes: 15, SamplingIntervalSeconds: 60, ObserverID: "sampler", StartedAt: begin, EndedAt: begin.Add(15 * time.Minute), Samples: samples})
		if err != nil {
			t.Fatal(err)
		}
	}
	assessment, err := trial.Assess("short-peak", start.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	var co2Trend ExposureTrend
	for _, trend := range assessment.ExposureTrends {
		if trend.Metric == "co2_above_baseline_ppm" {
			co2Trend = trend
		}
	}
	if !assessment.StopRequired || assessment.MetricResults[2].DecisiveStage != StageHigh || co2Trend.Conclusion != "reverse_change" || co2Trend.DecisiveSensorID != "s1" {
		t.Fatalf("短时峰值应暂停并冻结反向累计暴露提示: assessment=%+v trend=%+v", assessment, co2Trend)
	}
}

func TestRejectUnstableBaselineWithStructuredDetails(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	start := now.Add(time.Hour)
	readings := []BaselineReading{
		{SampledAt: now.Add(-10 * time.Minute), TemperatureC: 14, RelativeHumidity: 60, CO2PPM: 500},
		{SampledAt: now.Add(-5 * time.Minute), TemperatureC: 14.1, RelativeHumidity: 64, CO2PPM: 510},
		{SampledAt: now, TemperatureC: 14.2, RelativeHumidity: 68, CO2PPM: 520},
	}
	_, err := NewTrial(CreateInput{TrialID: "unstable", CaveSectionID: "section", WindowStart: start, WindowEnd: start.Add(time.Hour), LeadObserverID: "lead", Baseline: BaselineProfile{Sensors: []SensorBaseline{{SensorID: "s1", CalibrationRef: "cal", CalibrationValidTo: start.Add(2 * time.Hour).Format(time.RFC3339), Readings: readings}}}, Thresholds: DefaultThresholds(), Now: now})
	var validation *Error
	if err == nil || !errors.As(err, &validation) || validation.Field != "baseline.sensors[s1].stability.relative_humidity_pct" || validation.Details == nil {
		t.Fatalf("应返回湿度波动字段级错误: %#v", err)
	}
}

func TestCoverageGapAndReviewReconciliation(t *testing.T) {
	trial, start := testTrial(t)
	begin := start
	var samples []SensorSample
	for minute := 0; minute <= 15; minute++ {
		if minute != 7 {
			samples = append(samples, SensorSample{SensorID: "s1", SampledAt: begin.Add(time.Duration(minute) * time.Minute), TemperatureC: 14, RelativeHumidity: 70, CO2PPM: 600})
		}
	}
	err := trial.AddObservation(LoadStageObservation{Stage: StageLow, VisitorCount: 3, DurationMinutes: 15, SamplingIntervalSeconds: 60, ObserverID: "sampler", StartedAt: begin, EndedAt: begin.Add(15 * time.Minute), Samples: samples})
	if err == nil || len(trial.Observations) != 0 {
		t.Fatalf("阶段中部断采必须拒绝且不改变聚合: %v", err)
	}
	addThree(t, trial, start, false)
	if _, err := trial.Assess("assessment", start.Add(75*time.Minute)); err != nil {
		t.Fatal(err)
	}
	err = trial.ReviewTrial(ReviewInput{ReviewerID: "reviewer", Approved: true, Checks: ReviewChecks{StagesComplete: true, CalibrationsValid: true, AssessmentVerified: false, RecoveryVerified: true}, MaxConcurrentVisitors: 9, MaxStayMinutes: 25, ValidFrom: start.Add(90 * time.Minute), ValidUntil: start.Add(24 * time.Hour), PermitID: "permit", Now: start.Add(85 * time.Minute)})
	var validation *Error
	if !errors.As(err, &validation) || validation.Field != "checks.assessment_verified" || trial.Status != StatusReadyReview {
		t.Fatalf("复核对账差异应阻止批准且状态不变: %#v %+v", err, trial.Status)
	}
}

func TestRejectUnknownSensorAndWrongStage(t *testing.T) {
	trial, start := testTrial(t)
	bad := LoadStageObservation{Stage: StageMedium, VisitorCount: 2, DurationMinutes: 10, SamplingIntervalSeconds: 60, ObserverID: "sampler", StartedAt: start, EndedAt: start.Add(5 * time.Minute), Samples: []SensorSample{{SensorID: "s1", SampledAt: start, CO2PPM: 500}, {SensorID: "s1", SampledAt: start.Add(time.Minute), CO2PPM: 500}}}
	if err := trial.AddObservation(bad); err == nil {
		t.Fatal("应拒绝错误阶段顺序")
	}
	bad.Stage = StageLow
	bad.Samples[1].SensorID = "unknown"
	if err := trial.AddObservation(bad); err == nil {
		t.Fatal("应拒绝基线外传感器")
	}
}

func TestIndependentReviewerRequired(t *testing.T) {
	trial, start := testTrial(t)
	addThree(t, trial, start, false)
	if _, err := trial.Assess("assessment", start.Add(70*time.Minute)); err != nil {
		t.Fatal(err)
	}
	err := trial.ReviewTrial(ReviewInput{ReviewerID: "sampler", Approved: false, Checks: ReviewChecks{true, true, true, true}, RejectionReasons: []RejectionReason{{Code: "x", Detail: "x"}}, Now: start.Add(80 * time.Minute)})
	if err == nil {
		t.Fatal("采样人员不得复核")
	}
}
