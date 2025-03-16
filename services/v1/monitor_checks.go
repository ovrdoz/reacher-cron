package v1

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"reacher-cron/client"
	"reacher-cron/models"

	"github.com/go-redis/redis/v8"
)

// doHealthCheck executa o health check para um monitor,
// usando as regras e, em seguida, chamando ProcessIncidentCreation se necessário.
func doHealthCheck(m models.Monitor, rdb *redis.Client, db *sql.DB) {
	startTime := time.Now()
	resp, err := http.Get(m.URL)
	duration := time.Since(startTime)

	var healthStatus models.Status
	if err != nil {
		log.Printf("[HEALTH] Monitor %s (ID: %d) check failed: %v", m.Name, m.ID, err)
		healthStatus = models.MajorOutage
	} else {
		defer resp.Body.Close()
		if m.ExpectedStatus != nil && resp.StatusCode == *m.ExpectedStatus {
			healthStatus = models.Operational
		} else {
			healthStatus = models.MajorOutage
		}
	}

	// Registra o estado e atualiza métricas no Redis.
	registerStateHistoryAndMetrics(m, healthStatus, duration, rdb)

	// Verifica se a classificação detalhada está habilitada e a automação de incidentes também.
	if m.ThresholdClassification != nil && *m.ThresholdClassification &&
		m.AutoIncident != nil && *m.AutoIncident {
		healthStatus = evaluateDetailedStatus(m, resp, duration)
	}

	// Processa a criação/atualização do incidente com base no status final avaliado.
	ProcessIncidentCreation(m, healthStatus, db, rdb)

	log.Printf("[HEALTH] Monitor %s (ID: %d) check completed with status %s", m.Name, m.ID, healthStatus)
}

// evaluateDetailedStatus usa thresholds para classificar o monitor como
// operational, service_degraded, partial_outage ou major_outage.
func evaluateDetailedStatus(m models.Monitor, resp *http.Response, duration time.Duration) models.Status {
	if resp == nil {
		return models.MajorOutage
	}

	timeout := 5 * time.Second
	if m.Timeout != nil {
		timeout = time.Duration(*m.Timeout) * time.Millisecond
	}

	failureRate := int((duration.Seconds() / timeout.Seconds()) * 100)

	if m.ServiceDegradedThreshold != nil && failureRate < *m.ServiceDegradedThreshold {
		return models.Operational
	}
	if m.ServiceDegradedThreshold != nil && m.PartialOutageThreshold != nil &&
		failureRate >= *m.ServiceDegradedThreshold && failureRate < *m.PartialOutageThreshold {
		return models.ServiceDegraded
	}
	if m.PartialOutageThreshold != nil && m.MajorOutageThreshold != nil &&
		failureRate >= *m.PartialOutageThreshold && failureRate < *m.MajorOutageThreshold {
		return models.PartialOutage
	}
	if m.MajorOutageThreshold != nil && failureRate >= *m.MajorOutageThreshold {
		return models.MajorOutage
	}

	return models.MajorOutage
}

// registerStateHistoryAndMetrics registra o histórico e incrementa contadores de status.
func registerStateHistoryAndMetrics(m models.Monitor, healthStatus models.Status, duration time.Duration, rdb *redis.Client) {
	stateHistory := map[string]interface{}{
		"timestamp":    time.Now().Format(time.RFC3339),
		"status":       healthStatus,
		"responseTime": duration.Milliseconds(),
	}

	stateJSON, err := json.Marshal(stateHistory)
	if err != nil {
		log.Printf("[REDIS] Error serializing state history for monitor %s (ID: %d): %v", m.Name, m.ID, err)
		return
	}

	historyKey := fmt.Sprintf("monitor:%d:history", m.ID)
	// Armazena o registro no final da lista de histórico:
	err = rdb.RPush(client.Ctx, historyKey, stateJSON).Err()
	if err != nil {
		log.Printf("[REDIS] Error registering state history in Redis for monitor %s (ID: %d): %v", m.Name, m.ID, err)
	} else {
		log.Printf("[REDIS] Successfully registered state history for monitor %s (ID: %d)", m.Name, m.ID)
	}

	// Opcional: LTRIM para manter apenas os últimos N registros (ex.: 1000)
	if err := rdb.LTrim(client.Ctx, historyKey, -1000, -1).Err(); err != nil {
		log.Printf("[REDIS] Error trimming state history list for monitor %s (ID: %d): %v", m.Name, m.ID, err)
	}

	dateKey := time.Now().Format("2006-01-02")
	metricsKey := fmt.Sprintf("monitor:%d:metrics:%s", m.ID, dateKey)

	// Incrementa total de checks
	if err := rdb.HIncrBy(client.Ctx, metricsKey, "total_checks", 1).Err(); err != nil {
		log.Printf("[REDIS] Error incrementing total_checks for monitor %s (ID: %d): %v", m.Name, m.ID, err)
	}

	// Incrementa o status específico
	if err := rdb.HIncrBy(client.Ctx, metricsKey, string(healthStatus), 1).Err(); err != nil {
		log.Printf("[REDIS] Error incrementing counter for status %s for monitor %s (ID: %d): %v", healthStatus, m.Name, m.ID, err)
	}
}
