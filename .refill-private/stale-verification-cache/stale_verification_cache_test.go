package stale_verification_cache_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cave-microclimate-clearance/internal/application"
	"cave-microclimate-clearance/internal/domain"
	"cave-microclimate-clearance/internal/evidence"
	"cave-microclimate-clearance/internal/store"
)

func createRejectedTrial(t *testing.T, app *application.Service, trialID string) {
	t.Helper()
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	start := now.Add(-2 * time.Hour)
	end := now.Add(time.Hour)
	readings := []domain.BaselineReading{
		{SampledAt: start.Add(-11 * time.Minute), TemperatureC: 13.9, RelativeHumidity: 69.5, CO2PPM: 490},
		{SampledAt: start.Add(-6 * time.Minute), TemperatureC: 14, RelativeHumidity: 70, CO2PPM: 500},
		{SampledAt: start.Add(-time.Minute), TemperatureC: 14.1, RelativeHumidity: 70.5, CO2PPM: 510},
	}
	create := application.CreateTrialCommand{
		CommandMeta:     application.CommandMeta{RequestID: "create", ActorID: "lead"},
		TrialID:         trialID,
		CaveSectionID:   "section-cache-resource",
		TestWindowStart: start,
		TestWindowEnd:   end,
		LeadObserverID:  "lead",
		Baseline: domain.BaselineProfile{Sensors: []domain.SensorBaseline{{
			SensorID: "sensor-1", CalibrationRef: "calibration-1",
			CalibrationValidTo: end.Add(time.Hour).Format(time.RFC3339), Readings: readings,
		}}},
	}
	if _, err := app.CreateTrial(context.Background(), create); err != nil {
		t.Fatal(err)
	}

	revision := int64(1)
	for i, stage := range domain.PlannedStages {
		begin := start.Add(time.Duration(i*15) * time.Minute)
		samples := []domain.SensorSample{
			{SensorID: "sensor-1", SampledAt: begin, TemperatureC: 14.1, RelativeHumidity: 70.5, CO2PPM: 550},
			{SensorID: "sensor-1", SampledAt: begin.Add(5 * time.Minute), TemperatureC: 14.2, RelativeHumidity: 71, CO2PPM: 575},
			{SensorID: "sensor-1", SampledAt: begin.Add(10 * time.Minute), TemperatureC: 14.3, RelativeHumidity: 71.5, CO2PPM: 600},
		}
		command := application.ObservationCommand{
			CommandMeta: application.CommandMeta{RequestID: "stage-" + string(stage), ExpectedRevision: revision, ActorID: "sampler"},
			Observation: domain.LoadStageObservation{
				Stage: stage, VisitorCount: (i + 1) * 3, DurationMinutes: 10,
				SamplingIntervalSeconds: 300, Samples: samples, ObserverID: "sampler",
				StartedAt: begin, EndedAt: begin.Add(10 * time.Minute),
			},
		}
		if _, err := app.AddObservation(context.Background(), trialID, command); err != nil {
			t.Fatal(err)
		}
		revision++
	}
	if _, err := app.Assess(context.Background(), trialID, application.AssessmentCommand{CommandMeta: application.CommandMeta{RequestID: "assess", ExpectedRevision: revision, ActorID: "lead"}}); err != nil {
		t.Fatal(err)
	}
	revision++
	review := application.ReviewCommand{
		CommandMeta:      application.CommandMeta{RequestID: "review", ExpectedRevision: revision, ActorID: "reviewer"},
		Approved:         false,
		Checks:           domain.ReviewChecks{StagesComplete: true, CalibrationsValid: true, AssessmentVerified: true, RecoveryVerified: true},
		RejectionReasons: []domain.RejectionReason{{Code: "conservation_hold", Detail: "保护性暂缓"}},
	}
	if _, err := app.Review(context.Background(), trialID, review); err != nil {
		t.Fatal(err)
	}
}

func TestVerificationCacheDoesNotMaskStoreCorruption(t *testing.T) {
	dir := t.TempDir()
	repo, err := store.NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	const trialID = "trial-cache-resource"
	createRejectedTrial(t, application.NewService(repo), trialID)
	service := evidence.NewService(repo)

	first, err := service.VerifyCurrent(context.Background(), trialID)
	if err != nil || !first.Valid {
		t.Fatalf("首次校验应读取健康仓库并得到有效结果: valid=%v err=%v", first.Valid, err)
	}
	if err := os.WriteFile(filepath.Join(dir, trialID+".json"), []byte("{\"truncated\":"), 0o640); err != nil {
		t.Fatal(err)
	}

	_, err = service.VerifyCurrent(context.Background(), trialID)
	var corrupt *store.CorruptError
	if !errors.As(err, &corrupt) {
		t.Fatalf("底层快照失效后必须重新校验并返回 CorruptError，实际错误: %v", err)
	}
}
