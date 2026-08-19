package response

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type errorBody struct {
	Error string `json:"error"`
}

func JSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Error("response: encode failed", "error", err)
	}
}

func Error(w http.ResponseWriter, status int, message string) {
	JSON(w, status, errorBody{Error: message})
}
