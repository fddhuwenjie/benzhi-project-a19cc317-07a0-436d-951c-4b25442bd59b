package duration_overflow_test

import (
	"errors"
	"testing"
	"time"

	"cave-microclimate-clearance/internal/domain"
)

func TestOverflowedDeclaredDurationCannotBecomePermitLimit(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	start := now.Add(time.Hour)
	end := start.Add(2 * time.Hour)
	readings := []domain.BaselineReading{
		{SampledAt: now.Add(-10 * time.Minute), TemperatureC: 13.9, RelativeHumidity: 69.5, CO2PPM: 490},
		{SampledAt: now.Add(-5 * time.Minute), TemperatureC: 14, RelativeHumidity: 70, CO2PPM: 500},
		{SampledAt: now, TemperatureC: 14.1, RelativeHumidity: 70.5, CO2PPM: 510},
	}
	trial, err := domain.NewTrial(domain.CreateInput{
		TrialID: "duration-overflow", CaveSectionID: "section", WindowStart: start, WindowEnd: end,
		LeadObserverID: "lead", Thresholds: domain.DefaultThresholds(), Now: now,
		Baseline: domain.BaselineProfile{Sensors: []domain.SensorBaseline{{
			SensorID: "s1", CalibrationRef: "cal", CalibrationValidTo: end.Add(time.Hour).Format(time.RFC3339), Readings: readings,
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}

	const wrappedMinutes = int(1<<62 + 10)
	for i, stage := range domain.PlannedStages {
		begin := start.Add(time.Duration(i*15) * time.Minute)
		samples := []domain.SensorSample{
			{SensorID: "s1", SampledAt: begin, TemperatureC: 14.1, RelativeHumidity: 70.5, CO2PPM: 550},
			{SensorID: "s1", SampledAt: begin.Add(5 * time.Minute), TemperatureC: 14.2, RelativeHumidity: 71, CO2PPM: 575},
			{SensorID: "s1", SampledAt: begin.Add(10 * time.Minute), TemperatureC: 14.3, RelativeHumidity: 71.5, CO2PPM: 600},
		}
		err := trial.AddObservation(domain.LoadStageObservation{
			ObservationID: string(stage), Stage: stage, VisitorCount: i + 1, DurationMinutes: wrappedMinutes,
			SamplingIntervalSeconds: 300, ObserverID: "sampler", StartedAt: begin, EndedAt: begin.Add(10 * time.Minute), Samples: samples,
		})
		if err != nil {
			var validation *domain.Error
			if errors.As(err, &validation) && validation.Code == domain.CodeValidation {
				return
			}
			t.Fatalf("超大声明时长产生了非校验错误: %v", err)
		}
	}
	if _, err := trial.Assess("assessment", start.Add(45*time.Minute)); err != nil {
		t.Fatalf("溢出值穿过采集后导致判定异常: %v", err)
	}
	err = trial.ReviewTrial(domain.ReviewInput{
		ReviewerID: "reviewer", Approved: true, Checks: domain.ReviewChecks{StagesComplete: true, CalibrationsValid: true, AssessmentVerified: true, RecoveryVerified: true},
		MaxConcurrentVisitors: 3, MaxStayMinutes: wrappedMinutes, PermitID: "permit",
		ValidFrom: start.Add(50 * time.Minute), ValidUntil: start.Add(24 * time.Hour), Now: start.Add(45 * time.Minute),
	})
	if err != nil {
		var validation *domain.Error
		if errors.As(err, &validation) && validation.Code == domain.CodeValidation {
			return
		}
		t.Fatalf("溢出值穿过判定后导致复核异常: %v", err)
	}
	if trial.Permit != nil && trial.Permit.MaxStayMinutes == wrappedMinutes {
		t.Fatalf("十分钟实际观测被整数溢出放大为 %d 分钟许可证上限", trial.Permit.MaxStayMinutes)
	}
}
