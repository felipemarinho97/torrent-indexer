package logging

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// InitLogger initializes the global logger with zerolog
func InitLogger() {
	// Configure zerolog
	zerolog.TimeFieldFormat = time.RFC3339
	// Set log level from environment or default to Info
	level := zerolog.InfoLevel
	if envLevel := os.Getenv("LOG_LEVEL"); envLevel != "" {
		if parsedLevel, err := zerolog.ParseLevel(envLevel); err == nil {
			level = parsedLevel
		}
	}

	zerolog.SetGlobalLevel(level)

	// Use console writer for development, JSON for production
	if os.Getenv("LOG_FORMAT") != "json" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})
	}
}

// Info logs an info message with optional fields
func Info() *zerolog.Event {
	return log.Info()
}

// Debug logs a debug message with optional fields
func Debug() *zerolog.Event {
	return log.Debug()
}

// Error logs an error message with optional fields
func Error() *zerolog.Event {
	return log.Error()
}

// Warn logs a warning message with optional fields
func Warn() *zerolog.Event {
	return log.Warn()
}

// Fatal logs a fatal message and exits
func Fatal() *zerolog.Event {
	return log.Fatal()
}

// InfoWithRequest returns an info logger event with request context (IP, method, URL)
func InfoWithRequest(r *http.Request) *zerolog.Event {
	clientIP := getClientIP(r)
	return log.Info().
		Str("method", r.Method).
		Str("url", r.URL.String()).
		Str("client_ip", clientIP)
}

// ErrorWithRequest returns an error logger event with request context (IP, method, URL)
func ErrorWithRequest(r *http.Request) *zerolog.Event {
	clientIP := getClientIP(r)
	return log.Error().
		Str("method", r.Method).
		Str("url", r.URL.String()).
		Str("client_ip", clientIP)
}

// DebugWithRequest returns a debug logger event with request context (IP, method, URL)
func DebugWithRequest(r *http.Request) *zerolog.Event {
	clientIP := getClientIP(r)
	return log.Debug().
		Str("method", r.Method).
		Str("url", r.URL.String()).
		Str("client_ip", clientIP)
}

// WarnWithRequest returns a warn logger event with request context (IP, method, URL)
func WarnWithRequest(r *http.Request) *zerolog.Event {
	clientIP := getClientIP(r)
	return log.Warn().
		Str("method", r.Method).
		Str("url", r.URL.String()).
		Str("client_ip", clientIP)
}

// WithContext returns a logger event with context values
func WithContext(ctx context.Context) *zerolog.Event {
	event := log.Info()

	// Add any context values if needed
	if reqID := ctx.Value("request_id"); reqID != nil {
		if id, ok := reqID.(string); ok {
			event = event.Str("request_id", id)
		}
	}
	return event
}

// getClientIP extracts the real client IP address from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// Fall back to RemoteAddr
	return r.RemoteAddr
}
