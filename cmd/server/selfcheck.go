package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"cave-microclimate-clearance/internal/application"
	"cave-microclimate-clearance/internal/domain"
	"cave-microclimate-clearance/internal/evidence"
)

func runSelfCheck(c config) error {
	dir, err := os.MkdirTemp("", "cave-clearance-selfcheck-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	srv, err := buildServer(dir)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", c.Addr)
	if err != nil {
		return fmt.Errorf("自检监听失败: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ln) }()
	base := "http://" + ln.Addr().String()
	client := &http.Client{Timeout: 5 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	flowErr := exerciseFlow(ctx, client, base)
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer shutdownCancel()
	shutdownErr := srv.Shutdown(shutdownCtx)
	serveErr := <-done
	if flowErr != nil {
		return flowErr
	}
	if shutdownErr != nil {
		return shutdownErr
	}
	if !errors.Is(serveErr, http.ErrServerClosed) {
		return serveErr
	}
	fmt.Println("self-check: 洞穴微气候异常恢复、独立复核、许可与证据校验全流程通过")
	return nil
}

func sendJSON(ctx context.Context, client *http.Client, method, url string, body any, want int, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != want {
		return fmt.Errorf("%s %s 返回 %d，期望 %d: %s", method, url, resp.StatusCode, want, string(data))
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return err
		}
	}
	return nil
}

func exerciseFlow(ctx context.Context, client *http.Client, base string) error {
	now := time.Now().UTC().Truncate(time.Second)
	start := now.Add(-60 * time.Minute)
	end := now.Add(5 * time.Minute)
	trialID := "selfcheck-trial"
	baselineReadings := []domain.BaselineReading{
		{SampledAt: start.Add(-11 * time.Minute), TemperatureC: 13.9, RelativeHumidity: 69.5, CO2PPM: 510},
		{SampledAt: start.Add(-6 * time.Minute), TemperatureC: 14, RelativeHumidity: 70, CO2PPM: 520},
		{SampledAt: start.Add(-time.Minute), TemperatureC: 14.1, RelativeHumidity: 70.5, CO2PPM: 530},
	}
	create := application.CreateTrialCommand{CommandMeta: application.CommandMeta{RequestID: "req-create", ExpectedRevision: 0, ActorID: "lead-monitor"}, TrialID: trialID, CaveSectionID: "cave-section-A", TestWindowStart: start, TestWindowEnd: end, LeadObserverID: "lead-monitor", Baseline: domain.BaselineProfile{BaselineID: "baseline-A", ThresholdProfileVersion: "cave-clearance-rules/v2", Sensors: []domain.SensorBaseline{{SensorID: "sensor-1", CalibrationRef: "CAL-2026-A", CalibrationValidTo: end.Add(24 * time.Hour).Format(time.RFC3339), Readings: baselineReadings}}}}
	var cr application.CommandResponse
	if err := sendJSON(ctx, client, http.MethodPost, base+"/api/v1/trials", create, http.StatusCreated, &cr); err != nil {
		return err
	}
	if cr.Revision != 1 {
		return errors.New("创建修订错误")
	}
	stages := []struct {
		stage              domain.Stage
		visitors, duration int
		temp, rh, co2      float64
	}{{domain.StageLow, 2, 10, 14.3, 71, 650}, {domain.StageMedium, 5, 15, 14.8, 73, 900}, {domain.StageHigh, 8, 20, 16, 77, 1450}}
	rev := int64(1)
	offsets := []int{0, 15, 35}
	for i, x := range stages {
		begin := start.Add(time.Duration(offsets[i]) * time.Minute)
		var samples []domain.SensorSample
		for minute := 0; minute <= x.duration; minute++ {
			fraction := float64(minute) / float64(x.duration)
			samples = append(samples, domain.SensorSample{SensorID: "sensor-1", SampledAt: begin.Add(time.Duration(minute) * time.Minute), TemperatureC: x.temp - .1 + .1*fraction, RelativeHumidity: x.rh - .2 + .2*fraction, CO2PPM: x.co2 - 30 + 30*fraction})
		}
		cmd := application.ObservationCommand{CommandMeta: application.CommandMeta{RequestID: fmt.Sprintf("req-stage-%d", i), ExpectedRevision: rev, ActorID: "field-sampler"}, Observation: domain.LoadStageObservation{ObservationID: fmt.Sprintf("obs-%d", i), Stage: x.stage, VisitorCount: x.visitors, DurationMinutes: x.duration, SamplingIntervalSeconds: 60, Samples: samples, ObserverID: "field-sampler", StartedAt: begin, EndedAt: begin.Add(time.Duration(x.duration) * time.Minute)}}
		if err := sendJSON(ctx, client, http.MethodPost, base+"/api/v1/trials/"+trialID+"/observations", cmd, http.StatusOK, nil); err != nil {
			return err
		}
		rev++
	}
	assessment := application.AssessmentCommand{CommandMeta: application.CommandMeta{RequestID: "req-assess", ExpectedRevision: rev, ActorID: "lead-monitor"}}
	if err := sendJSON(ctx, client, http.MethodPost, base+"/api/v1/trials/"+trialID+"/assessment", assessment, http.StatusOK, nil); err != nil {
		return err
	}
	rev++
	completedAt := now.Add(-4 * time.Minute)
	failedRecovery := application.RecoveryCommand{CommandMeta: application.CommandMeta{RequestID: "req-recovery-failed", ExpectedRevision: rev, ActorID: "recovery-monitor"}, IsolationMeasures: []string{"关闭候选区段入口", "撤离全部访客", "关闭候选区段入口"}, MeasureCompletedAt: completedAt, Samples: []domain.SensorSample{{SensorID: "sensor-1", SampledAt: now.Add(-3 * time.Minute), TemperatureC: 15, RelativeHumidity: 74, CO2PPM: 900}}}
	if err := sendJSON(ctx, client, http.MethodPost, base+"/api/v1/trials/"+trialID+"/recovery", failedRecovery, http.StatusOK, nil); err != nil {
		return err
	}
	rev++
	recoverySamples := []domain.SensorSample{}
	for i := 0; i < 3; i++ {
		recoverySamples = append(recoverySamples, domain.SensorSample{SensorID: "sensor-1", SampledAt: now.Add(time.Duration(i-3) * time.Minute), TemperatureC: 14.2 - float64(i)*.05, RelativeHumidity: 71 - float64(i)*.2, CO2PPM: 620 - float64(i)*20})
	}
	recovery := application.RecoveryCommand{CommandMeta: application.CommandMeta{RequestID: "req-recovery", ExpectedRevision: rev, ActorID: "recovery-monitor"}, IsolationMeasures: []string{"撤离全部访客", "关闭候选区段入口"}, MeasureCompletedAt: completedAt, Samples: recoverySamples}
	if err := sendJSON(ctx, client, http.MethodPost, base+"/api/v1/trials/"+trialID+"/recovery", recovery, http.StatusOK, nil); err != nil {
		return err
	}
	rev++
	review := application.ReviewCommand{CommandMeta: application.CommandMeta{RequestID: "req-review", ExpectedRevision: rev, ActorID: "independent-reviewer"}, Approved: true, Checks: domain.ReviewChecks{StagesComplete: true, CalibrationsValid: true, AssessmentVerified: true, RecoveryVerified: true}, MaxConcurrentVisitors: 5, MaxStayMinutes: 15, ValidFrom: now.Add(time.Minute), ValidUntil: now.Add(30 * 24 * time.Hour)}
	if err := sendJSON(ctx, client, http.MethodPost, base+"/api/v1/trials/"+trialID+"/reviews", review, http.StatusOK, nil); err != nil {
		return err
	}
	var verification evidence.Verification
	if err := sendJSON(ctx, client, http.MethodGet, base+"/api/v1/trials/"+trialID+"/evidence/verification", nil, http.StatusOK, &verification); err != nil {
		return err
	}
	if !verification.Valid {
		return fmt.Errorf("证据校验失败: %+v", verification)
	}
	var timeline struct {
		Events []domain.AuditEvent `json:"events"`
		Valid  bool                `json:"valid"`
	}
	if err := sendJSON(ctx, client, http.MethodGet, base+"/api/v1/trials/"+trialID+"/timeline", nil, http.StatusOK, &timeline); err != nil {
		return err
	}
	if !timeline.Valid || len(timeline.Events) != 8 {
		return fmt.Errorf("审计时间线异常，事件数 %d", len(timeline.Events))
	}
	return nil
}
