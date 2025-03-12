package v1

import (
	"database/sql"
	"reacher-cron/models"
)

// FetchAllMonitors retorna monitores que podem estar "Active" ou "Inactive".
// (A query pode ser adaptada se quiser lidar com monitores "deletados", etc.)
func FetchAllMonitors(db *sql.DB) ([]models.Monitor, error) {
	rows, err := db.Query(`
        SELECT
			m.id, m.name, m.url, m.status, m.interval,
			m.expected_status, m.timeout, m.auto_incident,
			m.service_degraded_threshold, m.partial_outage_threshold,
			m.major_outage_threshold, m.escalation_window, m.group_id,
			COALESCE(g.name, '') AS group_name,
			COALESCE(g.visibility, false) AS group_visibility
		FROM monitors m
		LEFT JOIN monitor_groups g ON m.group_id = g.id
		WHERE m.status IN ('Active', 'Inactive')
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var monitors []models.Monitor
	for rows.Next() {
		var m models.Monitor
		scanErr := rows.Scan(
			&m.ID,
			&m.Name,
			&m.URL,
			&m.Status,
			&m.Interval,
			&m.ExpectedStatus,
			&m.Timeout,
			&m.AutoIncident,
			&m.ServiceDegradedThreshold,
			&m.PartialOutageThreshold,
			&m.MajorOutageThreshold,
			&m.EscalationWindow,
			&m.GroupID,
			&m.GroupName,
			&m.GroupVisibility,
		)
		if scanErr == nil {
			monitors = append(monitors, m)
		}
	}
	return monitors, nil
}
