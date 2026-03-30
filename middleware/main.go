package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Telemetry struct {
	DeviceID    int64     `json:"device_id"`
	Timestamp   time.Time `json:"timestamp"`
	SensorType  string    `json:"sensor_type"`
	ReadingType string    `json:"reading_type"`
	Value       float64   `json:"value"`
}

func main() {
	rabbitURL := getEnv("RABBITMQ_URL", "amqp://guest:guest@rabbitmq:5672/")
	queueName := getEnv("RABBITMQ_QUEUE", "telemetry_queue")
	dbDSN := getEnv("DATABASE_URL", "postgres://postgres:postgres@postgres:5432/telemetry?sslmode=disable")

	db, err := sql.Open("postgres", dbDSN)
	if err != nil {
		log.Fatalf("failed to open DB: %v", err)
	}
	defer db.Close()

	if err := waitForDB(db, 30, 2*time.Second); err != nil {
		log.Fatalf("database not ready: %v", err)
	}

	conn, ch, q, err := setupRabbit(rabbitURL, queueName)
	if err != nil {
		log.Fatalf("failed to connect RabbitMQ: %v", err)
	}
	defer conn.Close()
	defer ch.Close()

	if err := ch.Qos(50, 0, false); err != nil {
		log.Fatalf("failed to set QoS: %v", err)
	}

	messages, err := ch.Consume(
		q.Name,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		log.Fatalf("failed to register consumer: %v", err)
	}

	log.Printf("middleware consuming queue %q", q.Name)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for msg := range messages {
			var payload Telemetry
			if err := json.Unmarshal(msg.Body, &payload); err != nil {
				log.Printf("invalid message body: %v", err)
				_ = msg.Nack(false, false)
				continue
			}

			if err := insertTelemetry(db, payload); err != nil {
				log.Printf("failed to persist telemetry: %v", err)
				_ = msg.Nack(false, true)
				continue
			}

			_ = msg.Ack(false)
		}
	}()

	<-quit
	log.Println("shutting down middleware")

	_ = ch.Cancel("", false)
	<-done
}

func insertTelemetry(db *sql.DB, t Telemetry) error {
	_, err := db.Exec(
		`
		INSERT INTO telemetry (
			device_id,
			event_time,
			sensor_type,
			reading_type,
			value,
			captured_at
		) VALUES ($1, $2, $3, $4, $5, NOW())
		`,
		t.DeviceID,
		t.Timestamp,
		t.SensorType,
		t.ReadingType,
		t.Value,
	)
	return err
}

func waitForDB(db *sql.DB, attempts int, interval time.Duration) error {
	var lastErr error
	for i := 0; i < attempts; i++ {
		if err := db.Ping(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(interval)
	}
	return lastErr
}

func setupRabbit(url, queueName string) (*amqp.Connection, *amqp.Channel, amqp.Queue, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, nil, amqp.Queue{}, err
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, nil, amqp.Queue{}, err
	}

	q, err := ch.QueueDeclare(
		queueName,
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, nil, amqp.Queue{}, err
	}

	return conn, ch, q, nil
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
