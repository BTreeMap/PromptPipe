// Package models defines flow type definitions to avoid circular imports.
package models

// FlowType represents a specific type of intervention flow
type FlowType string

// StateType represents a specific state within a flow
type StateType string

// DataKey represents a key for storing state-specific data
type DataKey string

// FlowAssignment represents the type of intervention assignment
type FlowAssignment string

// ResponseValue represents expected response values
type ResponseValue string

// Flow type constants.
const (
	FlowTypeConversation FlowType = "conversation"
)

// State constants for conversation flow.
const (
	StateConversationActive StateType = "CONVERSATION_ACTIVE"
	// Removed: StateCoordinator (deprecated in new design)
	StateIntake   StateType = "INTAKE"   // Intake bot conversations (default sub-state)
	StateFeedback StateType = "FEEDBACK" // Feedback tracker conversations
)

// Data key constants for conversation flow.
const (
	DataKeyConversationHistory     DataKey = "conversationHistory"
	DataKeySystemPrompt            DataKey = "systemPrompt"
	DataKeyParticipantBackground   DataKey = "participantBackground"
	DataKeyUserProfile             DataKey = "userProfile"             // For storing structured user profiles
	DataKeyLastHabitPrompt         DataKey = "lastHabitPrompt"         // For tracking the last habit prompt sent
	DataKeyFeedbackState           DataKey = "feedbackState"           // For tracking feedback collection state
	DataKeyFeedbackTimerID         DataKey = "feedbackTimerID"         // For tracking initial feedback timer
	DataKeyFeedbackFollowupTimerID DataKey = "feedbackFollowupTimerID" // For tracking follow-up feedback timer
	DataKeyScheduleRegistry        DataKey = "scheduleRegistry"        // For storing active schedules metadata
	DataKeyConversationState       DataKey = "conversationState"       // For tracking current conversation sub-state (INTAKE or FEEDBACK)
	DataKeyStateTransitionTimerID  DataKey = "stateTransitionTimerID"  // For delayed state transitions
	DataKeyLastPromptSentAt        DataKey = "lastPromptSentAt"        // Timestamp of last scheduled habit prompt delivery (RFC3339)
	DataKeyAutoFeedbackTimerID     DataKey = "autoFeedbackTimerID"     // Timer ID for 5-min auto feedback enforcement
)
