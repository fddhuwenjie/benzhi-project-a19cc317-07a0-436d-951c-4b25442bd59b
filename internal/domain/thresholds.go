package domain

import "fmt"

func NormalizeThresholds(t Thresholds) Thresholds {
	d := DefaultThresholds()
	if t.BaselineMinPoints == 0 {
		t.BaselineMinPoints = d.BaselineMinPoints
	}
	if t.BaselineMinSpanMinutes == 0 {
		t.BaselineMinSpanMinutes = d.BaselineMinSpanMinutes
	}
	if t.MaxBaselineTemperatureRangeC == 0 {
		t.MaxBaselineTemperatureRangeC = d.MaxBaselineTemperatureRangeC
	}
	if t.MaxBaselineHumidityRangePct == 0 {
		t.MaxBaselineHumidityRangePct = d.MaxBaselineHumidityRangePct
	}
	if t.MaxBaselineCO2RangePPM == 0 {
		t.MaxBaselineCO2RangePPM = d.MaxBaselineCO2RangePPM
	}
	if t.MaxBaselineStalenessMinutes == 0 {
		t.MaxBaselineStalenessMinutes = d.MaxBaselineStalenessMinutes
	}
	if t.MaxBaselineAlignmentSeconds == 0 {
		t.MaxBaselineAlignmentSeconds = d.MaxBaselineAlignmentSeconds
	}
	if t.MinStageRestMinutes == 0 {
		t.MinStageRestMinutes = d.MinStageRestMinutes
	}
	if t.MaxSamplingAlignmentSeconds == 0 {
		t.MaxSamplingAlignmentSeconds = d.MaxSamplingAlignmentSeconds
	}
	if t.RecoveryMinMinutes == 0 {
		t.RecoveryMinMinutes = d.RecoveryMinMinutes
	}
	if t.RecoveryMaxGapSeconds == 0 {
		t.RecoveryMaxGapSeconds = d.RecoveryMaxGapSeconds
	}
	return t
}

func ValidateThresholds(t Thresholds) error {
	if t.MaxTemperatureDeltaC <= 0 || t.MaxHumidityDeltaPct <= 0 || t.MaxCO2PPM <= 0 {
		return Validation("thresholds", "停止阈值必须为正数")
	}
	// 阈值参与 round3 与百分比安全余量计算（margin*100/limit），缺乏上界会使数量级极大但 JSON
	// 可表示的有限阈值在判定派生计算中溢出为 ±Inf，无法编码为 JSON。
	if t.MaxTemperatureDeltaC > 100000 || t.MaxHumidityDeltaPct > 100000 || t.MaxCO2PPM > 100000 {
		return Validation("thresholds", "停止阈值超出物理合理范围")
	}
	if t.RecoveryTempDeltaC <= 0 || t.RecoveryTempDeltaC >= t.MaxTemperatureDeltaC {
		return Validation("thresholds.recovery_temperature_delta_c", "温度恢复阈值必须小于停止阈值")
	}
	if t.RecoveryHumidityDelta <= 0 || t.RecoveryHumidityDelta >= t.MaxHumidityDeltaPct {
		return Validation("thresholds.recovery_humidity_delta_pct", "湿度恢复阈值必须小于停止阈值")
	}
	if t.RecoveryCO2PPM <= 0 || t.RecoveryCO2PPM >= t.MaxCO2PPM {
		return Validation("thresholds.recovery_co2_ppm", "二氧化碳恢复阈值必须小于停止阈值")
	}
	if t.RecoveryPoints < 2 || t.RecoveryPoints > 60 {
		return Validation("thresholds.recovery_points", "连续恢复点数必须在 2 到 60 之间")
	}
	if t.BaselineMinPoints < 3 || t.BaselineMinPoints > 120 {
		return Validation("thresholds.baseline_min_points", "基线点数要求必须在 3 到 120 之间")
	}
	if t.BaselineMinSpanMinutes < 1 || t.BaselineMinSpanMinutes > 1440 {
		return Validation("thresholds.baseline_min_span_minutes", "基线最短跨度必须在 1 到 1440 分钟之间")
	}
	if t.MaxBaselineTemperatureRangeC <= 0 || t.MaxBaselineHumidityRangePct <= 0 || t.MaxBaselineCO2RangePPM <= 0 {
		return Validation("thresholds", "基线波动限值必须为正数")
	}
	if t.MaxBaselineTemperatureRangeC > 100000 || t.MaxBaselineHumidityRangePct > 100000 || t.MaxBaselineCO2RangePPM > 100000 {
		return Validation("thresholds", "基线波动限值超出物理合理范围")
	}
	if t.MaxBaselineStalenessMinutes < 1 || t.MaxBaselineStalenessMinutes > 10080 {
		return Validation("thresholds.max_baseline_staleness_minutes", "基线最大陈旧时长必须在 1 到 10080 分钟之间")
	}
	if t.MaxBaselineAlignmentSeconds < 1 || t.MaxBaselineAlignmentSeconds > 86400 {
		return Validation("thresholds.max_baseline_alignment_seconds", "基线跨传感器对齐限值必须在 1 到 86400 秒之间")
	}
	if t.MinStageRestMinutes < 1 || t.MinStageRestMinutes > 1440 {
		return Validation("thresholds.min_stage_rest_minutes", "阶段静置间隔必须在 1 到 1440 分钟之间")
	}
	if t.MaxSamplingAlignmentSeconds < 1 || t.MaxSamplingAlignmentSeconds > 3600 {
		return Validation("thresholds.max_sampling_alignment_seconds", "采样对齐偏差必须在 1 到 3600 秒之间")
	}
	if t.RecoveryMinMinutes < 1 || t.RecoveryMinMinutes > 1440 || t.RecoveryMaxGapSeconds < 1 || t.RecoveryMaxGapSeconds > 3600 {
		return Validation("thresholds", "恢复连续窗口配置无效")
	}
	return nil
}

func ThresholdSummary(t Thresholds, version string) string {
	return fmt.Sprintf("%s|temp<=%.3f|rh<=%.3f|co2<=%.3f|baseline_sync=%dm/%ds|recovery=%d/%dm|rest=%dm|alignment=%ds|exposure=trapezoid_positive/v1|round=0.001|tie=stage_asc,sensor_asc,time_asc", version, t.MaxTemperatureDeltaC, t.MaxHumidityDeltaPct, t.MaxCO2PPM, t.MaxBaselineStalenessMinutes, t.MaxBaselineAlignmentSeconds, t.RecoveryPoints, t.RecoveryMinMinutes, t.MinStageRestMinutes, t.MaxSamplingAlignmentSeconds)
}
