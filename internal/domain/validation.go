package domain

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

type CreateInput struct {
	TrialID        string
	CaveSectionID  string
	WindowStart    time.Time
	WindowEnd      time.Time
	LeadObserverID string
	Baseline       BaselineProfile
	Thresholds     Thresholds
	Now            time.Time
}

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }

// readingWithinPhysicalRange 验证单次传感器读数的物理范围。温度缺失边界会使数量级极大但
// JSON 可表示的有限读数通过校验，随后在阈值判定与暴露积分的派生计算中溢出为 +Inf/-Inf，
// 导致持久化层无法编码为 JSON。这里选取的上下界覆盖任何合理的洞穴监测场景，同时保证
// round3 与梯形积分等派生运算不会溢出 MaxFloat64。
func readingWithinPhysicalRange(temperatureC, relativeHumidity, co2PPM float64) bool {
	return temperatureC >= -100 && temperatureC <= 100 && relativeHumidity >= 0 && relativeHumidity <= 100 && co2PPM > 0 && co2PPM <= 100000
}

func median(values []float64) float64 {
	x := append([]float64(nil), values...)
	sort.Float64s(x)
	m := len(x) / 2
	if len(x)%2 == 1 {
		return round3(x[m])
	}
	return round3((x[m-1] + x[m]) / 2)
}

func stableMetric(name string, values []float64, allowed float64) MetricStability {
	min, max := values[0], values[0]
	for _, v := range values[1:] {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	fluctuation := round3(max - min)
	return MetricStability{Metric: name, Median: median(values), Minimum: round3(min), Maximum: round3(max), Fluctuation: fluctuation, AllowedFluctuation: allowed, Stable: fluctuation <= allowed}
}

func freezeBaseline(b BaselineProfile, windowStart time.Time, limits Thresholds) (BaselineProfile, error) {
	if len(b.Sensors) == 0 {
		return b, Validation("baseline.sensors", "至少需要一组基线传感器")
	}
	seen := map[string]bool{}
	calibrations := map[string]bool{}
	sort.Slice(b.Sensors, func(i, j int) bool { return b.Sensors[i].SensorID < b.Sensors[j].SensorID })
	for i := range b.Sensors {
		s := &b.Sensors[i]
		field := "baseline.sensors[" + s.SensorID + "]"
		if strings.TrimSpace(s.SensorID) == "" || strings.TrimSpace(s.CalibrationRef) == "" {
			return b, Validation("baseline.sensors", "第 %d 个传感器缺少身份或校准标识", i+1)
		}
		if seen[s.SensorID] {
			return b, Validation("baseline.sensors", "传感器 %s 重复", s.SensorID)
		}
		seen[s.SensorID] = true
		if calibrations[s.CalibrationRef] {
			return b, Validation("baseline.sensors.calibration_ref", "校准标识 %s 不能对应多个传感器", s.CalibrationRef)
		}
		calibrations[s.CalibrationRef] = true
		if len(s.Readings) < limits.BaselineMinPoints {
			return b, Validation(field+".readings", "传感器 %s 基线点数 %d，至少需要 %d", s.SensorID, len(s.Readings), limits.BaselineMinPoints)
		}
		temp, humidity, co2 := make([]float64, 0, len(s.Readings)), make([]float64, 0, len(s.Readings)), make([]float64, 0, len(s.Readings))
		for j, r := range s.Readings {
			if !r.SampledAt.Before(windowStart) {
				return b, Validation(field+".readings.sampled_at", "传感器 %s 第 %d 个无人扰动读数必须早于试验窗口", s.SensorID, j+1)
			}
			if j > 0 && !r.SampledAt.After(s.Readings[j-1].SampledAt) {
				return b, Validation(field+".readings.sampled_at", "传感器 %s 基线时间必须严格递增", s.SensorID)
			}
			if !readingWithinPhysicalRange(r.TemperatureC, r.RelativeHumidity, r.CO2PPM) {
				return b, Validation(field+".readings", "传感器 %s 基线读数超出物理范围", s.SensorID)
			}
			temp, humidity, co2 = append(temp, r.TemperatureC), append(humidity, r.RelativeHumidity), append(co2, r.CO2PPM)
		}
		span := s.Readings[len(s.Readings)-1].SampledAt.Sub(s.Readings[0].SampledAt)
		required := time.Duration(limits.BaselineMinSpanMinutes) * time.Minute
		if span < required {
			return b, Validation(field+".readings", "传感器 %s 基线跨度 %s，至少需要 %s", s.SensorID, span, required)
		}
		metrics := []MetricStability{
			stableMetric("temperature_c", temp, limits.MaxBaselineTemperatureRangeC),
			stableMetric("relative_humidity_pct", humidity, limits.MaxBaselineHumidityRangePct),
			stableMetric("co2_ppm", co2, limits.MaxBaselineCO2RangePPM),
		}
		for _, m := range metrics {
			if !m.Stable {
				return b, ValidationDetails(field+".stability."+m.Metric, map[string]any{"sensor_id": s.SensorID, "metric": m.Metric, "observed_fluctuation": m.Fluctuation, "allowed_fluctuation": m.AllowedFluctuation}, "传感器 %s 指标 %s 实测波动 %.3f 超过允许波动 %.3f", s.SensorID, m.Metric, m.Fluctuation, m.AllowedFluctuation)
			}
		}
		s.TemperatureC, s.RelativeHumidity, s.CO2PPM = metrics[0].Median, metrics[1].Median, metrics[2].Median
		s.Stability = BaselineStabilitySummary{PointCount: len(s.Readings), ObservedSpanSeconds: int64(span / time.Second), RequiredSpanSeconds: int64(required / time.Second), Stable: true, Metrics: metrics}
	}
	required := time.Duration(limits.BaselineMinSpanMinutes) * time.Minute
	allowedStaleness := time.Duration(limits.MaxBaselineStalenessMinutes) * time.Minute
	sync := BaselineSynchronizationSummary{
		MinimumCommonSpanSeconds:         int64(required / time.Second),
		AllowedMaximumStalenessSeconds:   int64(allowedStaleness / time.Second),
		AllowedAlignmentDeviationSeconds: int64(limits.MaxBaselineAlignmentSeconds),
	}
	var latestFirst, earliestLast, earliestLatestReading, latestLatestReading time.Time
	for _, s := range b.Sensors {
		first := s.Readings[0].SampledAt.UTC()
		last := s.Readings[len(s.Readings)-1].SampledAt.UTC()
		boundary := BaselineSensorTimeBoundary{SensorID: s.SensorID, FirstSampledAt: first, LastSampledAt: last, StalenessSeconds: int64(windowStart.Sub(last) / time.Second)}
		sync.SensorBoundaries = append(sync.SensorBoundaries, boundary)
		if latestFirst.IsZero() || first.After(latestFirst) {
			latestFirst = first
			sync.CoverageStartDecisiveSensorID = s.SensorID
		}
		if earliestLast.IsZero() || last.Before(earliestLast) {
			earliestLast = last
			sync.CoverageEndDecisiveSensorID = s.SensorID
		}
		if earliestLatestReading.IsZero() || last.Before(earliestLatestReading) {
			earliestLatestReading = last
			sync.StalenessDecisiveSensorID = s.SensorID
			sync.AlignmentEarliestSensorID = s.SensorID
			sync.MaximumStalenessSeconds = boundary.StalenessSeconds
		}
		if latestLatestReading.IsZero() || last.After(latestLatestReading) {
			latestLatestReading = last
			sync.AlignmentLatestSensorID = s.SensorID
		}
	}
	sync.CommonCoverageStart = latestFirst
	sync.CommonCoverageEnd = earliestLast
	if earliestLast.After(latestFirst) {
		sync.CommonCoverageSeconds = int64(earliestLast.Sub(latestFirst) / time.Second)
	}
	sync.AlignmentDeviationSeconds = int64(latestLatestReading.Sub(earliestLatestReading) / time.Second)
	if sync.CommonCoverageSeconds < sync.MinimumCommonSpanSeconds {
		return b, ValidationDetails("baseline.synchronization.common_coverage", map[string]any{"common_coverage_start": sync.CommonCoverageStart, "common_coverage_end": sync.CommonCoverageEnd, "common_coverage_seconds": sync.CommonCoverageSeconds, "minimum_common_span_seconds": sync.MinimumCommonSpanSeconds, "sensor_boundaries": sync.SensorBoundaries}, "基线传感器共同覆盖 %d 秒，至少需要 %d 秒", sync.CommonCoverageSeconds, sync.MinimumCommonSpanSeconds)
	}
	if sync.MaximumStalenessSeconds > sync.AllowedMaximumStalenessSeconds {
		return b, ValidationDetails("baseline.synchronization.staleness", map[string]any{"maximum_staleness_seconds": sync.MaximumStalenessSeconds, "allowed_maximum_staleness_seconds": sync.AllowedMaximumStalenessSeconds, "decisive_sensor_id": sync.StalenessDecisiveSensorID, "sensor_boundaries": sync.SensorBoundaries}, "传感器 %s 的末次基线读数距试验开始 %d 秒，超过允许的 %d 秒", sync.StalenessDecisiveSensorID, sync.MaximumStalenessSeconds, sync.AllowedMaximumStalenessSeconds)
	}
	if sync.AlignmentDeviationSeconds > sync.AllowedAlignmentDeviationSeconds {
		return b, ValidationDetails("baseline.synchronization.alignment", map[string]any{"alignment_deviation_seconds": sync.AlignmentDeviationSeconds, "allowed_alignment_deviation_seconds": sync.AllowedAlignmentDeviationSeconds, "sensor_boundaries": sync.SensorBoundaries}, "基线传感器末次读数对齐偏差 %d 秒，超过允许的 %d 秒", sync.AlignmentDeviationSeconds, sync.AllowedAlignmentDeviationSeconds)
	}
	b.Synchronization = sync
	b.SampledAt = b.Sensors[0].Readings[len(b.Sensors[0].Readings)-1].SampledAt.UTC()
	for _, s := range b.Sensors[1:] {
		last := s.Readings[len(s.Readings)-1].SampledAt
		if last.After(b.SampledAt) {
			b.SampledAt = last.UTC()
		}
	}
	return b, nil
}

func NewTrial(in CreateInput) (*ClearanceTrial, error) {
	if strings.TrimSpace(in.TrialID) == "" {
		return nil, Validation("trial_id", "trial_id 不能为空")
	}
	if strings.TrimSpace(in.CaveSectionID) == "" {
		return nil, Validation("cave_section_id", "洞穴区段不能为空")
	}
	if strings.TrimSpace(in.LeadObserverID) == "" {
		return nil, Validation("lead_observer_id", "责任人员不能为空")
	}
	if in.WindowStart.IsZero() || !in.WindowEnd.After(in.WindowStart) {
		return nil, Validation("test_window_end", "试验结束时间必须晚于开始时间")
	}
	in.Thresholds = NormalizeThresholds(in.Thresholds)
	if err := ValidateThresholds(in.Thresholds); err != nil {
		return nil, err
	}
	for _, s := range in.Baseline.Sensors {
		validTo, err := time.Parse(time.RFC3339, s.CalibrationValidTo)
		if err != nil || validTo.Before(in.WindowEnd) {
			return nil, Validation("baseline.sensors.calibration_valid_to", "传感器 %s 校准有效期未覆盖试验窗口", s.SensorID)
		}
	}
	b, err := freezeBaseline(in.Baseline, in.WindowStart, in.Thresholds)
	if err != nil {
		return nil, err
	}
	b.FrozenAt = in.Now.UTC()
	if b.ThresholdProfileVersion == "" {
		b.ThresholdProfileVersion = "cave-clearance-rules/v2"
	}
	return &ClearanceTrial{TrialID: in.TrialID, CaveSectionID: in.CaveSectionID, Status: StatusBaselineFrozen, TestWindowStart: in.WindowStart.UTC(), TestWindowEnd: in.WindowEnd.UTC(), LeadObserverID: in.LeadObserverID, Revision: 1, CreatedAt: in.Now.UTC(), Baseline: b, Thresholds: in.Thresholds}, nil
}

func baselineMap(t *ClearanceTrial) map[string]SensorBaseline {
	m := make(map[string]SensorBaseline, len(t.Baseline.Sensors))
	for _, s := range t.Baseline.Sensors {
		m[s.SensorID] = s
	}
	return m
}

func stageIndex(s Stage) int {
	for i, x := range PlannedStages {
		if x == s {
			return i
		}
	}
	return -1
}

func coverageFor(t *ClearanceTrial, o LoadStageObservation) (SamplingCompletenessSummary, error) {
	if o.SamplingIntervalSeconds < 1 || o.SamplingIntervalSeconds > 3600 {
		return SamplingCompletenessSummary{}, Validation("observation.sampling_interval_seconds", "采样间隔必须在 1 到 3600 秒之间")
	}
	spanSeconds := int64(o.DurationMinutes) * 60
	interval := int64(o.SamplingIntervalSeconds)
	expected := int((spanSeconds+interval-1)/interval) + 1
	grouped := map[string][]SensorSample{}
	for _, s := range o.Samples {
		grouped[s.SensorID] = append(grouped[s.SensorID], s)
	}
	ids := make([]string, 0, len(t.Baseline.Sensors))
	for _, b := range t.Baseline.Sensors {
		ids = append(ids, b.SensorID)
	}
	sort.Strings(ids)
	summary := SamplingCompletenessSummary{SamplingIntervalSeconds: o.SamplingIntervalSeconds, RequiredCoverageSeconds: max64(0, spanSeconds-2*interval), Complete: true}
	var latestFirst, earliestLast time.Time
	var minFirst, maxLast time.Time
	var gaps []string
	for _, id := range ids {
		points := grouped[id]
		missing := expected - len(points)
		if missing < 0 {
			missing = 0
		}
		c := SensorCoverage{SensorID: id, ExpectedPoints: expected, ReceivedPoints: len(points), MissingPoints: missing}
		if expected > 0 {
			c.CoveragePercent = round3(float64(len(points)) * 100 / float64(expected))
			if c.CoveragePercent > 100 {
				c.CoveragePercent = 100
			}
		}
		if len(points) > 0 {
			c.FirstOffsetSeconds = int64(points[0].SampledAt.Sub(o.StartedAt) / time.Second)
			c.LastOffsetSeconds = int64(o.EndedAt.Sub(points[len(points)-1].SampledAt) / time.Second)
			for i := 1; i < len(points); i++ {
				gap := int64(points[i].SampledAt.Sub(points[i-1].SampledAt) / time.Second)
				if gap > c.MaxGapSeconds {
					c.MaxGapSeconds = gap
				}
			}
			if latestFirst.IsZero() || points[0].SampledAt.After(latestFirst) {
				latestFirst = points[0].SampledAt
			}
			if minFirst.IsZero() || points[0].SampledAt.Before(minFirst) {
				minFirst = points[0].SampledAt
			}
			last := points[len(points)-1].SampledAt
			if earliestLast.IsZero() || last.Before(earliestLast) {
				earliestLast = last
			}
			if maxLast.IsZero() || last.After(maxLast) {
				maxLast = last
			}
		}
		c.Complete = len(points) >= expected && len(points) > 0 && c.FirstOffsetSeconds <= interval && c.LastOffsetSeconds <= interval && c.MaxGapSeconds <= interval
		if !c.Complete {
			summary.Complete = false
			gaps = append(gaps, fmt.Sprintf("%s(应有%d点/实收%d点/缺失%d点/最大间隔%d秒)", id, expected, len(points), missing, c.MaxGapSeconds))
		}
		summary.Sensors = append(summary.Sensors, c)
	}
	if len(gaps) > 0 {
		return summary, ValidationDetails("observation.samples", map[string]any{"sensor_coverage": summary.Sensors}, "传感器覆盖不完整："+strings.Join(gaps, "；"))
	}
	summary.CommonCoverageStart, summary.CommonCoverageEnd = latestFirst.UTC(), earliestLast.UTC()
	if earliestLast.After(latestFirst) {
		summary.CommonCoverageSeconds = int64(earliestLast.Sub(latestFirst) / time.Second)
	}
	startDeviation, endDeviation := int64(latestFirst.Sub(minFirst)/time.Second), int64(maxLast.Sub(earliestLast)/time.Second)
	summary.AlignmentDeviationSeconds = max64(startDeviation, endDeviation)
	if summary.CommonCoverageSeconds < summary.RequiredCoverageSeconds {
		return summary, Validation("observation.coverage.common", "多传感器共同覆盖 %d 秒，至少需要 %d 秒", summary.CommonCoverageSeconds, summary.RequiredCoverageSeconds)
	}
	if summary.AlignmentDeviationSeconds > int64(t.Thresholds.MaxSamplingAlignmentSeconds) {
		return summary, Validation("observation.coverage.alignment", "传感器时间对齐偏差 %d 秒，允许最大 %d 秒", summary.AlignmentDeviationSeconds, t.Thresholds.MaxSamplingAlignmentSeconds)
	}
	return summary, nil
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func (t *ClearanceTrial) AddObservation(o LoadStageObservation) error {
	if t.Terminal() {
		return InvalidState("终态试验禁止修改")
	}
	if t.Status != StatusBaselineFrozen && t.Status != StatusSampling {
		return InvalidState("状态 %s 不允许提交负荷观测", t.Status)
	}
	expected := len(t.Observations)
	if expected >= len(PlannedStages) || o.Stage != PlannedStages[expected] {
		return Validation("observation.stage", "必须按 low、medium、high 顺序提交阶段")
	}
	if o.VisitorCount <= 0 || o.DurationMinutes <= 0 {
		return Validation("observation.visitor_count", "访客人数和持续时间必须为正数")
	}
	if o.ObserverID == "" {
		return Validation("observation.observer_id", "采样人员不能为空")
	}
	if o.StartedAt.Before(t.TestWindowStart) || o.EndedAt.After(t.TestWindowEnd) || !o.EndedAt.After(o.StartedAt) {
		return Validation("observation.started_at", "观测时间必须位于试验窗口且结束晚于开始")
	}
	actual := o.EndedAt.Sub(o.StartedAt)
	if time.Duration(o.DurationMinutes)*time.Minute < actual {
		return Validation("observation.duration_minutes", "声明持续时间 %d 分钟不得短于实际观测时长 %s", o.DurationMinutes, actual)
	}
	if expected > 0 {
		prior := t.Observations[expected-1]
		if o.VisitorCount <= prior.VisitorCount {
			return Validation("observation.visitor_count", "%s 阶段人数 %d 必须严格大于前序 %s 阶段人数 %d", o.Stage, o.VisitorCount, prior.Stage, prior.VisitorCount)
		}
		if o.DurationMinutes < prior.DurationMinutes {
			return Validation("observation.duration_minutes", "%s 阶段持续时间 %d 不得低于前序阶段 %d", o.Stage, o.DurationMinutes, prior.DurationMinutes)
		}
		required := time.Duration(t.Thresholds.MinStageRestMinutes) * time.Minute
		if o.StartedAt.Sub(prior.EndedAt) < required {
			return ValidationDetails("observation.started_at", map[string]any{"previous_ended_at": prior.EndedAt.UTC(), "current_started_at": o.StartedAt.UTC(), "required_rest_seconds": int64(required / time.Second)}, "前序结束时间 %s，当前开始时间 %s，至少需要静置 %s", prior.EndedAt.UTC().Format(time.RFC3339), o.StartedAt.UTC().Format(time.RFC3339), required)
		}
	}
	bases := baselineMap(t)
	last := map[string]time.Time{}
	for _, s := range o.Samples {
		if _, ok := bases[s.SensorID]; !ok {
			return Validation("observation.samples.sensor_id", "传感器 %s 未出现在冻结基线", s.SensorID)
		}
		if s.SampledAt.Before(o.StartedAt) || s.SampledAt.After(o.EndedAt) {
			return Validation("observation.samples.sampled_at", "采样点必须位于阶段时间范围")
		}
		if p, ok := last[s.SensorID]; ok && !s.SampledAt.After(p) {
			return Validation("observation.samples.sampled_at", "传感器 %s 采样时间必须严格递增", s.SensorID)
		}
		if !readingWithinPhysicalRange(s.TemperatureC, s.RelativeHumidity, s.CO2PPM) {
			return Validation("observation.samples", "采样读数超出物理范围")
		}
		last[s.SensorID] = s.SampledAt
	}
	coverage, err := coverageFor(t, o)
	if err != nil {
		return err
	}
	o.Coverage = coverage
	t.Observations = append(t.Observations, o)
	if len(t.Observations) == len(PlannedStages) {
		t.Status = StatusReadyAssess
	} else {
		t.Status = StatusSampling
	}
	return nil
}
