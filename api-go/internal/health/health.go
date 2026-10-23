// Package health espelha o GET /actuator/health do Spring Boot, que o deploy usa
// como healthcheck.
package health

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
)

// O Actuator NAO responde application/json: o content-type e versionado. Medido
// em producao e gravado no golden actuator-health.
const contentType = "application/vnd.spring-boot.actuator.v3+json"

type Pinger interface {
	Ping(ctx context.Context) error
}

type status struct {
	Status string `json:"status"`
}

func Handler(db Pinger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			slog.Error("healthcheck: banco inacessivel", "err", err)
			httpx.WriteJSONAs(w, http.StatusServiceUnavailable, contentType, status{"DOWN"})
			return
		}
		httpx.WriteJSONAs(w, http.StatusOK, contentType, status{"UP"})
	})
}
