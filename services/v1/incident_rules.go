package v1

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"reacher-cron/client"

	"github.com/go-redis/redis/v8"
)

// NormalizeStatus já existente...
func NormalizeStatus(healthCheckStatus string, incidentStatus string, ruleExists bool) string {
	if healthCheckStatus == "up" {
		return "operational"
	}
	if healthCheckStatus == "down" {
		if ruleExists || incidentStatus == "major_outage" {
			return "major_outage"
		}
		return "service_degraded"
	}
	if healthCheckStatus == "degraded" {
		return "service_degraded"
	}
	return "operational"
}

// ApplyIncidentRules aplica as regras de incidente, atualiza o Redis e, se comprovado,
// insere o incidente no PostgreSQL (e sincroniza para o Redis)
func ApplyIncidentRules(monitorID int, autoIncident bool, newStatus string, deg, part, maj interface{}, rdb *redis.Client, db *sql.DB) {
	key := fmt.Sprintf("monitor:%d", monitorID)
	// Se o health check retornar "up", garante o status operacional
	if newStatus == "up" {
		updateIncidentInRedis(monitorID, "operational", rdb)
		return
	}
	// Se o monitor não está configurado para autoIncident, atualiza com o novo status do health check
	if !autoIncident {
		updateIncidentInRedis(monitorID, newStatus, rdb)
		return
	}

	// Recuperar thresholds e a janela de escalonamento do Redis
	degStr, err := rdb.HGet(client.Ctx, key, "service_degraded_threshold").Result()
	if err != nil {
		degStr = "0"
	}
	partStr, err := rdb.HGet(client.Ctx, key, "partial_outage_threshold").Result()
	if err != nil {
		partStr = "0"
	}
	majStr, err := rdb.HGet(client.Ctx, key, "major_outage_threshold").Result()
	if err != nil {
		majStr = "0"
	}
	windowStr, err := rdb.HGet(client.Ctx, key, "escalation_window").Result()
	var escalationWindow int
	if err != nil || strings.TrimSpace(windowStr) == "" {
		escalationWindow = 5
	} else {
		fmt.Sscanf(windowStr, "%d", &escalationWindow)
	}

	var degThreshold, partThreshold, majThreshold int
	fmt.Sscanf(degStr, "%d", &degThreshold)
	fmt.Sscanf(partStr, "%d", &partThreshold)
	fmt.Sscanf(majStr, "%d", &majThreshold)

	// Recupera o histórico do monitor e filtra os eventos dentro da janela de escalonamento
	historyKey := fmt.Sprintf("%s:history", key)
	history, err := rdb.LRange(client.Ctx, historyKey, 0, -1).Result()
	if err != nil || len(history) == 0 {
		updateIncidentInRedis(monitorID, newStatus, rdb)
		return
	}
	now := time.Now()
	var total, downCount int
	for _, entry := range history {
		parts := strings.Split(entry, "|")
		if len(parts) != 2 {
			continue
		}
		t, err := time.Parse(time.RFC3339, parts[0])
		if err != nil {
			continue
		}
		if now.Sub(t) <= time.Duration(escalationWindow)*time.Minute {
			total++
			// Considere "down" ou "degraded" como falha
			if parts[1] == "down" || parts[1] == "degraded" {
				downCount++
			}
		}
	}
	var failureRate int
	if total > 0 {
		failureRate = (downCount * 100) / total
	}
	// Definir o status final baseado na taxa de falha e thresholds
	var finalStatus string
	switch {
	case failureRate >= majThreshold:
		finalStatus = "major_outage"
	case failureRate >= partThreshold:
		finalStatus = "partial_outage"
	case failureRate >= degThreshold:
		finalStatus = "service_degraded"
	default:
		finalStatus = "operational"
	}
	updateIncidentInRedis(monitorID, finalStatus, rdb)

	// Se o status final indicar um incidente, insere no PostgreSQL e sincroniza no Redis.
	if finalStatus != "operational" {
		err := createAndSyncIncident(monitorID, finalStatus, db, rdb)
		if err != nil {
			log.Printf("Error inserting incident in Postgres: %v", err)
		} else {
			log.Printf("Incident inserted and synced for monitor %d", monitorID)
		}
	}
}

// createAndSyncIncident insere um incidente no PostgreSQL e sincroniza os dados para o Redis.
func createAndSyncIncident(monitorID int, status string, db *sql.DB, rdb *redis.Client) error {
	title := fmt.Sprintf("Incident detected for monitor %d", monitorID)
	description := "Incident automatically detected based on failure rate thresholds."
	query := `
		INSERT INTO incidents (title, description, monitor_id, status, notify_subscribers)
		VALUES ($1, $2, $3, $4, true) RETURNING id`
	var incidentID int
	err := db.QueryRow(query, title, description, monitorID, status).Scan(&incidentID)
	if err != nil {
		return err
	}

	// Agora sincroniza os dados do incidente para o Redis.
	redisKey := fmt.Sprintf("incident:%d", incidentID)
	fields := map[string]interface{}{
		"id":          incidentID,
		"monitor_id":  monitorID,
		"status":      status,
		"title":       title,
		"description": description,
		"created_at":  time.Now().Format(time.RFC3339),
	}
	if err := rdb.HSet(client.Ctx, redisKey, fields).Err(); err != nil {
		return err
	}

	return nil
}

// updateIncidentInRedis atualiza o campo "overall_status" e registra a hora da última atualização no Redis.
func updateIncidentInRedis(monitorID int, status string, rdb *redis.Client) {
	key := fmt.Sprintf("monitor:%d", monitorID)
	rdb.HSet(client.Ctx, key, map[string]interface{}{
		"overall_status": status,
		"last_updated":   time.Now().Format(time.RFC3339),
	})
	log.Printf("[INCIDENT] Updated incident info for monitor %d: status=%s", monitorID, status)
}
