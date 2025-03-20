package models

import (
	"time"
)

type Monitor struct {
	ID                       int        `json:"id"`
	Name                     string     `json:"name"`
	URL                      string     `json:"url"`
	Status                   string     `json:"status"`
	LastChecked              *time.Time `json:"lastChecked,omitempty"`  // Pode ser NULL
	ResponseTime             *string    `json:"responseTime,omitempty"` // Pode ser NULL
	Interval                 string     `json:"interval"`
	ExpectedStatus           *int       `json:"expectedStatus,omitempty"` // Código HTTP esperado
	Timeout                  *int       `json:"timeout,omitempty"`        // Timeout em ms
	ThresholdClassification  *bool      `json:"thresholdClassification,omitempty"`
	ServiceDegradedThreshold *int       `json:"serviceDegradedThreshold,omitempty"`
	PartialOutageThreshold   *int       `json:"partialOutageThreshold,omitempty"`
	MajorOutageThreshold     *int       `json:"majorOutageThreshold,omitempty"`
	EscalationWindow         *int       `json:"escalationWindow,omitempty"`
	AutoIncident             *bool      `json:"autoIncident,omitempty"`
	AutoResolveIncident      *bool      `json:"autoResolveIncident,omitempty"`
	IncidentCreationCriteria string     `json:"incidentCreationCriteria"`
	Group                    *string    `json:"group,omitempty"`
	GroupID                  *int       `json:"groupId,omitempty"`
	CreatedAt                time.Time  `json:"createdAt"`
	Tags                     []string   `json:"tags"`
}
