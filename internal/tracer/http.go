package tracer

import (
	"net/http"

	"github.com/GoogleCloudPlatform/opentelemetry-operations-go/propagator"
	"go.opentelemetry.io/otel/propagation"
)

func XCTCMiddleware() func(http.Handler) http.Handler {
	prop := propagator.New()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := prop.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}
}
