package models

import "time"

type MonitorGroup struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Visibility  *bool     `json:"visibility,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}
