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

// FetchAllMonitors returns monitors with status "Active" or "Inactive" from Redis.
func FetchAllMonitors() ([]models.Monitor, error) {
	ctx := client.Ctx
	rdb := client.ConnectRedis()

	// Retrieve all monitor IDs from a set "monitors:ids"
	ids, err := rdb.SMembers(ctx, "monitors:ids").Result()
	if err != nil {
		return nil, err
	}

	var monitors []models.Monitor
	for _, idStr := range ids {
		monitorKey := fmt.Sprintf("monitor:%s", idStr)
		data, err := rdb.HGetAll(ctx, monitorKey).Result()
		if err != nil {
			// Log and skip this monitor if there is an error.
			continue
		}
		// Skip if key is empty
		if len(data) == 0 {
			continue
		}
		m, err := mapToMonitor(data, ctx, rdb)
		if err != nil {
			// Log and skip this monitor.
			continue
		}
		// Filter by status if necessary.
		if m.Status == "Active" || m.Status == "Inactive" {
			monitors = append(monitors, m)
		}
	}

	return monitors, nil
}

// mapToMonitor converts a Redis hash (map[string]string) into a models.Monitor.
// It also fetches group information (name and visibility) from Redis if a group_id is set.
func mapToMonitor(data map[string]string, ctx context.Context, rdb *redis.Client) (models.Monitor, error) {
	var m models.Monitor

	// Convert basic fields.
	id, err := strconv.Atoi(data["id"])
	if err != nil {
		return m, err
	}
	m.ID = id
	m.Name = data["name"]
	m.URL = data["url"]
	m.Status = data["status"]
	m.Interval = data["interval"]

	// Numeric fields conversion helper.
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

	// AutoIncident: parse as boolean.
	if v, ok := data["auto_incident"]; ok && v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			m.AutoIncident = &b
		}
	}

	// GroupID
	if v, ok := data["group_id"]; ok && v != "" {
		if num, err := strconv.Atoi(v); err == nil {
			m.GroupID = &num
		}
	}

	// CreatedAt: parse using RFC3339
	if v, ok := data["created_at"]; ok && v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			m.CreatedAt = t
		}
	}

	// Tags: stored as comma-separated string
	if v, ok := data["tags"]; ok && v != "" {
		m.Tags = strings.Split(v, ",")
	} else {
		m.Tags = []string{}
	}

	// Fetch group name and visibility if group_id exists.
	if m.GroupID != nil && *m.GroupID > 0 {
		groupKey := fmt.Sprintf("monitor_group:%d", *m.GroupID)
		groupData, err := rdb.HGetAll(ctx, groupKey).Result()
		if err == nil && len(groupData) > 0 {
			m.GroupName = groupData["name"]
			// Convert visibility string to bool.
			if vis, ok := groupData["visibility"]; ok && vis != "" {
				if b, err := strconv.ParseBool(vis); err == nil {
					m.GroupVisibility = b
				}
			}
		}
	}

	return m, nil
}
