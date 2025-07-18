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
	FlowTypeMicroHealthIntervention FlowType = "micro_health_intervention"
	FlowTypeConversation            FlowType = "conversation"
)

// State constants for micro health intervention flow.
const (
	StateOrientation                  StateType = "ORIENTATION"
	StateCommitmentPrompt             StateType = "COMMITMENT_PROMPT"
	StateFeelingPrompt                StateType = "FEELING_PROMPT"
	StateRandomAssignment             StateType = "RANDOM_ASSIGNMENT"
	StateSendInterventionImmediate    StateType = "SEND_INTERVENTION_IMMEDIATE"
	StateSendInterventionReflective   StateType = "SEND_INTERVENTION_REFLECTIVE"
	StateReinforcementFollowup        StateType = "REINFORCEMENT_FOLLOWUP"
	StateDidYouGetAChance             StateType = "DID_YOU_GET_A_CHANCE"
	StateContextQuestion              StateType = "CONTEXT_QUESTION"
	StateMoodQuestion                 StateType = "MOOD_QUESTION"
	StateBarrierCheckAfterContextMood StateType = "BARRIER_CHECK_AFTER_CONTEXT_MOOD"
	StateBarrierReasonNoChance        StateType = "BARRIER_REASON_NO_CHANCE"
	StateIgnoredPath                  StateType = "IGNORED_PATH"
	StateEndOfDay                     StateType = "END_OF_DAY"
	StateHabitReminder                StateType = "HABIT_REMINDER"
	StateFollowUp                     StateType = "FOLLOW_UP"
	StateComplete                     StateType = "COMPLETE"
)

// State constants for conversation flow.
const (
	StateConversationActive StateType = "CONVERSATION_ACTIVE"
)

// Data key constants for state data storage.
const (
	DataKeyFlowAssignment        DataKey = "flowAssignment"
	DataKeyFeelingResponse       DataKey = "feelingResponse"
	DataKeyCompletionResponse    DataKey = "completionResponse"
	DataKeyGotChanceResponse     DataKey = "gotChanceResponse"
	DataKeyContextResponse       DataKey = "contextResponse"
	DataKeyMoodResponse          DataKey = "moodResponse"
	DataKeyBarrierResponse       DataKey = "barrierResponse"
	DataKeyBarrierReasonResponse DataKey = "barrierReasonResponse"

	// Timer ID storage keys for managing timeouts
	DataKeyCommitmentTimerID       DataKey = "commitmentTimerID"
	DataKeyFeelingTimerID          DataKey = "feelingTimerID"
	DataKeyCompletionTimerID       DataKey = "completionTimerID"
	DataKeyDidYouGetAChanceTimerID DataKey = "didYouGetAChanceTimerID"
	DataKeyContextTimerID          DataKey = "contextTimerID"
	DataKeyMoodTimerID             DataKey = "moodTimerID"
	DataKeyBarrierCheckTimerID     DataKey = "barrierCheckTimerID"
	DataKeyBarrierReasonTimerID    DataKey = "barrierReasonTimerID"
)

// Data key constants for conversation flow.
const (
	DataKeyConversationHistory   DataKey = "conversationHistory"
	DataKeySystemPrompt          DataKey = "systemPrompt"
	DataKeyParticipantBackground DataKey = "participantBackground"
)

// Flow assignment values.
const (
	FlowAssignmentImmediate  FlowAssignment = "IMMEDIATE"
	FlowAssignmentReflective FlowAssignment = "REFLECTIVE"
)

// Response values for completion tracking.
const (
	ResponseDone    ResponseValue = "done"
	ResponseNo      ResponseValue = "no"
	ResponseNoReply ResponseValue = "no_reply"
	ResponseReady   ResponseValue = "ready"
)
