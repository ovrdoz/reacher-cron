package models

import (
	"database/sql"
)

// Monitor representa um registro no banco, onde:
// - Status = "Active" ou "Inactive" (se deve ter job ou não)
// - Os detalhes de health check ficam no Redis, não aqui.
type Monitor struct {
	ID                       int
	Name                     string
	URL                      string
	Status                   string // "Active" ou "Inactive"
	Interval                 string
	ExpectedStatus           sql.NullInt64
	Timeout                  sql.NullInt64
	AutoIncident             bool
	ServiceDegradedThreshold sql.NullInt64
	PartialOutageThreshold   sql.NullInt64
	MajorOutageThreshold     sql.NullInt64
	EscalationWindow         sql.NullInt64
	GroupID                  sql.NullInt64
	GroupName                string
	GroupVisibility          bool
}
