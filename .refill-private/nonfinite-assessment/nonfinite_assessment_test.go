package nonfinite_assessment_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"cave-microclimate-clearance/internal/application"
	"cave-microclimate-clearance/internal/domain"
	"cave-microclimate-clearance/internal/store"
)

func TestExtremeFiniteReadingCannotBreakAssessmentPersistence(t *testing.T) {
	repo, err := store.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := application.NewService(repo)
	now := time.Now().UTC().Truncate(time.Second)
	start := now.Add(-2 * time.Hour)
	end := now.Add(time.Hour)
	readings := []domain.BaselineReading{
		{SampledAt: start.Add(-11 * time.Minute), TemperatureC: 14, RelativeHumidity: 70, CO2PPM: 500},
		{SampledAt: start.Add(-6 * time.Minute), TemperatureC: 14.1, RelativeHumidity: 70.5, CO2PPM: 510},
		{SampledAt: start.Add(-time.Minute), TemperatureC: 14, RelativeHumidity: 70, CO2PPM: 505},
	}
	create := application.CreateTrialCommand{
		CommandMeta: application.CommandMeta{RequestID: "create", ActorID: "lead"},
		TrialID: "extreme-reading", CaveSectionID: "section", TestWindowStart: start, TestWindowEnd: end, LeadObserverID: "lead",
		Baseline: domain.BaselineProfile{Sensors: []domain.SensorBaseline{{
			SensorID: "s1", CalibrationRef: "cal", CalibrationValidTo: end.Add(time.Hour).Format(time.RFC3339), Readings: readings,
		}}},
	}
	if _, err := app.CreateTrial(context.Background(), create); err != nil {
		t.Fatal(err)
	}

	for i, stage := range domain.PlannedStages {
		begin := start.Add(time.Duration(i*15) * time.Minute)
		samples := []domain.SensorSample{
			{SensorID: "s1", SampledAt: begin, TemperatureC: 1e308, RelativeHumidity: 71, CO2PPM: 600},
			{SensorID: "s1", SampledAt: begin.Add(5 * time.Minute), TemperatureC: 1e308, RelativeHumidity: 71, CO2PPM: 600},
			{SensorID: "s1", SampledAt: begin.Add(10 * time.Minute), TemperatureC: 1e308, RelativeHumidity: 71, CO2PPM: 600},
		}
		command := application.ObservationCommand{
			CommandMeta: application.CommandMeta{RequestID: "stage-" + string(stage), ExpectedRevision: int64(i + 1), ActorID: "sampler"},
			Observation: domain.LoadStageObservation{Stage: stage, VisitorCount: i + 1, DurationMinutes: 10, SamplingIntervalSeconds: 300, Samples: samples, ObserverID: "sampler", StartedAt: begin, EndedAt: begin.Add(10 * time.Minute)},
		}
		if _, err := app.AddObservation(context.Background(), create.TrialID, command); err != nil {
			var validation *domain.Error
			if errors.As(err, &validation) && validation.Code == domain.CodeValidation {
				return
			}
			t.Fatalf("极端读数在采集层产生了非校验错误: %v", err)
		}
	}

	_, err = app.Assess(context.Background(), create.TrialID, application.AssessmentCommand{CommandMeta: application.CommandMeta{RequestID: "assess", ExpectedRevision: 4, ActorID: "lead"}})
	if err != nil {
		var validation *domain.Error
		if errors.As(err, &validation) && validation.Code == domain.CodeValidation {
			return
		}
		t.Fatalf("极端有限读数跨层生成了无法持久化的非有限判定值: %v", err)
	}
	stored, err := repo.Get(context.Background(), create.TrialID)
	if err != nil {
		t.Fatalf("判定成功后持久化记录不可读: %v", err)
	}
	if _, err := json.Marshal(stored); err != nil {
		t.Fatalf("判定成功后聚合无法编码为 JSON: %v", err)
	}
}
