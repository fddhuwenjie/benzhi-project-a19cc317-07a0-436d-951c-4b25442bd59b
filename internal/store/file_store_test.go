package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cave-microclimate-clearance/internal/domain"
)

func storedTrial(t *testing.T) *domain.ClearanceTrial {
	t.Helper()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	start := now.Add(time.Hour)
	end := start.Add(time.Hour)
	readings := []domain.BaselineReading{
		{SampledAt: now.Add(-10 * time.Minute), TemperatureC: 12, RelativeHumidity: 60, CO2PPM: 500},
		{SampledAt: now.Add(-5 * time.Minute), TemperatureC: 12.1, RelativeHumidity: 60.5, CO2PPM: 510},
		{SampledAt: now, TemperatureC: 12.2, RelativeHumidity: 61, CO2PPM: 520},
	}
	trial, err := domain.NewTrial(domain.CreateInput{TrialID: "trial-store", CaveSectionID: "section", WindowStart: start, WindowEnd: end, LeadObserverID: "lead", Baseline: domain.BaselineProfile{BaselineID: "base", ThresholdProfileVersion: "rules/v1", Sensors: []domain.SensorBaseline{{SensorID: "s1", CalibrationRef: "cal", CalibrationValidTo: end.Add(time.Hour).Format(time.RFC3339), Readings: readings}}}, Thresholds: domain.DefaultThresholds(), Now: now})
	if err != nil {
		t.Fatal(err)
	}
	return trial
}

func TestIdempotentCreateAndConflict(t *testing.T) {
	repo, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	trial := storedTrial(t)
	first, err := repo.Create(context.Background(), trial, "req-1", "fingerprint-A", "payload-A", "lead", map[string]any{"revision": 1})
	if err != nil {
		t.Fatal(err)
	}
	second, err := repo.Create(context.Background(), trial, "req-1", "fingerprint-A", "payload-A", "lead", map[string]any{"revision": 999})
	if err != nil {
		t.Fatal(err)
	}
	if !second.Replayed || string(first.ResponseJSON) != string(second.ResponseJSON) {
		t.Fatal("幂等响应不稳定")
	}
	_, err = repo.Create(context.Background(), trial, "req-1", "fingerprint-B", "payload-B", "lead", nil)
	var conflict *IdempotencyConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("应返回幂等冲突: %v", err)
	}
}

func TestRevisionConflictAndCorruptionDetection(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	trial := storedTrial(t)
	if _, err := repo.Create(context.Background(), trial, "req-1", "fp", "payload", "lead", map[string]int{"revision": 1}); err != nil {
		t.Fatal(err)
	}
	_, err = repo.Transact(context.Background(), trial.TrialID, "req-2", "fp2", 9, func(*domain.ClearanceTrial) (Mutation, error) { return Mutation{}, nil })
	var conflict *RevisionConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("应返回修订冲突: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, trial.TrialID+".json"), []byte("{\"truncated\":"), 0o640); err != nil {
		t.Fatal(err)
	}
	_, err = repo.Get(context.Background(), trial.TrialID)
	var corrupt *CorruptError
	if !errors.As(err, &corrupt) {
		t.Fatalf("应检测截断记录: %v", err)
	}
}
