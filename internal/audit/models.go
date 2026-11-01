package audit

import "time"

// EventType enumerates the audit events required by the buildplan.
type EventType string

const (
	EventRunStarted                   EventType = "RUN_STARTED"
	EventValidationCompleted          EventType = "VALIDATION_COMPLETED"
	EventStateReconciliationCompleted EventType = "STATE_RECONCILIATION_COMPLETED"
	EventDriftDetected                EventType = "DRIFT_DETECTED"
	EventReadinessAnalysisCompleted   EventType = "READINESS_ANALYSIS_COMPLETED"
	EventBlockingFindingFound         EventType = "BLOCKING_FINDING_FOUND"
	EventWarningAcknowledged          EventType = "WARNING_ACKNOWLEDGED"
	EventApprovalRequested            EventType = "APPROVAL_REQUESTED"
	EventApprovalGranted              EventType = "APPROVAL_GRANTED"
	EventEksctlCommandStarted         EventType = "EKSCTL_COMMAND_STARTED"
	EventEksctlCommandCompleted       EventType = "EKSCTL_COMMAND_COMPLETED"
	EventPhaseCompleted               EventType = "PHASE_COMPLETED"
	EventHealthGatePassed             EventType = "HEALTH_GATE_PASSED"
	EventHealthGateFailed             EventType = "HEALTH_GATE_FAILED"
	EventCanaryPassed                 EventType = "CANARY_PASSED"
	EventCanaryFailed                 EventType = "CANARY_FAILED"
	EventPaused                       EventType = "PAUSED"
	EventResumed                      EventType = "RESUMED"
	EventAborted                      EventType = "ABORTED"
	EventRunCompleted                 EventType = "RUN_COMPLETED"
)

// Event is a single immutable, append-only audit record.
type Event struct {
	Time    time.Time              `json:"time"`
	RunID   string                 `json:"runID"`
	Cluster string                 `json:"cluster"`
	Region  string                 `json:"region"`
	Type    EventType              `json:"type"`
	Phase   string                 `json:"phase,omitempty"`
	Message string                 `json:"message"`
	Data    map[string]interface{} `json:"data,omitempty"`
}
