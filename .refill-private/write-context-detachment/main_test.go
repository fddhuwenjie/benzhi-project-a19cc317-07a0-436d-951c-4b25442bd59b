package write_context_detachment_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"cave-microclimate-clearance/internal/application"
	"cave-microclimate-clearance/internal/domain"
	"cave-microclimate-clearance/internal/store"
)

func TestWriteCommandsPreserveCallerCancellation(t *testing.T) {
	repo, err := store.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewService(repo)
	start := time.Now().UTC().Truncate(time.Second).Add(time.Hour)
	end := start.Add(time.Hour)
	readings := []domain.BaselineReading{
		{SampledAt: start.Add(-20 * time.Minute), TemperatureC: 12.0, RelativeHumidity: 60.0, CO2PPM: 500},
		{SampledAt: start.Add(-15 * time.Minute), TemperatureC: 12.1, RelativeHumidity: 60.2, CO2PPM: 510},
		{SampledAt: start.Add(-10 * time.Minute), TemperatureC: 12.2, RelativeHumidity: 60.4, CO2PPM: 520},
	}
	command := application.CreateTrialCommand{
		CommandMeta:     application.CommandMeta{RequestID: "request-cancelled", ActorID: "observer-a"},
		TrialID:         "trial-cancelled-write",
		CaveSectionID:   "section-cancelled-write",
		TestWindowStart: start,
		TestWindowEnd:   end,
		LeadObserverID:  "observer-a",
		Baseline: domain.BaselineProfile{Sensors: []domain.SensorBaseline{{
			SensorID:           "sensor-a",
			CalibrationRef:     "calibration-a",
			CalibrationValidTo: end.Add(time.Hour).Format(time.RFC3339),
			Readings:           readings,
		}}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, commandErr := service.CreateTrial(ctx, command)
	_, loadErr := repo.Get(context.Background(), command.TrialID)
	if !errors.Is(commandErr, context.Canceled) || !errors.Is(loadErr, store.ErrNotFound) {
		t.Fatalf("TestWriteCommandsPreserveCallerCancellation: 已取消的写命令应返回 context.Canceled 且不得持久化，command_err=%v load_err=%v", commandErr, loadErr)
	}
}
