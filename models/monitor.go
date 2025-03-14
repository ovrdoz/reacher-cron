package models

import "time"

// Monitor representa um registro no banco, onde:
// - Status = "Active" ou "Inactive" (se deve ter job ou não)
// - Os detalhes de health check ficam no Redis, não aqui.
type Monitor struct {
	ID                       int        `json:"id"`
	Name                     string     `json:"name"`
	URL                      string     `json:"url"`
	Status                   string     `json:"status"`
	LastChecked              *time.Time `json:"lastChecked,omitempty"`
	ResponseTime             *string    `json:"responseTime,omitempty"`
	Interval                 string     `json:"interval"`
	ExpectedStatus           *int       `json:"expectedStatus,omitempty"`
	Timeout                  *int       `json:"timeout,omitempty"`
	AutoIncident             *bool      `json:"autoIncident,omitempty"`
	ServiceDegradedThreshold *int       `json:"serviceDegradedThreshold,omitempty"`
	PartialOutageThreshold   *int       `json:"partialOutageThreshold,omitempty"`
	MajorOutageThreshold     *int       `json:"majorOutageThreshold,omitempty"`
	EscalationWindow         *int       `json:"escalationWindow,omitempty"`
	Group                    *string    `json:"group,omitempty"`
	GroupID                  *int       `json:"groupId,omitempty"`
	GroupName                string     `json:"groupName,omitempty"`
	GroupVisibility          bool       `json:"groupVisibility,omitempty"`
	CreatedAt                time.Time  `json:"createdAt"`
	Tags                     []string   `json:"tags"`
}
