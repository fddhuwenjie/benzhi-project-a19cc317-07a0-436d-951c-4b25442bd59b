package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
)

const maxBodyBytes = 1 << 20

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return errors.New("Content-Type 必须为 application/json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return errors.New("请求 JSON 无效或包含未知字段: " + err.Error())
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return errors.New("请求体只能包含一个 JSON 对象")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeRawJSON(w http.ResponseWriter, status int, raw []byte, replay bool) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if replay {
		w.Header().Set("Idempotent-Replay", "true")
	}
	w.WriteHeader(status)
	_, _ = w.Write(raw)
	_, _ = w.Write([]byte("\n"))
}
