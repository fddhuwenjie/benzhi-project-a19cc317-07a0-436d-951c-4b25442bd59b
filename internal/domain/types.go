package domain

import "time"

type Status string

const (
	StatusBaselineFrozen Status = "baseline_frozen"
	StatusSampling       Status = "sampling"
	StatusReadyAssess    Status = "ready_for_assessment"
	StatusPaused         Status = "paused"
	StatusReadyReview    Status = "ready_for_review"
	StatusPermitted      Status = "permitted"
	StatusRejected       Status = "rejected"
)

type Stage string

const (
	StageLow    Stage = "low"
	StageMedium Stage = "medium"
	StageHigh   Stage = "high"
)

var PlannedStages = []Stage{StageLow, StageMedium, StageHigh}

type BaselineReading struct {
	SampledAt        time.Time `json:"sampled_at"`
	TemperatureC     float64   `json:"temperature_c"`
	RelativeHumidity float64   `json:"relative_humidity_pct"`
	CO2PPM           float64   `json:"co2_ppm"`
}

type MetricStability struct {
	Metric             string  `json:"metric"`
	Median             float64 `json:"median"`
	Minimum            float64 `json:"minimum"`
	Maximum            float64 `json:"maximum"`
	Fluctuation        float64 `json:"fluctuation"`
	AllowedFluctuation float64 `json:"allowed_fluctuation"`
	Stable             bool    `json:"stable"`
}

type BaselineStabilitySummary struct {
	PointCount          int               `json:"point_count"`
	ObservedSpanSeconds int64             `json:"observed_span_seconds"`
	RequiredSpanSeconds int64             `json:"required_span_seconds"`
	Stable              bool              `json:"stable"`
	Metrics             []MetricStability `json:"metrics"`
}

type BaselineSensorTimeBoundary struct {
	SensorID         string    `json:"sensor_id"`
	FirstSampledAt   time.Time `json:"first_sampled_at"`
	LastSampledAt    time.Time `json:"last_sampled_at"`
	StalenessSeconds int64     `json:"staleness_seconds"`
}

type BaselineSynchronizationSummary struct {
	CommonCoverageStart              time.Time                    `json:"common_coverage_start"`
	CommonCoverageEnd                time.Time                    `json:"common_coverage_end"`
	CommonCoverageSeconds            int64                        `json:"common_coverage_seconds"`
	MinimumCommonSpanSeconds         int64                        `json:"minimum_common_span_seconds"`
	MaximumStalenessSeconds          int64                        `json:"maximum_staleness_seconds"`
	AllowedMaximumStalenessSeconds   int64                        `json:"allowed_maximum_staleness_seconds"`
	AlignmentDeviationSeconds        int64                        `json:"alignment_deviation_seconds"`
	AllowedAlignmentDeviationSeconds int64                        `json:"allowed_alignment_deviation_seconds"`
	CoverageStartDecisiveSensorID    string                       `json:"coverage_start_decisive_sensor_id"`
	CoverageEndDecisiveSensorID      string                       `json:"coverage_end_decisive_sensor_id"`
	StalenessDecisiveSensorID        string                       `json:"staleness_decisive_sensor_id"`
	AlignmentEarliestSensorID        string                       `json:"alignment_earliest_sensor_id"`
	AlignmentLatestSensorID          string                       `json:"alignment_latest_sensor_id"`
	SensorBoundaries                 []BaselineSensorTimeBoundary `json:"sensor_boundaries"`
}

type SensorBaseline struct {
	SensorID           string                   `json:"sensor_id"`
	CalibrationRef     string                   `json:"calibration_ref"`
	CalibrationValidTo string                   `json:"calibration_valid_to"`
	Readings           []BaselineReading        `json:"readings"`
	TemperatureC       float64                  `json:"temperature_c"`
	RelativeHumidity   float64                  `json:"relative_humidity_pct"`
	CO2PPM             float64                  `json:"co2_ppm"`
	Stability          BaselineStabilitySummary `json:"stability"`
}

type BaselineProfile struct {
	BaselineID              string                         `json:"baseline_id"`
	SampledAt               time.Time                      `json:"sampled_at"`
	ThresholdProfileVersion string                         `json:"threshold_profile_version"`
	Sensors                 []SensorBaseline               `json:"sensors"`
	Synchronization         BaselineSynchronizationSummary `json:"synchronization"`
	FrozenAt                time.Time                      `json:"frozen_at"`
}

type SensorSample struct {
	SensorID         string    `json:"sensor_id"`
	SampledAt        time.Time `json:"sampled_at"`
	TemperatureC     float64   `json:"temperature_c"`
	RelativeHumidity float64   `json:"relative_humidity_pct"`
	CO2PPM           float64   `json:"co2_ppm"`
}

type SensorCoverage struct {
	SensorID           string  `json:"sensor_id"`
	ExpectedPoints     int     `json:"expected_points"`
	ReceivedPoints     int     `json:"received_points"`
	MissingPoints      int     `json:"missing_points"`
	CoveragePercent    float64 `json:"coverage_percent"`
	FirstOffsetSeconds int64   `json:"first_offset_seconds"`
	LastOffsetSeconds  int64   `json:"last_offset_seconds"`
	MaxGapSeconds      int64   `json:"max_gap_seconds"`
	Complete           bool    `json:"complete"`
}

type SamplingCompletenessSummary struct {
	SamplingIntervalSeconds   int              `json:"sampling_interval_seconds"`
	Sensors                   []SensorCoverage `json:"sensors"`
	CommonCoverageStart       time.Time        `json:"common_coverage_start"`
	CommonCoverageEnd         time.Time        `json:"common_coverage_end"`
	CommonCoverageSeconds     int64            `json:"common_coverage_seconds"`
	RequiredCoverageSeconds   int64            `json:"required_coverage_seconds"`
	AlignmentDeviationSeconds int64            `json:"alignment_deviation_seconds"`
	Complete                  bool             `json:"complete"`
}

type LoadStageObservation struct {
	ObservationID           string                      `json:"observation_id"`
	Stage                   Stage                       `json:"stage"`
	VisitorCount            int                         `json:"visitor_count"`
	DurationMinutes         int                         `json:"duration_minutes"`
	SamplingIntervalSeconds int                         `json:"sampling_interval_seconds"`
	Samples                 []SensorSample              `json:"samples"`
	Coverage                SamplingCompletenessSummary `json:"coverage"`
	ObserverID              string                      `json:"observer_id"`
	StartedAt               time.Time                   `json:"started_at"`
	EndedAt                 time.Time                   `json:"ended_at"`
}

type Thresholds struct {
	MaxTemperatureDeltaC         float64 `json:"max_temperature_delta_c"`
	MaxHumidityDeltaPct          float64 `json:"max_humidity_delta_pct"`
	MaxCO2PPM                    float64 `json:"max_co2_ppm"`
	RecoveryTempDeltaC           float64 `json:"recovery_temperature_delta_c"`
	RecoveryHumidityDelta        float64 `json:"recovery_humidity_delta_pct"`
	RecoveryCO2PPM               float64 `json:"recovery_co2_ppm"`
	RecoveryPoints               int     `json:"recovery_points"`
	BaselineMinPoints            int     `json:"baseline_min_points"`
	BaselineMinSpanMinutes       int     `json:"baseline_min_span_minutes"`
	MaxBaselineTemperatureRangeC float64 `json:"max_baseline_temperature_range_c"`
	MaxBaselineHumidityRangePct  float64 `json:"max_baseline_humidity_range_pct"`
	MaxBaselineCO2RangePPM       float64 `json:"max_baseline_co2_range_ppm"`
	MaxBaselineStalenessMinutes  int     `json:"max_baseline_staleness_minutes"`
	MaxBaselineAlignmentSeconds  int     `json:"max_baseline_alignment_seconds"`
	MinStageRestMinutes          int     `json:"min_stage_rest_minutes"`
	MaxSamplingAlignmentSeconds  int     `json:"max_sampling_alignment_seconds"`
	RecoveryMinMinutes           int     `json:"recovery_min_minutes"`
	RecoveryMaxGapSeconds        int     `json:"recovery_max_gap_seconds"`
}

func DefaultThresholds() Thresholds {
	return Thresholds{MaxTemperatureDeltaC: 1.5, MaxHumidityDeltaPct: 6, MaxCO2PPM: 1200, RecoveryTempDeltaC: .4, RecoveryHumidityDelta: 2, RecoveryCO2PPM: 700, RecoveryPoints: 3, BaselineMinPoints: 3, BaselineMinSpanMinutes: 10, MaxBaselineTemperatureRangeC: .6, MaxBaselineHumidityRangePct: 3, MaxBaselineCO2RangePPM: 120, MaxBaselineStalenessMinutes: 120, MaxBaselineAlignmentSeconds: 60, MinStageRestMinutes: 5, MaxSamplingAlignmentSeconds: 30, RecoveryMinMinutes: 2, RecoveryMaxGapSeconds: 90}
}

type ExposureMetric struct {
	Metric                      string  `json:"metric"`
	IntegratedExposure          float64 `json:"integrated_exposure"`
	ObservedMinutes             float64 `json:"observed_minutes"`
	NormalizedExposurePerMinute float64 `json:"normalized_exposure_per_minute"`
}

type SensorStageExposure struct {
	SensorID string           `json:"sensor_id"`
	Metrics  []ExposureMetric `json:"metrics"`
}

type StageExposure struct {
	Stage   Stage                 `json:"stage"`
	Sensors []SensorStageExposure `json:"sensors"`
}

type ExposureTrend struct {
	Metric                string  `json:"metric"`
	Conclusion            string  `json:"conclusion"`
	DecisiveSensorID      string  `json:"decisive_sensor_id"`
	LargestJumpFromStage  Stage   `json:"largest_jump_from_stage"`
	LargestJumpToStage    Stage   `json:"largest_jump_to_stage"`
	LargestNormalizedJump float64 `json:"largest_normalized_jump"`
}

type MetricResult struct {
	Metric            string    `json:"metric"`
	Observed          float64   `json:"observed"`
	Limit             float64   `json:"limit"`
	Passed            bool      `json:"passed"`
	Conclusion        string    `json:"conclusion"`
	DecisiveStage     Stage     `json:"decisive_stage"`
	DecisiveSensorID  string    `json:"decisive_sensor_id"`
	DecisiveSampledAt time.Time `json:"decisive_sampled_at"`
	AbsoluteMargin    float64   `json:"absolute_margin"`
	PercentMargin     float64   `json:"percent_margin"`
}

type SensorThresholdResult struct {
	SensorID  string         `json:"sensor_id"`
	Metrics   []MetricResult `json:"metrics"`
	AllPassed bool           `json:"all_passed"`
}

type StageThresholdResult struct {
	Stage     Stage                   `json:"stage"`
	Sensors   []SensorThresholdResult `json:"sensors"`
	AllPassed bool                    `json:"all_passed"`
}

type ThresholdAssessment struct {
	AssessmentID        string                 `json:"assessment_id"`
	RuleVersion         string                 `json:"rule_version"`
	RuleSummary         string                 `json:"rule_summary"`
	MetricResults       []MetricResult         `json:"metric_results"`
	StageResults        []StageThresholdResult `json:"stage_results"`
	StageExposures      []StageExposure        `json:"stage_exposures"`
	ExposureTrends      []ExposureTrend        `json:"exposure_trends"`
	ExposureRuleVersion string                 `json:"exposure_rule_version"`
	ExposureInputDigest string                 `json:"exposure_input_digest"`
	FirstExceededStage  *Stage                 `json:"first_exceeded_stage,omitempty"`
	LastAllPassedStage  *Stage                 `json:"last_all_passed_stage,omitempty"`
	StopRequired        bool                   `json:"stop_required"`
	RecoveryRequired    bool                   `json:"recovery_required"`
	InputDigest         string                 `json:"input_digest"`
	EvaluatedAt         time.Time              `json:"evaluated_at"`
}

type RecoverySensorConclusion struct {
	SensorID            string   `json:"sensor_id"`
	ReceivedPoints      int      `json:"received_points"`
	ContinuousPoints    int      `json:"continuous_points"`
	ObservedSpanSeconds int64    `json:"observed_span_seconds"`
	Trend               string   `json:"trend"`
	Passed              bool     `json:"passed"`
	FailureReasons      []string `json:"failure_reasons"`
}

type RecoveryRecord struct {
	AttemptID           string                     `json:"attempt_id"`
	IsolationMeasures   []string                   `json:"isolation_measures"`
	IsolationExecutorID string                     `json:"isolation_executor_id"`
	MeasureCompletedAt  time.Time                  `json:"measure_completed_at"`
	Samples             []SensorSample             `json:"samples"`
	ObserverID          string                     `json:"observer_id"`
	SensorConclusions   []RecoverySensorConclusion `json:"sensor_conclusions"`
	FailureReasons      []string                   `json:"failure_reasons"`
	VerifiedAt          time.Time                  `json:"verified_at"`
	Passed              bool                       `json:"passed"`
}

type ReviewChecks struct {
	StagesComplete     bool `json:"stages_complete"`
	CalibrationsValid  bool `json:"calibrations_valid"`
	AssessmentVerified bool `json:"assessment_verified"`
	RecoveryVerified   bool `json:"recovery_verified"`
}

type ReconciliationItem struct {
	Code               string   `json:"code"`
	Declared           bool     `json:"declared"`
	Computed           bool     `json:"computed"`
	Matched            bool     `json:"matched"`
	EvidenceReferences []string `json:"evidence_references"`
}

type ReviewReconciliation struct {
	Items                []ReconciliationItem `json:"items"`
	AllFactsSatisfied    bool                 `json:"all_facts_satisfied"`
	AllDeclarationsMatch bool                 `json:"all_declarations_match"`
	ComputedAt           time.Time            `json:"computed_at"`
}

type ReviewDecision struct {
	ReviewerID       string               `json:"reviewer_id"`
	Approved         bool                 `json:"approved"`
	Checks           ReviewChecks         `json:"checks"`
	Reconciliation   ReviewReconciliation `json:"reconciliation"`
	RejectionReasons []RejectionReason    `json:"rejection_reasons,omitempty"`
	ReviewedAt       time.Time            `json:"reviewed_at"`
}

type RejectionReason struct {
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

type ClearancePermit struct {
	PermitID              string         `json:"permit_id"`
	ReviewerID            string         `json:"reviewer_id"`
	MaxConcurrentVisitors int            `json:"max_concurrent_visitors"`
	MaxStayMinutes        int            `json:"max_stay_minutes"`
	StopThresholds        Thresholds     `json:"stop_thresholds"`
	BasisStage            Stage          `json:"basis_stage"`
	BasisSafetyMargins    []MetricResult `json:"basis_safety_margins"`
	DerivationRuleVersion string         `json:"derivation_rule_version"`
	ValidFrom             time.Time      `json:"valid_from"`
	ValidUntil            time.Time      `json:"valid_until"`
	EvidenceDigest        string         `json:"evidence_digest"`
}

type AuditEvent struct {
	EventID        string    `json:"event_id"`
	TrialID        string    `json:"trial_id"`
	Sequence       int64     `json:"sequence"`
	EventType      string    `json:"event_type"`
	ActorID        string    `json:"actor_id"`
	RequestID      string    `json:"request_id"`
	PayloadDigest  string    `json:"payload_digest"`
	PreviousDigest string    `json:"previous_digest"`
	EventDigest    string    `json:"event_digest"`
	OccurredAt     time.Time `json:"occurred_at"`
}

type ClearanceTrial struct {
	TrialID                 string                 `json:"trial_id"`
	CaveSectionID           string                 `json:"cave_section_id"`
	Status                  Status                 `json:"status"`
	TestWindowStart         time.Time              `json:"test_window_start"`
	TestWindowEnd           time.Time              `json:"test_window_end"`
	LeadObserverID          string                 `json:"lead_observer_id"`
	Revision                int64                  `json:"revision"`
	CreatedAt               time.Time              `json:"created_at"`
	FinalizedAt             *time.Time             `json:"finalized_at,omitempty"`
	Baseline                BaselineProfile        `json:"baseline"`
	Thresholds              Thresholds             `json:"thresholds"`
	Observations            []LoadStageObservation `json:"observations"`
	Assessment              *ThresholdAssessment   `json:"assessment,omitempty"`
	Recovery                *RecoveryRecord        `json:"recovery,omitempty"`
	RecoveryAttempts        []RecoveryRecord       `json:"recovery_attempts,omitempty"`
	Review                  *ReviewDecision        `json:"review,omitempty"`
	Permit                  *ClearancePermit       `json:"permit,omitempty"`
	RejectionReasons        []RejectionReason      `json:"rejection_reasons,omitempty"`
	RejectionEvidenceDigest string                 `json:"rejection_evidence_digest,omitempty"`
}

func (t *ClearanceTrial) Terminal() bool {
	return t.Status == StatusPermitted || t.Status == StatusRejected
}
