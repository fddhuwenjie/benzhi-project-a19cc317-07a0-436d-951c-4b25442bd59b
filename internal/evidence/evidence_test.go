package evidence_test

import (
	"context"
	"testing"
	"time"

	"cave-microclimate-clearance/internal/application"
	"cave-microclimate-clearance/internal/domain"
	"cave-microclimate-clearance/internal/evidence"
	"cave-microclimate-clearance/internal/store"
)

func TestRejectedEvidenceIsStableAndAnchored(t *testing.T) {
	repo, err := store.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := application.NewService(repo)
	ev := evidence.NewService(repo)
	now := time.Now().UTC().Truncate(time.Second)
	start := now.Add(-2 * time.Hour)
	end := now.Add(time.Hour)
	readings := []domain.BaselineReading{
		{SampledAt: start.Add(-11 * time.Minute), TemperatureC: 13.9, RelativeHumidity: 69.5, CO2PPM: 490},
		{SampledAt: start.Add(-6 * time.Minute), TemperatureC: 14, RelativeHumidity: 70, CO2PPM: 500},
		{SampledAt: start.Add(-time.Minute), TemperatureC: 14.1, RelativeHumidity: 70.5, CO2PPM: 510},
	}
	create := application.CreateTrialCommand{CommandMeta: application.CommandMeta{RequestID: "create", ActorID: "lead"}, TrialID: "rejected-evidence", CaveSectionID: "section", TestWindowStart: start, TestWindowEnd: end, LeadObserverID: "lead", Baseline: domain.BaselineProfile{Sensors: []domain.SensorBaseline{{SensorID: "s1", CalibrationRef: "cal", CalibrationValidTo: end.Add(time.Hour).Format(time.RFC3339), Readings: readings}}}}
	if _, err := app.CreateTrial(context.Background(), create); err != nil {
		t.Fatal(err)
	}
	revision := int64(1)
	for i, stage := range domain.PlannedStages {
		begin := start.Add(time.Duration(i*15) * time.Minute)
		samples := []domain.SensorSample{
			{SensorID: "s1", SampledAt: begin, TemperatureC: 14.1, RelativeHumidity: 70.5, CO2PPM: 550},
			{SensorID: "s1", SampledAt: begin.Add(5 * time.Minute), TemperatureC: 14.2, RelativeHumidity: 71, CO2PPM: 575},
			{SensorID: "s1", SampledAt: begin.Add(10 * time.Minute), TemperatureC: 14.3, RelativeHumidity: 71.5, CO2PPM: 600},
		}
		command := application.ObservationCommand{CommandMeta: application.CommandMeta{RequestID: "stage-" + string(stage), ExpectedRevision: revision, ActorID: "sampler"}, Observation: domain.LoadStageObservation{Stage: stage, VisitorCount: (i + 1) * 3, DurationMinutes: 10, SamplingIntervalSeconds: 300, Samples: samples, ObserverID: "sampler", StartedAt: begin, EndedAt: begin.Add(10 * time.Minute)}}
		if _, err := app.AddObservation(context.Background(), "rejected-evidence", command); err != nil {
			t.Fatal(err)
		}
		revision++
	}
	if _, err := app.Assess(context.Background(), "rejected-evidence", application.AssessmentCommand{CommandMeta: application.CommandMeta{RequestID: "assess", ExpectedRevision: revision, ActorID: "lead"}}); err != nil {
		t.Fatal(err)
	}
	revision++
	review := application.ReviewCommand{CommandMeta: application.CommandMeta{RequestID: "review", ExpectedRevision: revision, ActorID: "reviewer"}, Approved: false, Checks: domain.ReviewChecks{StagesComplete: true, CalibrationsValid: true, AssessmentVerified: true, RecoveryVerified: true}, RejectionReasons: []domain.RejectionReason{{Code: "conservation_hold", Detail: "保护性暂缓"}, {Code: "seasonal_risk", Detail: "季节风险"}}}
	if _, err := app.Review(context.Background(), "rejected-evidence", review); err != nil {
		t.Fatal(err)
	}
	first, err := ev.Build(context.Background(), "rejected-evidence")
	if err != nil {
		t.Fatal(err)
	}
	second, err := ev.Build(context.Background(), "rejected-evidence")
	if err != nil {
		t.Fatal(err)
	}
	if first.GeneratedAt != second.GeneratedAt || first.ContentDigest != second.ContentDigest || first.Trial.RejectionEvidenceDigest != first.ContentDigest {
		t.Fatalf("拒绝证据包不稳定或未锚定: %#v %#v", first, second)
	}
	if verification := evidence.VerifyPackage(first); !verification.Valid || !verification.TerminalDigestMatches {
		t.Fatalf("拒绝证据校验失败: %+v", verification)
	}
	if reconciliation := evidence.ReconcileSemantics(first.Trial, first.AuditEvents); !reconciliation.Valid || len(reconciliation.EventCounts) == 0 {
		t.Fatalf("完整业务事实应通过语义对账: %+v", reconciliation)
	}
	missingMedium := append([]domain.AuditEvent(nil), first.AuditEvents...)
	missingMedium = append(missingMedium[:2], missingMedium[3:]...)
	reconciliation := evidence.ReconcileSemantics(first.Trial, missingMedium)
	if reconciliation.Valid || len(reconciliation.Issues) == 0 || reconciliation.Issues[0].Code != "missing_event" || reconciliation.Issues[0].ExpectedEventType != "load_stage_observed_medium" {
		t.Fatalf("缺少中负荷事件应被准确定位: %+v", reconciliation)
	}
	wrongReviewer := append([]domain.AuditEvent(nil), first.AuditEvents...)
	wrongReviewer[len(wrongReviewer)-1].ActorID = "field-sampler"
	reconciliation = evidence.ReconcileSemantics(first.Trial, wrongReviewer)
	if reconciliation.Valid || reconciliation.Issues[0].Code != "actor_identity_mismatch" || reconciliation.Issues[0].FactReference != "review:rejected" {
		t.Fatalf("终态复核身份不匹配应被定位: %+v", reconciliation)
	}
	first.Trial.RejectionReasons[0].Detail = "被替换"
	if verification := evidence.VerifyPackage(first); verification.Valid || verification.ContentDigestMatches {
		t.Fatalf("改变拒绝原因后应识别内容摘要失配: %+v", verification)
	}
	first.ContentDigest = domain.EvidenceContentDigest(first.Trial, first.AuditEvents)
	if verification := evidence.VerifyPackage(first); verification.Valid || verification.TerminalDigestMatches {
		t.Fatalf("重算内容摘要后仍应识别拒绝终态锚点失配: %+v", verification)
	}
}
