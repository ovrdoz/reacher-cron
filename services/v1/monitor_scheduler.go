package v1

import (
	"fmt"
	"log"
	"sync"

	"reacher-cron/client"
	"reacher-cron/models"

	"github.com/go-redis/redis/v8"
	"github.com/robfig/cron/v3"
)

var (
	monitorCron *cron.Cron
	jobMap      = make(map[int]struct {
		EntryID  cron.EntryID
		CronExpr string
	})
	mu sync.Mutex
)

// StartGlobalMonitorScheduler inicializa a cron e agenda um “sync” periódico
func StartGlobalMonitorScheduler() {
	log.Println("[CRON] Starting global scheduler...")
	monitorCron = cron.New()
	_, err := monitorCron.AddFunc("@every 15s", syncMonitorJobs)
	if err != nil {
		log.Fatalf("[CRON] Failed to add sync job: %v", err)
	}

	monitorCron.Start()
	syncMonitorJobs() // Executa imediatamente na inicialização
}

// syncMonitorJobs busca monitores do banco e ajusta a cron dinamicamente
func syncMonitorJobs() {
	log.Println("[CRON] Syncing monitor jobs...")

	db := client.ConnectPostgres()
	rdb := client.ConnectRedis()

	monitors, err := FetchAllMonitors(db)
	if err != nil {
		log.Println("[CRON] Error fetching monitors:", err)
		return
	}

	mu.Lock()
	defer mu.Unlock()

	// Mapear IDs de monitores ativos
	activeMonitors := make(map[int]bool)

	// 1️ Adicionar ou atualizar jobs apenas para monitores "Active"
	for _, monitor := range monitors {
		activeMonitors[monitor.ID] = true

		updateMonitorInfoInRedis(monitor, rdb)

		if monitor.Status != "Active" {
			// Se for "Inactive", não ganha job, mas mantemos os dados no Redis
			continue
		}

		cronExpr := getCronExpression(monitor.Interval)

		// Se já havia um job, remove para recriar
		if job, exists := jobMap[monitor.ID]; exists {
			log.Printf("[CRON] Updating monitor job: %s (ID: %d) - %s\n", monitor.Name, monitor.ID, cronExpr)
			monitorCron.Remove(job.EntryID)
			delete(jobMap, monitor.ID)
		}

		log.Printf("[CRON] Adding monitor job: %s (ID: %d) - %s\n", monitor.Name, monitor.ID, cronExpr)
		monitorCopy := monitor
		entryID, err := monitorCron.AddFunc(cronExpr, func() {
			doHealthCheck(monitorCopy, rdb, db)
		})
		if err != nil {
			log.Printf("[CRON] Failed to schedule monitor: %s (ID: %d): %v\n", monitor.Name, monitor.ID, err)
			continue
		}

		jobMap[monitor.ID] = struct {
			EntryID  cron.EntryID
			CronExpr string
		}{
			EntryID:  entryID,
			CronExpr: cronExpr,
		}
	}

	// 2️ Remover jobs de monitores que não estão mais ativos
	for id, job := range jobMap {
		if !activeMonitors[id] {
			log.Printf("[CRON] Removing monitor job (ID: %d)\n", id)
			monitorCron.Remove(job.EntryID)
			delete(jobMap, id)
		}
	}

	// 3️ Remover do Redis **somente monitores que foram deletados do banco**
	removeDeletedMonitorsFromRedis(rdb, activeMonitors)
}

// Função para converter cron expression para 6 campos se necessário
func getCronExpression(interval string) string {
	return interval
}

func updateMonitorInfoInRedis(m models.Monitor, rdb *redis.Client) {
	key := fmt.Sprintf("monitor:%d", m.ID)

	// 🔹 Buscar os dados atuais do Redis para esse monitor
	existingData, err := rdb.HGetAll(client.Ctx, key).Result()
	if err != nil {
		log.Printf("[REDIS] Error fetching monitor %d: %v", m.ID, err)
		return
	}

	// 🔹 Preparar os dados esperados
	groupID := "0"
	if m.GroupID.Valid {
		groupID = fmt.Sprintf("%d", m.GroupID.Int64)
	}

	expectedData := map[string]string{
		"status":                     m.Status,
		"name":                       m.Name,
		"group_id":                   groupID,
		"group_name":                 m.GroupName,
		"group_visibility":           fmt.Sprintf("%t", m.GroupVisibility),
		"auto_incident":              fmt.Sprintf("%t", m.AutoIncident),
		"service_degraded_threshold": fmt.Sprintf("%d", m.ServiceDegradedThreshold.Int64),
		"partial_outage_threshold":   fmt.Sprintf("%d", m.PartialOutageThreshold.Int64),
		"major_outage_threshold":     fmt.Sprintf("%d", m.MajorOutageThreshold.Int64),
		"escalation_window":          fmt.Sprintf("%d", m.EscalationWindow.Int64),
	}

	// 🔹 Comparar valores e atualizar apenas se forem diferentes
	needsUpdate := false
	for key, expectedValue := range expectedData {
		if existingData[key] != expectedValue {
			needsUpdate = true
			break
		}
	}

	if needsUpdate {
		rdb.HSet(client.Ctx, key, expectedData)
		log.Printf("[REDIS] Updated monitor (ID: %d) information in Redis\n", m.ID)
	}
}

// Remove do Redis apenas monitores que **não** aparecem mais na base de dados
func removeDeletedMonitorsFromRedis(rdb *redis.Client, activeMonitors map[int]bool) {
	var cursor uint64
	for {
		keys, newCursor, err := rdb.Scan(client.Ctx, cursor, "monitor:*", 0).Result()
		if err != nil {
			log.Println("[REDIS] Error scanning:", err)
			break
		}
		cursor = newCursor

		for _, key := range keys {
			var monitorID int
			_, err := fmt.Sscanf(key, "monitor:%d", &monitorID)
			if err != nil {
				continue // Se não for um ID de monitor, pula
			}

			if !activeMonitors[monitorID] {
				historyKey := fmt.Sprintf("monitor:%d:history", monitorID)
				rdb.Del(client.Ctx, key)
				rdb.Del(client.Ctx, historyKey)
				log.Printf("[CRON] Removed Redis key: %s and its history\n", key)
			}
		}

		if cursor == 0 {
			break
		}
	}
}
