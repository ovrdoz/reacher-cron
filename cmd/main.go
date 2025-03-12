package main

import (
	"reacher-cron/api"
	"reacher-cron/config"
	v1 "reacher-cron/services/v1"
)

func main() {
	config.LoadConfig()

	// Start monitor cron scheduler in a separate goroutine
	go v1.StartGlobalMonitorScheduler()

	// Start HTTP server for API endpoints (e.g., health check)
	api.StartServer()

	// Prevents main from exiting (ensures cron keeps running)
	select {}
}
