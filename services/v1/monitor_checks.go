package v1

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"reacher-cron/client"
	"reacher-cron/models"
	"time"

	"github.com/go-redis/redis/v8"
)

const historySize = 1440

func doHealthCheck(m models.Monitor, rdb *redis.Client, db *sql.DB) {
	hc := &http.Client{Timeout: time.Duration(m.Timeout.Int64) * time.Millisecond}
	res, err := hc.Get(m.URL)
	key := fmt.Sprintf("monitor:%d", m.ID)
	var healthCheckStatus string
	if err != nil {
		healthCheckStatus = "down"
	} else {
		defer res.Body.Close()
		expected := 200
		if m.ExpectedStatus.Valid {
			expected = int(m.ExpectedStatus.Int64)
		}
		if res.StatusCode == expected {
			healthCheckStatus = "up"
		} else {
			healthCheckStatus = "down"
		}
	}
	timestamp := time.Now().Format(time.RFC3339)
	rdb.HSet(client.Ctx, key,
		"health_check", healthCheckStatus,
		"last_checked", timestamp,
	)
	log.Printf("[CRON] Monitor %s (ID: %d) => health_check: %s", m.Name, m.ID, healthCheckStatus)
	updateStatusHistory(rdb, key, healthCheckStatus, timestamp)

	ApplyIncidentRules(m.ID, m.AutoIncident, healthCheckStatus, m.ServiceDegradedThreshold, m.PartialOutageThreshold, m.MajorOutageThreshold, rdb, db)

}

func updateStatusHistory(rdb *redis.Client, key, status, timestamp string) {
	historyKey := fmt.Sprintf("%s:history", key)
	rdb.LPush(client.Ctx, historyKey, fmt.Sprintf("%s|%s", timestamp, status))
	rdb.LTrim(client.Ctx, historyKey, 0, historySize-1)
	updateUptime(rdb, historyKey, key)
}

func updateUptime(rdb *redis.Client, historyKey, key string) {
	history, err := rdb.LRange(client.Ctx, historyKey, 0, -1).Result()
	if err != nil {
		log.Printf("[REDIS] Error fetching history: %v", err)
		return
	}
	var upCount float64
	for _, entry := range history {
		if len(entry) >= 3 && entry[len(entry)-3:] == "|up" {
			upCount++
		}
	}
	uptime := (upCount / float64(len(history))) * 100
	rdb.HSet(client.Ctx, key, "uptime", fmt.Sprintf("%.2f", uptime))
}
