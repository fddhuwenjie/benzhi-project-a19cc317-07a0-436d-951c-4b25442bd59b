package httpapi

import (
	"context"
	"net/http"

	"cave-microclimate-clearance/internal/application"
	"cave-microclimate-clearance/internal/domain"
)

func trialID(r *http.Request) string { return r.PathValue("trial_id") }

func (a *API) prepareTerminalEvidence(ctx context.Context, id string) error {
	if _, err := a.evidence.Build(ctx, id); err != nil {
		return err
	}
	verification, err := a.evidence.VerifyCurrent(ctx, id)
	if err != nil {
		return err
	}
	if !verification.Valid {
		return domain.InvalidState("终态证据校验未通过")
	}
	return nil
}

func (a *API) HealthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) CreateTrialHandler(w http.ResponseWriter, r *http.Request) {
	var c application.CreateTrialCommand
	if err := decodeJSON(w, r, &c); err != nil {
		writeJSON(w, http.StatusBadRequest, Problem{Code: "invalid_json", Message: err.Error()})
		return
	}
	result, err := a.app.CreateTrial(r.Context(), c)
	if err != nil {
		writeProblem(w, err)
		return
	}
	writeRawJSON(w, http.StatusCreated, result.ResponseJSON, result.Replayed)
}

func (a *API) AddObservationHandler(w http.ResponseWriter, r *http.Request) {
	var c application.ObservationCommand
	if err := decodeJSON(w, r, &c); err != nil {
		writeJSON(w, http.StatusBadRequest, Problem{Code: "invalid_json", Message: err.Error()})
		return
	}
	result, err := a.app.AddObservation(r.Context(), trialID(r), c)
	if err != nil {
		writeProblem(w, err)
		return
	}
	writeRawJSON(w, http.StatusOK, result.ResponseJSON, result.Replayed)
}

func (a *API) AssessmentHandler(w http.ResponseWriter, r *http.Request) {
	var c application.AssessmentCommand
	if err := decodeJSON(w, r, &c); err != nil {
		writeJSON(w, http.StatusBadRequest, Problem{Code: "invalid_json", Message: err.Error()})
		return
	}
	result, err := a.app.Assess(r.Context(), trialID(r), c)
	if err != nil {
		writeProblem(w, err)
		return
	}
	writeRawJSON(w, http.StatusOK, result.ResponseJSON, result.Replayed)
}

func (a *API) RecoveryHandler(w http.ResponseWriter, r *http.Request) {
	var c application.RecoveryCommand
	if err := decodeJSON(w, r, &c); err != nil {
		writeJSON(w, http.StatusBadRequest, Problem{Code: "invalid_json", Message: err.Error()})
		return
	}
	result, err := a.app.VerifyRecovery(r.Context(), trialID(r), c)
	if err != nil {
		writeProblem(w, err)
		return
	}
	writeRawJSON(w, http.StatusOK, result.ResponseJSON, result.Replayed)
}

func (a *API) ReviewHandler(w http.ResponseWriter, r *http.Request) {
	var c application.ReviewCommand
	if err := decodeJSON(w, r, &c); err != nil {
		writeJSON(w, http.StatusBadRequest, Problem{Code: "invalid_json", Message: err.Error()})
		return
	}
	result, err := a.app.Review(r.Context(), trialID(r), c)
	if err != nil {
		writeProblem(w, err)
		return
	}
	if result.Trial.Terminal() {
		if err := a.prepareTerminalEvidence(r.Context(), trialID(r)); err != nil {
			writeProblem(w, err)
			return
		}
	}
	writeRawJSON(w, http.StatusOK, result.ResponseJSON, result.Replayed)
}

func (a *API) GetTrialHandler(w http.ResponseWriter, r *http.Request) {
	value, err := a.app.GetTrial(r.Context(), trialID(r))
	if err != nil {
		writeProblem(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
func (a *API) TimelineHandler(w http.ResponseWriter, r *http.Request) {
	value, err := a.app.GetTimeline(r.Context(), trialID(r))
	if err != nil {
		writeProblem(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
func (a *API) EvidenceHandler(w http.ResponseWriter, r *http.Request) {
	value, err := a.evidence.Build(r.Context(), trialID(r))
	if err != nil {
		writeProblem(w, err)
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename=clearance-evidence.json")
	writeJSON(w, http.StatusOK, value)
}
func (a *API) EvidenceVerificationHandler(w http.ResponseWriter, r *http.Request) {
	value, err := a.evidence.VerifyCurrent(r.Context(), trialID(r))
	if err != nil {
		writeProblem(w, err)
		return
	}
	status := http.StatusOK
	if !value.Valid {
		status = http.StatusConflict
	}
	writeJSON(w, status, value)
}
