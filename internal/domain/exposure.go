package domain

import (
	"math"
	"sort"
)

const ExposureRuleVersion = "positive-trapezoid-actual-time/v1"

var exposureMetricNames = []string{
	"positive_temperature_delta_c",
	"positive_relative_humidity_delta_pct",
	"co2_above_baseline_ppm",
}

func positive(v float64) float64 {
	if v > 0 {
		return v
	}
	return 0
}

func integrateExposure(samples []SensorSample, base SensorBaseline) []ExposureMetric {
	metrics := []ExposureMetric{
		{Metric: exposureMetricNames[0]},
		{Metric: exposureMetricNames[1]},
		{Metric: exposureMetricNames[2]},
	}
	if len(samples) < 2 {
		return metrics
	}
	values := func(s SensorSample) [3]float64 {
		return [3]float64{
			positive(s.TemperatureC - base.TemperatureC),
			positive(s.RelativeHumidity - base.RelativeHumidity),
			positive(s.CO2PPM - base.CO2PPM),
		}
	}
	for i := 1; i < len(samples); i++ {
		minutes := samples[i].SampledAt.Sub(samples[i-1].SampledAt).Minutes()
		left, right := values(samples[i-1]), values(samples[i])
		for metric := range metrics {
			metrics[metric].IntegratedExposure += (left[metric] + right[metric]) * minutes / 2
		}
	}
	observed := samples[len(samples)-1].SampledAt.Sub(samples[0].SampledAt).Minutes()
	for i := range metrics {
		metrics[i].IntegratedExposure = round3(metrics[i].IntegratedExposure)
		metrics[i].ObservedMinutes = round3(observed)
		if observed > 0 {
			metrics[i].NormalizedExposurePerMinute = round3(metrics[i].IntegratedExposure / observed)
		}
	}
	return metrics
}

func exposureAnalysis(t *ClearanceTrial) ([]StageExposure, []ExposureTrend) {
	bases := baselineMap(t)
	stageExposures := make([]StageExposure, 0, len(t.Observations))
	series := map[string]map[string][]float64{}
	for _, metric := range exposureMetricNames {
		series[metric] = map[string][]float64{}
	}
	for _, observation := range t.Observations {
		grouped := map[string][]SensorSample{}
		for _, sample := range observation.Samples {
			grouped[sample.SensorID] = append(grouped[sample.SensorID], sample)
		}
		ids := make([]string, 0, len(grouped))
		for id := range grouped {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		stage := StageExposure{Stage: observation.Stage}
		for _, id := range ids {
			metrics := integrateExposure(grouped[id], bases[id])
			stage.Sensors = append(stage.Sensors, SensorStageExposure{SensorID: id, Metrics: metrics})
			for _, metric := range metrics {
				series[metric.Metric][id] = append(series[metric.Metric][id], metric.NormalizedExposurePerMinute)
			}
		}
		stageExposures = append(stageExposures, stage)
	}
	trends := make([]ExposureTrend, 0, len(exposureMetricNames))
	for _, metric := range exposureMetricNames {
		ids := make([]string, 0, len(series[metric]))
		for id := range series[metric] {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		stageMax := make([]float64, len(PlannedStages))
		set := make([]bool, len(PlannedStages))
		trend := ExposureTrend{Metric: metric, Conclusion: "basically_stable"}
		largestJump := math.Inf(-1)
		for _, id := range ids {
			values := series[metric][id]
			for i, value := range values {
				if !set[i] || value > stageMax[i] {
					stageMax[i], set[i] = value, true
				}
			}
			for i := 1; i < len(values); i++ {
				jump := round3(values[i] - values[i-1])
				if jump > largestJump {
					largestJump = jump
					trend.DecisiveSensorID = id
					trend.LargestJumpFromStage = PlannedStages[i-1]
					trend.LargestJumpToStage = PlannedStages[i]
					trend.LargestNormalizedJump = jump
				}
			}
		}
		if len(stageMax) == 3 {
			d1, d2 := stageMax[1]-stageMax[0], stageMax[2]-stageMax[1]
			switch {
			case d1 > 0.001 && d2 > 0.001:
				trend.Conclusion = "persistent_deterioration"
			case math.Abs(d1) <= 0.001 && math.Abs(d2) <= 0.001:
				trend.Conclusion = "basically_stable"
			default:
				trend.Conclusion = "reverse_change"
			}
		}
		trends = append(trends, trend)
	}
	return stageExposures, trends
}
