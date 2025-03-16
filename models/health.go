package models

// Status representa o status de um health check.
type Status string

/*
Basic Incident Creation Rules:

1. When Incident Automation is enabled:
   - Based on Detailed Classification (Threshold):
     The system evaluates health check results using defined thresholds:
       • Service Degraded: if the failure rate exceeds this threshold, the monitor is classified as "Service Degraded".
       • Partial Outage: if the failure rate is between the Service Degraded and Partial Outage thresholds, the monitor is considered in "Partial Outage".
       • Major Outage: if the failure rate meets or exceeds the Major Outage threshold, the monitor is classified as "Major Outage".
     An incident is automatically created and classified according to these thresholds.

   - Open Immediately on Failure:
     An incident is created immediately when a failure is detected, without using any thresholds.

2. If Detailed Classification is disabled:
   Health checks are evaluated simply as "Operational" (online) if passing or "major_outage" if failing.
   Attempting to use the threshold-based classification without enabling Detailed Classification is invalid.
*/

const (
	Operational     Status = "operational"      // Service is working normally (online).
	ServiceDegraded Status = "service_degraded" // Service is degraded; failure rate exceeds the minimal threshold.
	PartialOutage   Status = "partial_outage"   // Partial outage; failure rate is between the degraded and critical thresholds.
	MajorOutage     Status = "major_outage"     // Major outage; failure rate meets or exceeds the critical threshold.
)
