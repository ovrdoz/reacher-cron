package v1

import (
	"log"
	"sync"

	"reacher-cron/client"

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

	monitors, err := FetchAllMonitors()
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

}

// Função para converter cron expression para 6 campos se necessário
func getCronExpression(interval string) string {
	return interval
}
