package main

import (
	"errors"
	"os"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestInsertTelemetrySuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	payload := Telemetry{
		DeviceID:    1001,
		Timestamp:   time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC),
		SensorType:  "temperatura",
		ReadingType: "analogica",
		Value:       27.4,
	}

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO telemetry (")).
		WithArgs(payload.DeviceID, payload.Timestamp, payload.SensorType, payload.ReadingType, payload.Value).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := insertTelemetry(db, payload); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestInsertTelemetryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	payload := Telemetry{
		DeviceID:    1001,
		Timestamp:   time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC),
		SensorType:  "temperatura",
		ReadingType: "analogica",
		Value:       27.4,
	}

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO telemetry (")).
		WithArgs(payload.DeviceID, payload.Timestamp, payload.SensorType, payload.ReadingType, payload.Value).
		WillReturnError(errors.New("db failed"))

	if err := insertTelemetry(db, payload); err == nil {
		t.Fatalf("expected error, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestWaitForDBSuccess(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectPing().WillReturnError(errors.New("not ready"))
	mock.ExpectPing().WillReturnError(errors.New("not ready"))
	mock.ExpectPing()

	err = waitForDB(db, 3, 0)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestWaitForDBFailure(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectPing().WillReturnError(errors.New("not ready"))
	mock.ExpectPing().WillReturnError(errors.New("still not ready"))

	err = waitForDB(db, 2, 0)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGetEnv(t *testing.T) {
	const key = "UNIT_TEST_MW_ENV_KEY"
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
