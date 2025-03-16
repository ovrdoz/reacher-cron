package v1

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"reacher-cron/client"
	"reacher-cron/models"

	"github.com/go-redis/redis/v8"
)

// FetchAllMonitors returns monitors with status "Active" or "Inactive" from Redis,
// agora usando pipeline para reduzir round-trips.
func FetchAllMonitors() ([]models.Monitor, error) {
	ctx := client.Ctx
	rdb := client.ConnectRedis()

	// Recupera todos os IDs de monitores do conjunto "monitors:ids".
	ids, err := rdb.SMembers(ctx, "monitors:ids").Result()
	if err != nil {
		return nil, err
	}

	// Prepara um pipeline para buscar o hash de cada monitor em lote:
	pipe := rdb.Pipeline()
	cmds := make([]*redis.StringStringMapCmd, 0, len(ids))

	for _, idStr := range ids {
		monitorKey := fmt.Sprintf("monitor:%s", idStr)
		cmds = append(cmds, pipe.HGetAll(ctx, monitorKey))
	}

	// Executa o pipeline de uma só vez:
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, err
	}

	var monitors []models.Monitor
	for _, cmd := range cmds {
		data, err := cmd.Result()
		if err != nil {
			// Se houve erro neste HGetAll específico, apenas ignoramos este monitor.
			continue
		}
		if len(data) == 0 {
			// Se a hash estiver vazia, ignoramos.
			continue
		}

		// Converte o hash para o struct Monitor.
		m, err := mapToMonitor(data, ctx, rdb)
		if err != nil {
			continue
		}

		// Filtra pelos status "Active" ou "Inactive".
		if m.Status == "Active" || m.Status == "Inactive" {
			monitors = append(monitors, m)
		}
	}

	return monitors, nil
}

// mapToMonitor converte o hash Redis em models.Monitor.
// Mantém praticamente o mesmo código de antes.
func mapToMonitor(data map[string]string, ctx context.Context, rdb *redis.Client) (models.Monitor, error) {
	var m models.Monitor

	id, err := strconv.Atoi(data["id"])
	if err != nil {
		return m, err
	}
	m.ID = id
	m.Name = data["name"]
	m.URL = data["url"]
	m.Status = data["status"]
	m.Interval = data["interval"]

	// Converte LastChecked, se disponível.
	if v, ok := data["last_checked"]; ok && v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			m.LastChecked = &t
		}
	}

	// ResponseTime, se disponível.
	if v, ok := data["response_time"]; ok && v != "" {
		m.ResponseTime = &v
	}

	convertInt := func(key string) *int {
		if v, ok := data[key]; ok && v != "" {
			if num, err := strconv.Atoi(v); err == nil {
				return &num
			}
		}
		return nil
	}

	m.ExpectedStatus = convertInt("expected_status")
	m.Timeout = convertInt("timeout")
	m.ServiceDegradedThreshold = convertInt("service_degraded_threshold")
	m.PartialOutageThreshold = convertInt("partial_outage_threshold")
	m.MajorOutageThreshold = convertInt("major_outage_threshold")
	m.EscalationWindow = convertInt("escalation_window")

	// Converte auto_incident para booleano.
	if v, ok := data["auto_incident"]; ok && v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			m.AutoIncident = &b
		}
	}

	// Novo campo: threshold_classification (booleano).
	if v, ok := data["threshold_classification"]; ok && v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			m.ThresholdClassification = &b
		}
	}

	// Novo campo: incident_creation_criteria (string).
	m.IncidentCreationCriteria = data["incident_creation_criteria"]

	// GroupID
	if v, ok := data["group_id"]; ok && v != "" {
		if num, err := strconv.Atoi(v); err == nil {
			m.GroupID = &num
		}
	}

	// CreatedAt: converte usando RFC3339.
	if v, ok := data["created_at"]; ok && v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			m.CreatedAt = t
		}
	}

	// Tags: armazenadas como string separada por vírgulas.
	if v, ok := data["tags"]; ok && v != "" {
		m.Tags = strings.Split(v, ",")
	} else {
		m.Tags = []string{}
	}

	// Busca o nome do grupo, se group_id existir.
	if m.GroupID != nil && *m.GroupID > 0 {
		groupKey := fmt.Sprintf("monitor_group:%d", *m.GroupID)
		groupData, err := rdb.HGetAll(ctx, groupKey).Result()
		if err == nil && len(groupData) > 0 {
			name := groupData["name"]
			m.Group = &name
		}
	}

	return m, nil
}
