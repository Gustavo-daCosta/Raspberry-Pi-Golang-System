package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestBuildRouterHealth(t *testing.T) {
	r := buildRouter(func(_ context.Context, _ []byte) error { return nil })

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}
}

func TestBuildRouterTelemetryAccepted(t *testing.T) {
	called := false
	r := buildRouter(func(_ context.Context, body []byte) error {
called = true
var got Telemetry
if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("failed to unmarshal body: %v", err)
		}
		if got.DeviceID != 1001 {
			t.Fatalf("expected device_id 1001, got %d", got.DeviceID)
		}
		return nil
	})

	payload := `{"device_id":1001,"timestamp":"2026-03-22T12:00:00Z","sensor_type":"temperatura","reading_type":"analogica","value":27.4}`
	req := httptest.NewRequest(http.MethodPost, "/telemetry", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")

	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)

	if res.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, res.Code)
	}
	if !called {
		t.Fatalf("expected publisher to be called")
	}
}

func TestBuildRouterTelemetryInvalidJSON(t *testing.T) {
	r := buildRouter(func(_ context.Context, _ []byte) error { return nil })

	req := httptest.NewRequest(http.MethodPost, "/telemetry", bytes.NewBufferString("{invalid"))
	req.Header.Set("Content-Type", "application/json")

	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, res.Code)
	}
}

func TestBuildRouterTelemetryMissingFields(t *testing.T) {
	r := buildRouter(func(_ context.Context, _ []byte) error { return nil })

	payload := `{"device_id":0,"timestamp":"2026-03-22T12:00:00Z","sensor_type":"temperatura","reading_type":"analogica","value":27.4}`
	req := httptest.NewRequest(http.MethodPost, "/telemetry", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")

	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, res.Code)
	}
}

func TestBuildRouterTelemetryPublishError(t *testing.T) {
	r := buildRouter(func(_ context.Context, _ []byte) error { return errors.New("publish failed") })

	payload := `{"device_id":1001,"timestamp":"2026-03-22T12:00:00Z","sensor_type":"temperatura","reading_type":"analogica","value":27.4}`
	req := httptest.NewRequest(http.MethodPost, "/telemetry", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")

	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)

	if res.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, res.Code)
	}
}

func TestIsValidTelemetry(t *testing.T) {
	valid := Telemetry{
		DeviceID:    1,
		Timestamp:   time.Now().UTC(),
		SensorType:  "temperatura",
		ReadingType: "analogica",
		Value:       10,
	}
	if !isValidTelemetry(valid) {
		t.Fatalf("expected telemetry to be valid")
	}

	invalid := valid
	invalid.ReadingType = ""
	if isValidTelemetry(invalid) {
		t.Fatalf("expected telemetry to be invalid")
	}
}

func TestGetEnv(t *testing.T) {
	const key = "UNIT_TEST_ENV_KEY"
	_ = os.Unsetenv(key)

	if got := getEnv(key, "fallback"); got != "fallback" {
		t.Fatalf("expected fallback, got %q", got)
	}

	if err := os.Setenv(key, "value"); err != nil {
		t.Fatalf("failed to set env: %v", err)
	}
	t.Cleanup(func() { _ = os.Unsetenv(key) })

	if got := getEnv(key, "fallback"); got != "value" {
		t.Fatalf("expected value, got %q", got)
	}
}
