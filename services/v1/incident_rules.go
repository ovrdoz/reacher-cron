package v1

import (
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"time"

	"reacher-cron/client"
	"reacher-cron/models"

	"github.com/go-redis/redis/v8"
)

func ProcessIncidentCreation(m models.Monitor, healthStatus models.Status, db *sql.DB, rdb *redis.Client) {
	if m.AutoIncident == nil || !*m.AutoIncident {
		return
	}

	switch m.IncidentCreationCriteria {
	case "threshold":
		// Só cria incidente se o status for service_degraded, partial_outage ou major_outage.
		if healthStatus == models.Operational || healthStatus == models.MajorOutage {
			return
		}
	case "immediate":
		// "immediate": qualquer falha gera incidente.
		if healthStatus == models.Operational {
			return
		}
	}

	// 1) Verifica se já existe um incidente aberto para este monitor
	var existingIncidentID int
	err := db.QueryRow(`
        SELECT id
        FROM incident
        WHERE monitor_id = $1
          AND incident_status = 'open'
        LIMIT 1
    `, m.ID).Scan(&existingIncidentID)

	if err == nil {
		// Achamos um incidente com status open => não cria outro
		log.Printf("[INCIDENT] Not creating new incident for monitor %d because an open incident already exists (ID: %d).", m.ID, existingIncidentID)
		return
	} else if err != sql.ErrNoRows {
		// Se for outro erro, loga e sai
		log.Printf("[INCIDENT] Error checking open incident for monitor %d: %v", m.ID, err)
		return
	}
	// Se chegou aqui, err == sql.ErrNoRows => não existe incidente aberto, podemos criar.

	// 2) Insere um novo incidente no Postgres
	query := `
		INSERT INTO incident (title, description, monitor_id, incident_type, incident_status, notify_subscribers)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at
	`
	title := "Incident for monitor: " + m.Name
	description := "Automatic incident creation triggered by health check at " + time.Now().Format(time.RFC3339)
	notifySubscribers := false

	var incidentID int
	var createdAt, updatedAt time.Time

	err = db.QueryRow(query,
		title,
		description,
		m.ID,
		string(healthStatus), // incident_type
		"open",               // incident_status
		notifySubscribers,
	).Scan(&incidentID, &createdAt, &updatedAt)
	if err != nil {
		log.Printf("[INCIDENT] Error creating incident for monitor %s (ID: %d): %v", m.Name, m.ID, err)
		return
	}

	log.Printf("[INCIDENT] Incident created for monitor %s (ID: %d). IncidentID: %d, Type: %s, Status: open",
		m.Name, m.ID, incidentID, healthStatus)

	// 3) Incrementa contadores diários de incidentes, se quiser
	dateKey := time.Now().Format("2006-01-02")
	metricsKey := fmt.Sprintf("monitor:%d:metrics:%s", m.ID, dateKey)
	switch healthStatus {
	case models.MajorOutage:
		if err := rdb.HIncrBy(client.Ctx, metricsKey, "major_outage", 1).Err(); err != nil {
			log.Printf("[REDIS] Error incrementing major_outage for monitor %s (ID: %d): %v", m.Name, m.ID, err)
		}
	case models.PartialOutage:
		if err := rdb.HIncrBy(client.Ctx, metricsKey, "partial_outage", 1).Err(); err != nil {
			log.Printf("[REDIS] Error incrementing partial_outage for monitor %s (ID: %d): %v", m.Name, m.ID, err)
		}
	case models.ServiceDegraded:
		if err := rdb.HIncrBy(client.Ctx, metricsKey, "service_degraded", 1).Err(); err != nil {
			log.Printf("[REDIS] Error incrementing service_degraded for monitor %s (ID: %d): %v", m.Name, m.ID, err)
		}
	}

	// 4) Sincroniza o incidente no Redis
	if err := syncIncidentToRedis(incidentID, m, healthStatus, createdAt, updatedAt, rdb); err != nil {
		log.Printf("[INCIDENT] Failed to sync incident (ID: %d) to Redis: %v", incidentID, err)
	}
}

func syncIncidentToRedis(incidentID int, m models.Monitor, status models.Status,
	createdAt, updatedAt time.Time, rdb *redis.Client) error {

	key := "incident:" + strconv.Itoa(incidentID)
	incidentData := map[string]interface{}{
		"id":                 incidentID,
		"monitor_id":         m.ID,
		"monitor_name":       m.Name,
		"incident_type":      string(status),
		"incident_status":    "open",
		"created_at":         createdAt.Format(time.RFC3339),
		"updated_at":         updatedAt.Format(time.RFC3339),
		"title":              "Incident for monitor: " + m.Name,
		"description":        "Automatic incident creation triggered by health check",
		"notify_subscribers": false,
	}

	if err := rdb.HSet(client.Ctx, key, incidentData).Err(); err != nil {
		return err
	}

	if err := rdb.SAdd(client.Ctx, "incidents:ids", incidentID).Err(); err != nil {
		return err
	}

	log.Printf("[INCIDENT] Incident data synchronized to Redis for incident ID %d", incidentID)
	return nil
}
