package logging

import (
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

// ResponseWriter wraps http.ResponseWriter to capture status code
type ResponseWriter struct {
	http.ResponseWriter
	statusCode int
	written    int64
}

func NewResponseWriter(w http.ResponseWriter) *ResponseWriter {
	return &ResponseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK, // default to 200
	}
}

func (rw *ResponseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *ResponseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.written += int64(n)
	return n, err
}

// HTTPLoggingMiddleware logs HTTP requests in a structured format
func HTTPLoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := NewResponseWriter(w)
		clientIP := getClientIP(r)
		
		// Call the next handler
		next.ServeHTTP(rw, r)
		
		duration := time.Since(start)
		
		// Determine log level based on status code
		event := log.Info()
		if rw.statusCode >= 400 && rw.statusCode < 500 {
			event = log.Warn() // Client errors (4xx)
		} else if rw.statusCode >= 500 {
			event = log.Error() // Server errors (5xx)
		}
		
		// Build the log event
		logEvent := event.
			Str("client_ip", clientIP).
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Str("user_agent", r.UserAgent()).
			Int("status", rw.statusCode).
			Int64("bytes", rw.written).
			Dur("duration", duration)
		
		// Only add query if it's present
		if r.URL.RawQuery != "" {
			logEvent = logEvent.Str("query", r.URL.RawQuery)
		}
		
		// Log with a formatted message
		logEvent.Msgf("%s %s %d %dB %v", r.Method, r.URL.Path, rw.statusCode, rw.written, duration)
	})
}
