package server

import (
	"log"
	"net/http"
	"os"
	"time"
)

var (
	isCheckerRunning               bool
	consecutiveHealthCheckFailures int = 0
	lastHealthLog                  time.Time
)

const (
	maxConsecutiveHealthCheckFailures int = 5
)

func selfHealthCheck(healthCheckURL string) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(healthCheckURL)
	if err != nil {
		consecutiveHealthCheckFailures++
		log.Printf("Health check failed: %v (consecutive failures: %d)", err, consecutiveHealthCheckFailures)
		if consecutiveHealthCheckFailures >= 5 {
			log.Fatal("Health check failed 5 consecutive times, exiting...")
		}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		consecutiveHealthCheckFailures++
		log.Printf("Health check failed with status code: %d (consecutive failures: %d)", resp.StatusCode, consecutiveHealthCheckFailures)
		if consecutiveHealthCheckFailures >= maxConsecutiveHealthCheckFailures {
			log.Fatal("Health check failed 10 consecutive times, exiting...")
			os.Exit(0)
		}
	} else {
		// Reset counter on successful health check
		consecutiveHealthCheckFailures = 0
		now := time.Now()
		if lastHealthLog.IsZero() || now.Sub(lastHealthLog) >= time.Hour {
			lastHealthLog = now
		}
	}
}

func StartSelfHealthCheck(healthCheckURL string) {
	if isCheckerRunning {
		log.Printf("Self health check is already running.")
		return
	}
	isCheckerRunning = true
	selfHealthChecker := time.NewTicker(30 * time.Second)
	defer selfHealthChecker.Stop()
	for range selfHealthChecker.C {
		selfHealthCheck(healthCheckURL)
	}
}
