package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Telemetry struct {
	DeviceID    int64     `json:"device_id"`
	Timestamp   time.Time `json:"timestamp"`
	SensorType  string    `json:"sensor_type"`
	ReadingType string    `json:"reading_type"`
	Value       float64   `json:"value"`
}

type publishFunc func(ctx context.Context, body []byte) error

func main() {
	rabbitURL := getEnv("RABBITMQ_URL", "amqp://guest:guest@rabbitmq:5672/")
	queueName := getEnv("RABBITMQ_QUEUE", "telemetry_queue")
	port := getEnv("PORT", "8080")

	conn, ch, q, err := setupRabbit(rabbitURL, queueName)
	if err != nil {
		log.Fatalf("failed to connect RabbitMQ: %v", err)
	}
	defer conn.Close()
	defer ch.Close()

	r := buildRouter(func(ctx context.Context, body []byte) error {
		return ch.PublishWithContext(
			ctx,
			"",
			q.Name,
			false,
			false,
			amqp.Publishing{
				ContentType: "application/json",
				Body:        body,
				Timestamp:   time.Now().UTC(),
			},
		)
	})

	log.Printf("api listening on :%s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("failed to run API: %v", err)
	}
}

func buildRouter(publish publishFunc) *gin.Engine {
	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.POST("/telemetry", func(c *gin.Context) {
		var payload Telemetry
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON payload"})
			return
		}

		if !isValidTelemetry(payload) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing or invalid required fields"})
			return
		}

		body, err := json.Marshal(payload)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encode payload"})
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := publish(ctx, body); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to publish message"})
			return
		}

		c.JSON(http.StatusAccepted, gin.H{"status": "queued"})
	})

	return r
}

func isValidTelemetry(payload Telemetry) bool {
	return payload.DeviceID > 0 &&
		!payload.Timestamp.IsZero() &&
		strings.TrimSpace(payload.SensorType) != "" &&
		strings.TrimSpace(payload.ReadingType) != ""
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
