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
		if healthStatus == models.Operational {
			return
		}
	case "immediate":
		// "immediate": qualquer falha gera incidente.
		if healthStatus == models.Operational {
			return
		}
	}

	// 1) Verifica se já existe um incidente para este monitor com status 'open' OU 'monitoring'
	var existingIncidentID int
	var existingIncidentStatus string

	err := db.QueryRow(`
    SELECT id, incidentStatus
    FROM incident
    WHERE monitorId = $1
      AND (incidentStatus = 'open' OR incidentStatus = 'monitoring')
    LIMIT 1`, m.ID).Scan(&existingIncidentID, &existingIncidentStatus)

	if err == nil {
		// Achamos um incidente cujo status não é resolvido => não cria outro
		log.Printf("[INCIDENT] Not creating new incident for monitor %d because an incident with status '%s' already exists (ID: %d).", m.ID, existingIncidentStatus, existingIncidentID)
		return
	} else if err != sql.ErrNoRows {
		// Se for outro erro de banco, loga e sai
		log.Printf("[INCIDENT] Error checking existing incident for monitor %d: %v", m.ID, err)
		return
	}
	// Se chegou aqui, err == sql.ErrNoRows => não existe incidente com status 'open' ou 'monitoring'. Podemos criar um novo.

	// 2) Insere um novo incidente no Postgres
	query := `
		INSERT INTO incident (title, description, monitorId, incidentType, incidentStatus, notifySubscribers)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, createdAt, updatedAt
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
		string(healthStatus), // incidentType
		"open",               // incidentStatus
		notifySubscribers,
	).Scan(&incidentID, &createdAt, &updatedAt)
	if err != nil {
		log.Printf("[INCIDENT] Error creating incident for monitor %s (ID: %d): %v", m.Name, m.ID, err)
		return
	}

	log.Printf("[INCIDENT] Incident created for monitor %s (ID: %d). IncidentID: %d, Type: %s, Status: open",
		m.Name, m.ID, incidentID, healthStatus)

	// 3) Incrementa contadores diários de incidentes, se quiser
	dateKey := time.Now().UTC().Format("2006-01-02")
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

func ProcessIncidentAutoResolve(m models.Monitor, db *sql.DB, rdb *redis.Client) {
	log.Printf("[INCIDENT] Check incident has opened %s (ID: %d)", m.Name, m.ID)
	// Se o auto resolve não estiver habilitado, não faz nada.
	if m.AutoResolveIncident == nil || !*m.AutoResolveIncident {
		return
	}

	// Verifica se existe um incidente com status "open" ou "monitoring".
	var incidentID int
	var incidentStatus string
	err := db.QueryRow(`
        SELECT id, incidentStatus
        FROM incident
        WHERE monitorId = $1
          AND (incidentStatus = 'open' OR incidentStatus = 'monitoring')
        LIMIT 1`, m.ID).Scan(&incidentID, &incidentStatus)
	if err == sql.ErrNoRows {
		// Nenhum incidente aberto encontrado.
		log.Printf("[INCIDENT] Nothing to do, no incident opened %s (ID: %d)", m.Name, m.ID)
		return
	} else if err != nil {
		log.Printf("[INCIDENT] Error checking open incident for monitor %s (ID: %d): %v", m.Name, m.ID, err)
		return
	}

	// Atualiza o incidente para o status "closed".
	_, err = db.Exec(`
        UPDATE incident 
        SET incidentStatus = 'resolved', updatedAt = $2 
        WHERE id = $1`, incidentID, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		log.Printf("[INCIDENT] Error auto-closing incident %d for monitor %s (ID: %d): %v", incidentID, m.Name, m.ID, err)
		return
	}

	log.Printf("[INCIDENT] Auto-resolve incident %d for monitor %s (ID: %d)", incidentID, m.Name, m.ID)

	// Sincroniza a atualização no Redis.
	key := fmt.Sprintf("incident:%d", incidentID)
	err = rdb.HSet(client.Ctx, key,
		"incidentStatus", "resolved",
		"updatedAt", time.Now().UTC().Format(time.RFC3339),
	).Err()
	if err != nil {
		log.Printf("[REDIS] Error syncing auto-resolved incident %d for monitor %s (ID: %d): %v", incidentID, m.Name, m.ID, err)
	}
}

func syncIncidentToRedis(incidentID int, m models.Monitor, status models.Status,
	createdAt, updatedAt time.Time, rdb *redis.Client) error {

	key := "incident:" + strconv.Itoa(incidentID)
	incidentData := map[string]interface{}{
		"id":                incidentID,
		"monitorId":         m.ID,
		"monitorName":       m.Name,
		"incidentType":      string(status),
		"incidentStatus":    "open",
		"createdAt":         createdAt.Format(time.RFC3339),
		"updatedAt":         updatedAt.Format(time.RFC3339),
		"title":             "Incident for monitor: " + m.Name,
		"description":       "Automatic incident creation triggered by health check",
		"notifySubscribers": false,
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
