package httpapi

import (
	"errors"
	"net/http"

	"cave-microclimate-clearance/internal/domain"
	"cave-microclimate-clearance/internal/store"
)

type Problem struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
	Details any    `json:"details,omitempty"`
}

func writeProblem(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	p := Problem{Code: "internal_error", Message: "服务处理请求失败"}
	var de *domain.Error
	var rc *store.RevisionConflict
	var ic *store.IdempotencyConflict
	var corrupt *store.CorruptError
	switch {
	case errors.As(err, &de):
		p = Problem{Code: string(de.Code), Message: de.Message, Field: de.Field, Details: de.Details}
		switch de.Code {
		case domain.CodeValidation:
			status = http.StatusUnprocessableEntity
		case domain.CodeInvalidState:
			status = http.StatusConflict
		case domain.CodeCorrupt:
			status = http.StatusServiceUnavailable
		default:
			status = http.StatusBadRequest
		}
	case errors.As(err, &rc):
		status = http.StatusConflict
		p = Problem{Code: "revision_conflict", Message: rc.Error()}
	case errors.As(err, &ic):
		status = http.StatusConflict
		p = Problem{Code: "idempotency_conflict", Message: ic.Error()}
	case errors.Is(err, store.ErrNotFound):
		status = http.StatusNotFound
		p = Problem{Code: "not_found", Message: "未找到指定试验"}
	case errors.As(err, &corrupt):
		status = http.StatusServiceUnavailable
		p = Problem{Code: "storage_corrupt", Message: corrupt.Error()}
	}
	writeJSON(w, status, p)
}
