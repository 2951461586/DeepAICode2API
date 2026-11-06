package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

// --- HTTP 响应与脱敏工具 ---

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("写响应失败: %v", err)
	}
}

func writeErr(w http.ResponseWriter, code int, msg, typ string) {
	writeJSON(w, code, map[string]any{
		"error": map[string]any{"message": msg, "type": typ, "code": code},
	})
}

func contextWithTimeout(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}

func statusOf(err error) int {
	if err == nil {
		return http.StatusOK
	}
	return http.StatusBadGateway
}
