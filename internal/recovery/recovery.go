// Package recovery provides generic infrastructure recovery mechanisms for PromptPipe
// to handle application restarts gracefully. This package is application-agnostic and
// provides interfaces for flows to register their own recovery logic.
package recovery

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/store"
)

// Recoverable defines the interface for components that can recover their state
type Recoverable interface {
	// RecoverState is called during application startup to restore component state
	RecoverState(ctx context.Context, registry *RecoveryRegistry) error
}

// ParticipantRecoverable defines recovery for participant-based flows
type ParticipantRecoverable interface {
	Recoverable
	// RecoverParticipant is called for each active participant of this flow type
	RecoverParticipant(ctx context.Context, participantID string, participant interface{}, registry *RecoveryRegistry) error
	// GetFlowType returns the flow type this recoverable handles
	GetFlowType() models.FlowType
}

// TimerRecoveryInfo holds metadata about a timer that needs to be recovered
type TimerRecoveryInfo struct {
	ParticipantID string
	FlowType      models.FlowType
	StateType     models.StateType
	DataKey       models.DataKey
	OriginalTTL   time.Duration
	CreatedAt     time.Time
}

// ResponseHandlerRecoveryInfo holds metadata about response handlers
type ResponseHandlerRecoveryInfo struct {
	PhoneNumber   string
	ParticipantID string
	FlowType      models.FlowType
	HandlerType   string
	TTL           time.Duration
}

// RecoveryRegistry provides services that components can use during recovery
type RecoveryRegistry struct {
	store        store.Store
	timer        models.Timer
	timerInfos   []TimerRecoveryInfo
	handlerInfos []ResponseHandlerRecoveryInfo

	// Callbacks for infrastructure components to register
	timerRecoveryFunc   func(TimerRecoveryInfo) (string, error)
	handlerRecoveryFunc func(ResponseHandlerRecoveryInfo) error
}

// NewRecoveryRegistry creates a new recovery registry
func NewRecoveryRegistry(store store.Store, timer models.Timer) *RecoveryRegistry {
	return &RecoveryRegistry{
		store:        store,
		timer:        timer,
		timerInfos:   make([]TimerRecoveryInfo, 0),
		handlerInfos: make([]ResponseHandlerRecoveryInfo, 0),
	}
}

// RegisterTimerRecovery registers a callback for timer recovery
func (r *RecoveryRegistry) RegisterTimerRecovery(fn func(TimerRecoveryInfo) (string, error)) {
	r.timerRecoveryFunc = fn
}

// RegisterHandlerRecovery registers a callback for response handler recovery
func (r *RecoveryRegistry) RegisterHandlerRecovery(fn func(ResponseHandlerRecoveryInfo) error) {
	r.handlerRecoveryFunc = fn
}

// RecoverTimer requests recovery of a timer
func (r *RecoveryRegistry) RecoverTimer(info TimerRecoveryInfo) (string, error) {
	if r.timerRecoveryFunc == nil {
		return "", fmt.Errorf("no timer recovery handler registered")
	}
	return r.timerRecoveryFunc(info)
}

// RecoverResponseHandler requests recovery of a response handler
func (r *RecoveryRegistry) RecoverResponseHandler(info ResponseHandlerRecoveryInfo) error {
	if r.handlerRecoveryFunc == nil {
		return fmt.Errorf("no response handler recovery registered")
	}
	return r.handlerRecoveryFunc(info)
}

// GetStore provides access to the store for recovery operations
func (r *RecoveryRegistry) GetStore() store.Store {
	return r.store
}

// GetTimer provides access to the timer for recovery operations
func (r *RecoveryRegistry) GetTimer() models.Timer {
	return r.timer
}

// RecoveryManager orchestrates recovery of all registered components
type RecoveryManager struct {
	registry     *RecoveryRegistry
	recoverables []Recoverable
}

// NewRecoveryManager creates a new recovery manager
func NewRecoveryManager(store store.Store, timer models.Timer) *RecoveryManager {
	return &RecoveryManager{
		registry:     NewRecoveryRegistry(store, timer),
		recoverables: make([]Recoverable, 0),
	}
}

// RegisterRecoverable adds a component that can be recovered
func (rm *RecoveryManager) RegisterRecoverable(r Recoverable) {
	rm.recoverables = append(rm.recoverables, r)
}

// RegisterTimerRecovery registers the timer recovery infrastructure
func (rm *RecoveryManager) RegisterTimerRecovery(fn func(TimerRecoveryInfo) (string, error)) {
	rm.registry.RegisterTimerRecovery(fn)
}

// RegisterHandlerRecovery registers the response handler recovery infrastructure
func (rm *RecoveryManager) RegisterHandlerRecovery(fn func(ResponseHandlerRecoveryInfo) error) {
	rm.registry.RegisterHandlerRecovery(fn)
}

// RecoverAll performs recovery of all registered components
func (rm *RecoveryManager) RecoverAll(ctx context.Context) error {
	slog.Info("Starting application recovery", "components", len(rm.recoverables))

	recoveredCount := 0
	errorCount := 0

	for _, recoverable := range rm.recoverables {
		if err := recoverable.RecoverState(ctx, rm.registry); err != nil {
			slog.Error("Component recovery failed", "error", err, "component", fmt.Sprintf("%T", recoverable))
			errorCount++
			continue
		}
		recoveredCount++
	}

	slog.Info("Application recovery completed", "recovered", recoveredCount, "errors", errorCount)

	if errorCount > 0 {
		return fmt.Errorf("recovery completed with %d errors out of %d components", errorCount, len(rm.recoverables))
	}

	return nil
}

// GetRegistry provides access to the recovery registry for infrastructure setup
func (rm *RecoveryManager) GetRegistry() *RecoveryRegistry {
	return rm.registry
}
