package httpapi

import (
	"net/http"

	"cave-microclimate-clearance/internal/application"
	"cave-microclimate-clearance/internal/evidence"
)

type API struct {
	app      *application.Service
	evidence *evidence.Service
	mux      *http.ServeMux
}

func New(app *application.Service, ev *evidence.Service) *API {
	a := &API{app: app, evidence: ev, mux: http.NewServeMux()}
	a.routes()
	return a
}

func (a *API) routes() {
	a.mux.HandleFunc("GET /healthz", a.HealthHandler)
	a.mux.HandleFunc("POST /api/v1/trials", a.CreateTrialHandler)
	a.mux.HandleFunc("GET /api/v1/trials/{trial_id}", a.GetTrialHandler)
	a.mux.HandleFunc("POST /api/v1/trials/{trial_id}/observations", a.AddObservationHandler)
	a.mux.HandleFunc("POST /api/v1/trials/{trial_id}/assessment", a.AssessmentHandler)
	a.mux.HandleFunc("POST /api/v1/trials/{trial_id}/recovery", a.RecoveryHandler)
	a.mux.HandleFunc("POST /api/v1/trials/{trial_id}/reviews", a.ReviewHandler)
	a.mux.HandleFunc("GET /api/v1/trials/{trial_id}/timeline", a.TimelineHandler)
	a.mux.HandleFunc("GET /api/v1/trials/{trial_id}/evidence", a.EvidenceHandler)
	a.mux.HandleFunc("GET /api/v1/trials/{trial_id}/evidence/verification", a.EvidenceVerificationHandler)
}

func (a *API) Handler() http.Handler { return security(a.mux) }

func security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'")
		next.ServeHTTP(w, r)
	})
}
