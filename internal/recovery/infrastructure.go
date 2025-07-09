// Package recovery provides infrastructure helpers for wiring up recovery in the main application
package recovery

import (
	"fmt"
	"log/slog"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

// TimerRecoveryHandler provides the callback function for timer recovery infrastructure
func TimerRecoveryHandler(timer models.Timer) func(TimerRecoveryInfo) (string, error) {
	return func(info TimerRecoveryInfo) (string, error) {
		slog.Info("Recovering timer",
			"participantID", info.ParticipantID,
			"flowType", info.FlowType,
			"state", info.StateType,
			"ttl", info.OriginalTTL)

		// Create a simple timeout callback that logs the timeout event
		timeoutCallback := func() {
			slog.Warn("Timer timeout triggered during recovery",
				"participantID", info.ParticipantID,
				"flowType", info.FlowType,
				"state", info.StateType)

			// The actual timeout handling should be implemented by the flow logic
			// This is just infrastructure recovery - business logic is handled elsewhere
		}

		// Schedule the timer with the callback
		timerID, err := timer.ScheduleAfter(info.OriginalTTL, timeoutCallback)
		if err != nil {
			return "", fmt.Errorf("failed to schedule recovery timer: %w", err)
		}

		return timerID, nil
	}
}

// ResponseHandlerRecoveryCallback defines the callback signature for response handler recovery
// This allows the main application to provide the actual implementation without creating import cycles
type ResponseHandlerRecoveryCallback func(ResponseHandlerRecoveryInfo) error

// CreateResponseHandlerRecoveryHandler returns a simple handler that delegates to the provided callback
func CreateResponseHandlerRecoveryHandler(callback ResponseHandlerRecoveryCallback) func(ResponseHandlerRecoveryInfo) error {
	return func(info ResponseHandlerRecoveryInfo) error {
		slog.Info("Recovering response handler",
			"phone", info.PhoneNumber,
			"participantID", info.ParticipantID,
			"flowType", info.FlowType,
			"handlerType", info.HandlerType)

		if callback == nil {
			return fmt.Errorf("no response handler recovery callback provided")
		}

		return callback(info)
	}
}
