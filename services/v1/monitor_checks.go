package v1

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"reacher-cron/client"
	"reacher-cron/models"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
)

const historySize = 1440

func doHealthCheck(m models.Monitor, rdb *redis.Client, db *sql.DB) {
	// Verifica se Timeout está definido; se não, usa o valor padrão 5000 ms.
	timeout := 5000
	if m.Timeout != nil {
		timeout = *m.Timeout
	}
	hc := &http.Client{Timeout: time.Duration(timeout) * time.Millisecond}

	res, err := hc.Get(m.URL)
	key := fmt.Sprintf("monitor:%d", m.ID)
	var healthCheckStatus string
	if err != nil {
		healthCheckStatus = "down"
	} else {
		defer res.Body.Close()
		// Verifica se ExpectedStatus está definido; se não, usa o valor padrão 200.
		expected := 200
		if m.ExpectedStatus != nil {
			expected = *m.ExpectedStatus
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

	// Para aplicar regras de incidente, é importante converter os ponteiros.
	var autoIncident bool
	if m.AutoIncident != nil {
		autoIncident = *m.AutoIncident
	}
	ApplyIncidentRules(m.ID, autoIncident, healthCheckStatus, m.ServiceDegradedThreshold, m.PartialOutageThreshold, m.MajorOutageThreshold, rdb, db)
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
		// Considera "up" se a parte final for "|up"
		parts := strings.Split(entry, "|")
		if len(parts) == 2 && parts[1] == "up" {
			upCount++
		}
	}
	uptime := (upCount / float64(len(history))) * 100
	rdb.HSet(client.Ctx, key, "uptime", fmt.Sprintf("%.2f", uptime))
}
