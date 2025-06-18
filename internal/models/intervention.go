// Package models defines data structures for the micro health intervention flow.
package models

import (
	"errors"
	"fmt"
	"regexp"
	"time"
)

// InterventionParticipant represents a participant in the micro health intervention study.
type InterventionParticipant struct {
	ID          string    `json:"id"`              // Unique participant identifier
	PhoneNumber string    `json:"phone_number"`    // Participant's phone number
	Name        string    `json:"name,omitempty"`  // Optional participant name
	EnrolledAt  time.Time `json:"enrolled_at"`     // When participant was enrolled
	Status      string    `json:"status"`          // Current participant status
	CurrentState string   `json:"current_state"`   // Current state in the intervention flow
	Timezone    string    `json:"timezone"`        // Participant's timezone (e.g., "America/New_York")
	
	// Intervention-specific fields
	ScheduleTime        string                 `json:"schedule_time"`          // Daily prompt time (e.g., "10:00")
	HasSeenOrientation  bool                   `json:"has_seen_orientation"`   // Whether orientation was sent
	TimesCompletedWeek  int                    `json:"times_completed_week"`   // Weekly completion counter
	WeekStartDate       time.Time              `json:"week_start_date"`        // Start of current week
	LastPromptDate      time.Time              `json:"last_prompt_date"`       // Last prompt sent date
	CustomData          map[string]interface{} `json:"custom_data,omitempty"`  // Additional custom data
}

// ParticipantStatus constants
const (
	ParticipantStatusActive    = "active"
	ParticipantStatusPaused    = "paused"
	ParticipantStatusCompleted = "completed"
	ParticipantStatusWithdrawn = "withdrawn"
)

// InterventionResponse represents a participant's response to an intervention prompt.
type InterventionResponse struct {
	ID            string    `json:"id"`
	ParticipantID string    `json:"participant_id"`
	State         string    `json:"state"`         // State when response was received
	ResponseText  string    `json:"response_text"` // The actual response content
	ResponseType  string    `json:"response_type"` // Type of response (poll_option, free_text, etc.)
	ReceivedAt    time.Time `json:"received_at"`   // When response was received
	ProcessedAt   time.Time `json:"processed_at"`  // When response was processed
}

// ResponseType constants
const (
	ResponseTypePollOption = "poll_option"
	ResponseTypeFreeText   = "free_text"
	ResponseTypeReady      = "ready"
	ResponseTypeTimeout    = "timeout"
)

// InterventionMessage represents a message sent to a participant.
type InterventionMessage struct {
	ID            string    `json:"id"`
	ParticipantID string    `json:"participant_id"`
	State         string    `json:"state"`          // State when message was sent
	MessageType   string    `json:"message_type"`   // Type of message
	Content       string    `json:"content"`        // Message content
	SentAt        time.Time `json:"sent_at"`        // When message was sent
	DeliveredAt   time.Time `json:"delivered_at"`   // When message was delivered
	ReadAt        time.Time `json:"read_at"`        // When message was read
}

// MessageType constants
const (
	MessageTypeOrientation = "orientation"
	MessageTypeCommitment  = "commitment"
	MessageTypeFeeling     = "feeling"
	MessageTypeIntervention = "intervention"
	MessageTypeFollowUp    = "follow_up"
	MessageTypeWeeklySummary = "weekly_summary"
)

// EnrollmentRequest represents a request to enroll a new participant.
type EnrollmentRequest struct {
	PhoneNumber  string `json:"phone_number" binding:"required"`
	Name         string `json:"name,omitempty"`
	Timezone     string `json:"timezone,omitempty"`     // Defaults to UTC
	ScheduleTime string `json:"schedule_time,omitempty"` // Defaults to "10:00"
}

// ResponseProcessingRequest represents a request to process a participant's response.
type ResponseProcessingRequest struct {
	ResponseText string `json:"response_text" binding:"required"`
	ResponseType string `json:"response_type,omitempty"` // Defaults to free_text
}

// ParticipantStateAdvanceRequest represents a request to manually advance a participant's state.
type ParticipantStateAdvanceRequest struct {
	ToState string `json:"to_state" binding:"required"`
	Reason  string `json:"reason,omitempty"`
}

// WeeklySummaryRequest represents a request to trigger weekly summary processing.
type WeeklySummaryRequest struct {
	ParticipantIDs []string `json:"participant_ids,omitempty"` // If empty, process all eligible participants
	ForceAll       bool     `json:"force_all,omitempty"`       // Force summary for all participants regardless of timing
}

// ParticipantStatusUpdateRequest represents a request to update participant status.
type ParticipantStatusUpdateRequest struct {
	Status string `json:"status" binding:"required"`
	Reason string `json:"reason,omitempty"`
}

// FlowStateTransitionRequest represents a request to transition flow state.
type FlowStateTransitionRequest struct {
	FromState string `json:"from_state,omitempty"` // Optional validation
	ToState   string `json:"to_state" binding:"required"`
	Reason    string `json:"reason,omitempty"`
	Force     bool   `json:"force,omitempty"` // Force transition without validation
}

// MessageGenerationRequest represents a request to generate a message for current state.
type MessageGenerationRequest struct {
	MessageType string            `json:"message_type,omitempty"` // Override automatic type detection
	Variables   map[string]string `json:"variables,omitempty"`    // Template variables
	Preview     bool              `json:"preview,omitempty"`      // Generate preview without sending
}

// MessageSendRequest represents a request to send a message to a participant.
type MessageSendRequest struct {
	Content     string `json:"content" binding:"required"`
	MessageType string `json:"message_type,omitempty"`
	ScheduleAt  string `json:"schedule_at,omitempty"` // RFC3339 format, optional scheduling
}

// DailyPromptTriggerRequest represents a request to trigger daily prompts.
type DailyPromptTriggerRequest struct {
	ParticipantIDs []string `json:"participant_ids,omitempty"` // If empty, trigger for all eligible
	ForceAll       bool     `json:"force_all,omitempty"`       // Force trigger regardless of timing
	DryRun         bool     `json:"dry_run,omitempty"`         // Preview without sending
}

// ReadyOverrideRequest represents a "Ready" override request.
type ReadyOverrideRequest struct {
	ParticipantPhone string `json:"participant_phone,omitempty"` // Alternative to participant ID
	Source           string `json:"source,omitempty"`            // Source of the override (sms, whatsapp, etc.)
}Source           string `json:"source,omitempty"`            // Source of the override (sms, whatsapp, etc.)
}
// BulkEnrollmentRequest represents a request to enroll multiple participants.
type BulkEnrollmentRequest struct { a request to enroll multiple participants.
	Participants []EnrollmentRequest `json:"participants" binding:"required"`
	DryRun       bool                `json:"dry_run,omitempty"` // Preview without enrolling
}DryRun       bool                `json:"dry_run,omitempty"` // Preview without enrolling
}
// BulkStatusUpdateRequest represents a request to update multiple participant statuses.
type BulkStatusUpdateRequest struct { a request to update multiple participant statuses.
	ParticipantIDs []string `json:"participant_ids" binding:"required"`
	Status         string   `json:"status" binding:"required"`equired"`
	Reason         string   `json:"reason,omitempty"`equired"`
}Reason         string   `json:"reason,omitempty"`
}
// BulkMessageRequest represents a request to send bulk messages.
type BulkMessageRequest struct { a request to send bulk messages.
	ParticipantIDs []string          `json:"participant_ids,omitempty"` // If empty, send to all active
	Content        string            `json:"content" binding:"required"`// If empty, send to all active
	MessageType    string            `json:"message_type,omitempty"`ed"`
	Variables      map[string]string `json:"variables,omitempty"` // Template variables per participant
	ScheduleAt     string            `json:"schedule_at,omitempty"`/ Template variables per participant
}ScheduleAt     string            `json:"schedule_at,omitempty"`
}
// ScheduleUpdateRequest represents a request to update participant schedule.
type ScheduleUpdateRequest struct { a request to update participant schedule.
	Timezone     string `json:"timezone,omitempty"`
	ScheduleTime string `json:"schedule_time,omitempty"`
}ScheduleTime string `json:"schedule_time,omitempty"`
}
// DataExportRequest represents a request to export intervention data.
type DataExportRequest struct { a request to export intervention data.
	ParticipantIDs []string `json:"participant_ids,omitempty"` // If empty, export all
	StartDate      string   `json:"start_date,omitempty"`      // RFC3339 formatrt all
	EndDate        string   `json:"end_date,omitempty"`        // RFC3339 format
	Format         string   `json:"format,omitempty"`          // json, csv, etc.
	IncludeFields  []string `json:"include_fields,omitempty"`  // Specific fields to include
}IncludeFields  []string `json:"include_fields,omitempty"`  // Specific fields to include
}
// InterventionConfig represents intervention configuration.
type InterventionConfig struct { intervention configuration.
	DefaultTimezone         string            `json:"default_timezone"`
	DefaultScheduleTime     string            `json:"default_schedule_time"`
	CommitmentTimeout       int               `json:"commitment_timeout_hours"`    // Hours
	FeelingTimeout          int               `json:"feeling_timeout_minutes"`     // Minutes  
	CompletionTimeout       int               `json:"completion_timeout_minutes"`  // Minutes  
	GotChanceTimeout        int               `json:"got_chance_timeout_minutes"`  // Minutes
	ContextTimeout          int               `json:"context_timeout_minutes"`     // Minutes
	MoodTimeout             int               `json:"mood_timeout_minutes"`        // Minutes
	BarrierDetailTimeout    int               `json:"barrier_detail_timeout_minutes"` // Minutes
	BarrierReasonTimeout    int               `json:"barrier_reason_timeout_minutes"` // Minutes
	MessageTemplates        map[string]string `json:"message_templates"`out_minutes"` // Minutes
	EnableAutoTransitions   bool              `json:"enable_auto_transitions"`
	EnableWeeklySummary     bool              `json:"enable_weekly_summary"`"`
	RandomAssignmentEnabled bool              `json:"random_assignment_enabled"`
}RandomAssignmentEnabled bool              `json:"random_assignment_enabled"`
}
// FlowStateDetails represents detailed flow state information.
type FlowStateDetails struct { detailed flow state information.
	ParticipantID   string            `json:"participant_id"`
	FlowType        string            `json:"flow_type"`_id"`
	CurrentState    string            `json:"current_state"`
	StateData       map[string]string `json:"state_data"`e"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
	ActiveTimers    []TimerInfo       `json:"active_timers"`
	PossibleStates  []string          `json:"possible_next_states"`
	StateValidation StateValidation   `json:"state_validation"`es"`
}StateValidation StateValidation   `json:"state_validation"`
}
// TimerInfo represents timer information.
type TimerInfo struct { timer information.
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	ExpiresAt time.Time `json:"expires_at"`
	Remaining int64     `json:"remaining_seconds"`
}Remaining int64     `json:"remaining_seconds"`
}
// StateValidation represents state validation information.
type StateValidation struct { state validation information.
	IsValid          bool     `json:"is_valid"`
	ValidationErrors []string `json:"validation_errors,omitempty"`
	CanTransition    bool     `json:"can_transition"`s,omitempty"`
	BlockedReasons   []string `json:"blocked_reasons,omitempty"`
}BlockedReasons   []string `json:"blocked_reasons,omitempty"`
}
// DailyStatus represents a participant's daily flow status.
type DailyStatus struct { a participant's daily flow status.
	ParticipantID         string    `json:"participant_id"`
	Date                  string    `json:"date"` // YYYY-MM-DD format
	CurrentState          string    `json:"current_state"`MM-DD format
	FlowCompleted         bool      `json:"flow_completed"`
	CompletedAt           time.Time `json:"completed_at,omitempty"`
	MessagesReceived      int       `json:"messages_received"`pty"`
	ResponsesGiven        int       `json:"responses_given"`"`
	LastActivity          time.Time `json:"last_activity"`"`
	CompletionStatus      string    `json:"completion_status"` // done, no, no_reply, pending
	InterventionAssignment string   `json:"intervention_assignment,omitempty"` // IMMEDIATE, REFLECTIVE
}InterventionAssignment string   `json:"intervention_assignment,omitempty"` // IMMEDIATE, REFLECTIVE
}
// CompletionRateData represents participant completion rate information.
type CompletionRateData struct { participant completion rate information.
	ParticipantID     string  `json:"participant_id"`
	WeeklyRate        float64 `json:"weekly_completion_rate"`
	OverallRate       float64 `json:"overall_completion_rate"`
	CurrentStreak     int     `json:"current_streak_days"`te"`
	LongestStreak     int     `json:"longest_streak_days"`
	TotalCompletions  int     `json:"total_completions"`"`
	TotalOpportunities int    `json:"total_opportunities"`
	LastCompletion    time.Time `json:"last_completion"`"`
	WeekStartDate     time.Time `json:"week_start_date"`
}WeekStartDate     time.Time `json:"week_start_date"`
}
// WeeklySummaryData represents weekly summary data for a participant.
type WeeklySummaryData struct { weekly summary data for a participant.
	ParticipantID      string    `json:"participant_id"`
	WeekStartDate      time.Time `json:"week_start_date"`
	WeekEndDate        time.Time `json:"week_end_date"`"`
	CompletionsThisWeek int      `json:"completions_this_week"`
	TotalPrompts       int       `json:"total_prompts"`s_week"`
	CompletionRate     float64   `json:"completion_rate"`
	StreakDays         int       `json:"streak_days"`te"`
	MissedDays         int       `json:"missed_days"`
	AverageResponseTime float64  `json:"average_response_time_minutes"`
	MostCommonBarriers []string  `json:"most_common_barriers"`minutes"`
	MoodDistribution   map[string]int `json:"mood_distribution"`
	ContextDistribution map[string]int `json:"context_distribution"`
}ContextDistribution map[string]int `json:"context_distribution"`
}
// FlowAnalytics represents analytics for the intervention flow.
type FlowAnalytics struct { analytics for the intervention flow.
	TotalParticipants     int                    `json:"total_participants"`
	ActiveParticipants    int                    `json:"active_participants"`
	StateDistribution     map[string]int         `json:"state_distribution"``
	CompletionRates       map[string]float64     `json:"completion_rates_by_state"`
	AverageTimeInState    map[string]float64     `json:"average_time_in_state_minutes"`
	TransitionCounts      map[string]int         `json:"transition_counts"`te_minutes"`
	DropoffPoints         []StateDropoffInfo     `json:"dropoff_points"`s"`
	EngagementMetrics     EngagementMetrics      `json:"engagement_metrics"`
	ResponsePatterns      ResponsePatternAnalysis `json:"response_patterns"`
	InterventionEffectiveness InterventionEffectiveness `json:"intervention_effectiveness"`
}InterventionEffectiveness InterventionEffectiveness `json:"intervention_effectiveness"`
}
// StateDropoffInfo represents dropoff information for a state.
type StateDropoffInfo struct { dropoff information for a state.
	State       string  `json:"state"`
	DropoffRate float64 `json:"dropoff_rate"`
	Count       int     `json:"count"`_rate"`
}Count       int     `json:"count"`
}
// EngagementMetrics represents engagement metrics.
type EngagementMetrics struct { engagement metrics.
	AverageResponseTime   float64 `json:"average_response_time_minutes"`
	ReadyOverrideUsage    int     `json:"ready_override_usage_count"`s"`
	TimeoutRate           float64 `json:"timeout_rate"`_usage_count"`
	CompletionStreaks     map[string]int `json:"completion_streaks"` // streak_length -> count
	WeeklyRetention       float64 `json:"weekly_retention_rate"`ks"` // streak_length -> count
}WeeklyRetention       float64 `json:"weekly_retention_rate"`
}
// ResponsePatternAnalysis represents response pattern analysis.
type ResponsePatternAnalysis struct { response pattern analysis.
	MostCommonResponses   map[string]int `json:"most_common_responses"`
	ResponseTimesByState  map[string]float64 `json:"avg_response_times_by_state"`
	FeelingDistribution   map[string]int `json:"feeling_distribution"`_by_state"`
	BarrierFrequency      map[string]int `json:"barrier_frequency"`n"`
	ContextPreferences    map[string]int `json:"context_preferences"`
	MoodPatterns          map[string]int `json:"mood_patterns"`nces"`
}MoodPatterns          map[string]int `json:"mood_patterns"`
}
// InterventionEffectiveness represents intervention effectiveness metrics.
type InterventionEffectiveness struct { intervention effectiveness metrics.
	ImmediateVsReflective map[string]float64 `json:"immediate_vs_reflective_completion"`
	EffectivenessByTime   map[string]float64 `json:"effectiveness_by_time_of_day"`tion"`
	EffectivenessByDay    map[string]float64 `json:"effectiveness_by_day_of_week"`
	ContextEffectiveness  map[string]float64 `json:"effectiveness_by_context"`ek"`
	MoodEffectiveness     map[string]float64 `json:"effectiveness_by_mood"`t"`
}MoodEffectiveness     map[string]float64 `json:"effectiveness_by_mood"`
}
// BulkOperationResult represents the result of a bulk operation.
type BulkOperationResult struct { the result of a bulk operation.
	TotalRequested int                    `json:"total_requested"`
	Successful     int                    `json:"successful"`ted"`
	Failed         int                    `json:"failed"`ul"`
	Errors         []BulkOperationError   `json:"errors,omitempty"`
	Results        []BulkOperationItem    `json:"results,omitempty"`
}Results        []BulkOperationItem    `json:"results,omitempty"`
}
// BulkOperationError represents an error in a bulk operation.
type BulkOperationError struct { an error in a bulk operation.
	Index   int    `json:"index"` {
	ID      string `json:"id,omitempty"`
	Message string `json:"message"`pty"`
}Message string `json:"message"`
}
// BulkOperationItem represents an item result in a bulk operation.
type BulkOperationItem struct { an item result in a bulk operation.
	Index  int         `json:"index"`
	ID     string      `json:"id"`x"`
	Status string      `json:"status"` // success, error, skipped
	Result interface{} `json:"result,omitempty"`s, error, skipped
}Result interface{} `json:"result,omitempty"`
}
// StateInfo represents information about a flow state.
type StateInfo struct { information about a flow state.
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	PossibleNextStates []string `json:"possible_next_states"`
	RequiredData      []string `json:"required_data"`states"`
	OptionalData      []string `json:"optional_data"`
	Timeouts          []string `json:"timeouts"`ata"`
	MessageTemplate   string   `json:"message_template,omitempty"`
}MessageTemplate   string   `json:"message_template,omitempty"`
}
// Available state information for the micro health intervention flow
var StateInfoMap = map[string]StateInfo{icro health intervention flow
	"ORIENTATION": {= map[string]StateInfo{
		Name:              "Orientation",
		Description:       "Initial welcome and orientation message",
		PossibleNextStates: []string{"COMMITMENT_PROMPT"},n message",
		RequiredData:      []string{},COMMITMENT_PROMPT"},
		OptionalData:      []string{"participant_name"},
		Timeouts:          []string{},articipant_name"},
		MessageTemplate:   "orientation",
	},essageTemplate:   "orientation",
	"COMMITMENT_PROMPT": {
		Name:              "Commitment Prompt", 
		Description:       "Daily commitment prompt",
		PossibleNextStates: []string{"FEELING_PROMPT", "END_OF_DAY"},
		RequiredData:      []string{},FEELING_PROMPT", "END_OF_DAY"},
		OptionalData:      []string{},
		Timeouts:          []string{"COMMITMENT_TIMEOUT"},
		MessageTemplate:   "commitment",MITMENT_TIMEOUT"},
	},essageTemplate:   "commitment",
	"FEELING_PROMPT": {
		Name:              "Feeling Prompt",
		Description:       "How participant feels about the habit",
		PossibleNextStates: []string{"RANDOM_ASSIGNMENT"},e habit",
		RequiredData:      []string{},RANDOM_ASSIGNMENT"},
		OptionalData:      []string{},
		Timeouts:          []string{"FEELING_TIMEOUT"},
		MessageTemplate:   "feeling",FEELING_TIMEOUT"},
	},essageTemplate:   "feeling",
	"RANDOM_ASSIGNMENT": {
		Name:              "Random Assignment",
		Description:       "Random assignment to immediate or reflective intervention",
		PossibleNextStates: []string{"SEND_INTERVENTION_IMMEDIATE", "SEND_INTERVENTION_REFLECTIVE"},
		RequiredData:      []string{},SEND_INTERVENTION_IMMEDIATE", "SEND_INTERVENTION_REFLECTIVE"},
		OptionalData:      []string{"feeling_response"},
		Timeouts:          []string{},eeling_response"},
		MessageTemplate:   "",tring{},
	},essageTemplate:   "",
	"SEND_INTERVENTION_IMMEDIATE": {
		Name:              "Send Intervention (Immediate)",
		Description:       "Immediate action intervention",
		PossibleNextStates: []string{"REINFORCEMENT_FOLLOWUP", "DID_YOU_GET_A_CHANCE"},
		RequiredData:      []string{},REINFORCEMENT_FOLLOWUP", "DID_YOU_GET_A_CHANCE"},
		OptionalData:      []string{},
		Timeouts:          []string{"COMPLETION_TIMEOUT"},
		MessageTemplate:   "intervention_immediate",OUT"},
	},essageTemplate:   "intervention_immediate",
	"SEND_INTERVENTION_REFLECTIVE": {
		Name:              "Send Intervention (Reflective)",
		Description:       "Reflective intervention",tive)",
		PossibleNextStates: []string{"REINFORCEMENT_FOLLOWUP", "DID_YOU_GET_A_CHANCE"},
		RequiredData:      []string{},REINFORCEMENT_FOLLOWUP", "DID_YOU_GET_A_CHANCE"},
		OptionalData:      []string{},
		Timeouts:          []string{"COMPLETION_TIMEOUT"},
		MessageTemplate:   "intervention_reflective",UT"},
	},essageTemplate:   "intervention_reflective",
	"REINFORCEMENT_FOLLOWUP": {
		Name:              "Reinforcement Follow-up",
		Description:       "Positive reinforcement for completion",
		PossibleNextStates: []string{"END_OF_DAY"},for completion",
		RequiredData:      []string{},END_OF_DAY"},
		OptionalData:      []string{},
		Timeouts:          []string{},
		MessageTemplate:   "reinforcement",
	},essageTemplate:   "reinforcement",
	"DID_YOU_GET_A_CHANCE": {
		Name:              "Did You Get A Chance",
		Description:       "Ask if participant got a chance to try",
		PossibleNextStates: []string{"CONTEXT_QUESTION", "BARRIER_REASON_NO_CHANCE", "IGNORED_PATH"},
		RequiredData:      []string{},CONTEXT_QUESTION", "BARRIER_REASON_NO_CHANCE", "IGNORED_PATH"},
		OptionalData:      []string{},
		Timeouts:          []string{"GOT_CHANCE_TIMEOUT"},
		MessageTemplate:   "did_you_get_a_chance",MEOUT"},
	},essageTemplate:   "did_you_get_a_chance",
	"CONTEXT_QUESTION": {
		Name:              "Context Question",
		Description:       "Ask about context when habit was done",
		PossibleNextStates: []string{"MOOD_QUESTION", "END_OF_DAY"},
		RequiredData:      []string{},MOOD_QUESTION", "END_OF_DAY"},
		OptionalData:      []string{},
		Timeouts:          []string{"CONTEXT_TIMEOUT"},
		MessageTemplate:   "context_question",IMEOUT"},
	},essageTemplate:   "context_question",
	"MOOD_QUESTION": {
		Name:              "Mood Question",
		Description:       "Ask about mood before doing habit",
		PossibleNextStates: []string{"BARRIER_CHECK_AFTER_CONTEXT_MOOD", "END_OF_DAY"},
		RequiredData:      []string{"context_response"},R_CONTEXT_MOOD", "END_OF_DAY"},
		OptionalData:      []string{},ontext_response"},
		Timeouts:          []string{"MOOD_TIMEOUT"},
		MessageTemplate:   "mood_question",IMEOUT"},
	},essageTemplate:   "mood_question",
	"BARRIER_CHECK_AFTER_CONTEXT_MOOD": {
		Name:              "Barrier Check After Context & Mood",
		Description:       "Ask about barriers/facilitators",d",
		PossibleNextStates: []string{"END_OF_DAY"},litators",
		RequiredData:      []string{"context_response", "mood_response"},
		OptionalData:      []string{},ontext_response", "mood_response"},
		Timeouts:          []string{"BARRIER_DETAIL_TIMEOUT"},
		MessageTemplate:   "barrier_check",R_DETAIL_TIMEOUT"},
	},essageTemplate:   "barrier_check",
	"BARRIER_REASON_NO_CHANCE": {
		Name:              "Barrier Reason (No Chance)",
		Description:       "Ask why participant couldn't try",
		PossibleNextStates: []string{"END_OF_DAY"},ldn't try",
		RequiredData:      []string{},END_OF_DAY"},
		OptionalData:      []string{},
		Timeouts:          []string{"BARRIER_REASON_TIMEOUT"},
		MessageTemplate:   "barrier_reason",_REASON_TIMEOUT"},
	},essageTemplate:   "barrier_reason",
	"IGNORED_PATH": {
		Name:              "Ignored Path",
		Description:       "Handle non-responsive participants",
		PossibleNextStates: []string{"END_OF_DAY"},articipants",
		RequiredData:      []string{},END_OF_DAY"},
		OptionalData:      []string{},
		Timeouts:          []string{},
		MessageTemplate:   "ignored_path",
	},essageTemplate:   "ignored_path",
	"END_OF_DAY": {
		Name:              "End of Day",
		Description:       "Daily flow completed",
		PossibleNextStates: []string{"COMMITMENT_PROMPT", "WEEKLY_SUMMARY"},
		RequiredData:      []string{},COMMITMENT_PROMPT", "WEEKLY_SUMMARY"},
		OptionalData:      []string{},
		Timeouts:          []string{},
		MessageTemplate:   "",tring{},
	},essageTemplate:   "",
	"WEEKLY_SUMMARY": {
		Name:              "Weekly Summary",
		Description:       "Weekly summary message",
		PossibleNextStates: []string{"END_OF_DAY"},,
		RequiredData:      []string{"times_completed_week"},
		OptionalData:      []string{},imes_completed_week"},
		Timeouts:          []string{},
		MessageTemplate:   "weekly_summary",
	},essageTemplate:   "weekly_summary",
}},
}
// Validation constants
const (dation constants
	MaxParticipantNameLength = 100
	MaxResponseTextLength    = 1000
	MaxReasonLength         = 50000
)MaxReasonLength         = 500
)
// Error variables for validation
var (ror variables for validation
	ErrInvalidPhoneNumber  = errors.New("invalid phone number format")
	ErrInvalidTimezone     = errors.New("invalid timezone")er format")
	ErrInvalidScheduleTime = errors.New("invalid schedule time format")
	ErrInvalidStatus       = errors.New("invalid participant status")")
	ErrInvalidResponseType = errors.New("invalid response type")tus")
	ErrInvalidMessageType  = errors.New("invalid message type"))
)ErrInvalidMessageType  = errors.New("invalid message type")
)
// Phone number validation regex (basic E.164 format)
var phoneRegex = regexp.MustCompile(`^\+[1-9]\d{1,14}$`)
var phoneRegex = regexp.MustCompile(`^\+[1-9]\d{1,14}$`)
// Time format validation regex (HH:MM format)
var timeRegex = regexp.MustCompile(`^([0-1]?[0-9]|2[0-3]):[0-5][0-9]$`)
var timeRegex = regexp.MustCompile(`^([0-1]?[0-9]|2[0-3]):[0-5][0-9]$`)
// Validate validates an EnrollmentRequest.
func (r *EnrollmentRequest) Validate() error {
	if r.PhoneNumber == "" {t) Validate() error {
		return errors.New("phone number is required")
	}return errors.New("phone number is required")
	}
	if !phoneRegex.MatchString(r.PhoneNumber) {
		return ErrInvalidPhoneNumberPhoneNumber) {
	}return ErrInvalidPhoneNumber
	}
	if r.Name != "" && len(r.Name) > MaxParticipantNameLength {
		return errors.New("participant name too long")NameLength {
	}return errors.New("participant name too long")
	}
	if r.Timezone != "" {
		if _, err := time.LoadLocation(r.Timezone); err != nil {
			return ErrInvalidTimezonetion(r.Timezone); err != nil {
		}return ErrInvalidTimezone
	}}
	}
	if r.ScheduleTime != "" && !timeRegex.MatchString(r.ScheduleTime) {
		return ErrInvalidScheduleTimemeRegex.MatchString(r.ScheduleTime) {
	}return ErrInvalidScheduleTime
	}
	return nil
}return nil
}
// Validate validates a ResponseProcessingRequest.
func (r *ResponseProcessingRequest) Validate() error {
	if r.ResponseText == "" {gRequest) Validate() error {
		return errors.New("response text is required")
	}return errors.New("response text is required")
	}
	if len(r.ResponseText) > MaxResponseTextLength {
		return errors.New("response text too long")th {
	}return errors.New("response text too long")
	}
	if r.ResponseType != "" && !isValidResponseType(r.ResponseType) {
		return ErrInvalidResponseTypeValidResponseType(r.ResponseType) {
	}return ErrInvalidResponseType
	}
	return nil
}return nil
}
// Validate validates a ParticipantStateAdvanceRequest.
func (r *ParticipantStateAdvanceRequest) Validate() error {
	if r.ToState == "" {tateAdvanceRequest) Validate() error {
		return errors.New("to_state is required")
	}return errors.New("to_state is required")
	}
	if r.Reason != "" && len(r.Reason) > MaxReasonLength {
		return errors.New("reason too long")MaxReasonLength {
	}return errors.New("reason too long")
	}
	return nil
}return nil
}
// Validate validates an InterventionParticipant.
func (p *InterventionParticipant) Validate() error {
	if p.PhoneNumber == "" {icipant) Validate() error {
		return errors.New("phone number is required")
	}return errors.New("phone number is required")
	}
	if !phoneRegex.MatchString(p.PhoneNumber) {
		return ErrInvalidPhoneNumberPhoneNumber) {
	}return ErrInvalidPhoneNumber
	}
	if p.Name != "" && len(p.Name) > MaxParticipantNameLength {
		return errors.New("participant name too long")NameLength {
	}return errors.New("participant name too long")
	}
	if !isValidParticipantStatus(p.Status) {
		return ErrInvalidStatusatus(p.Status) {
	}return ErrInvalidStatus
	}
	if p.Timezone != "" {
		if _, err := time.LoadLocation(p.Timezone); err != nil {
			return ErrInvalidTimezonetion(p.Timezone); err != nil {
		}return ErrInvalidTimezone
	}}
	}
	if p.ScheduleTime != "" && !timeRegex.MatchString(p.ScheduleTime) {
		return ErrInvalidScheduleTimemeRegex.MatchString(p.ScheduleTime) {
	}return ErrInvalidScheduleTime
	}
	return nil
}return nil
}
// Validate validates a ParticipantStatusUpdateRequest.
func (r *ParticipantStatusUpdateRequest) Validate() error {
	if r.Status == "" {StatusUpdateRequest) Validate() error {
		return errors.New("status is required")
	}return errors.New("status is required")
	}
	if !isValidParticipantStatus(r.Status) {
		return ErrInvalidStatusatus(r.Status) {
	}return ErrInvalidStatus
	}
	if r.Reason != "" && len(r.Reason) > MaxReasonLength {
		return errors.New("reason too long")MaxReasonLength {
	}return errors.New("reason too long")
	}
	return nil
}return nil
}
// Validate validates a FlowStateTransitionRequest.
func (r *FlowStateTransitionRequest) Validate() error {
	if r.ToState == "" {nsitionRequest) Validate() error {
		return errors.New("to_state is required")
	}return errors.New("to_state is required")
	}
	// Validate that the state exists
	if _, exists := StateInfoMap[r.ToState]; !exists {
		return errors.New("invalid to_state")]; !exists {
	}return errors.New("invalid to_state")
	}
	if r.FromState != "" {
		if _, exists := StateInfoMap[r.FromState]; !exists {
			return errors.New("invalid from_state")]; !exists {
		}return errors.New("invalid from_state")
	}}
	}
	if r.Reason != "" && len(r.Reason) > MaxReasonLength {
		return errors.New("reason too long")MaxReasonLength {
	}return errors.New("reason too long")
	}
	return nil
}return nil
}
// Validate validates a MessageGenerationRequest.
func (r *MessageGenerationRequest) Validate() error {
	if r.MessageType != "" && !isValidMessageType(r.MessageType) {
		return ErrInvalidMessageTypeValidMessageType(r.MessageType) {
	}return ErrInvalidMessageType
	}
	return nil
}return nil
}
// Validate validates a MessageSendRequest.
func (r *MessageSendRequest) Validate() error {
	if r.Content == "" {equest) Validate() error {
		return errors.New("content is required")
	}return errors.New("content is required")
	}
	if len(r.Content) > MaxResponseTextLength {
		return errors.New("content too long")gth {
	}return errors.New("content too long")
	}
	if r.MessageType != "" && !isValidMessageType(r.MessageType) {
		return ErrInvalidMessageTypeValidMessageType(r.MessageType) {
	}return ErrInvalidMessageType
	}
	if r.ScheduleAt != "" {
		if _, err := time.Parse(time.RFC3339, r.ScheduleAt); err != nil {
			return errors.New("invalid schedule_at format, must be RFC3339")
		}return errors.New("invalid schedule_at format, must be RFC3339")
	}}
	}
	return nil
}return nil
}
// Validate validates a ReadyOverrideRequest.
func (r *ReadyOverrideRequest) Validate() error {
	if r.ParticipantID == "" && r.ParticipantPhone == "" {) Validate() error {
		return errors.New("either participant_id or participant_phone is required")
	}return errors.New("participants list cannot be empty")
	}
	if r.ParticipantID != "" && r.ParticipantPhone != "" {
		return errors.New("cannot specify both participant_id and participant_phone")
	}return errors.New("too many participants in single request (max 100)")
	}
	return nil
}

// Validate validates a BulkEnrollmentRequest.return fmt.Errorf("participant %d: %w", i, err)
func (r *BulkEnrollmentRequest) Validate() error {}
	if len(r.Participants) == 0 {}
		return errors.New("participants list cannot be empty")
	}return nil
	}
	if len(r.Participants) > 100 { // Reasonable limit
		return errors.New("too many participants in single request (max 100)")
	}) Validate() error {
	
	for i, participant := range r.Participants {return errors.New("participant_ids cannot be empty")
		if err := participant.Validate(); err != nil {}
			return fmt.Errorf("participant %d: %w", i, err)
		}
	}return errors.New("too many participant IDs in single request (max 100)")
	}
	return nil
}
return errors.New("status is required")
// Validate validates a BulkStatusUpdateRequest.}
func (r *BulkStatusUpdateRequest) Validate() error {
	if len(r.ParticipantIDs) == 0 {atus(r.Status) {
		return errors.New("participant_ids cannot be empty")return ErrInvalidStatus
	}}
	
	if len(r.ParticipantIDs) > 100 { // Reasonable limitMaxReasonLength {
		return errors.New("too many participant IDs in single request (max 100)")return errors.New("reason too long")
	}}
	
	if r.Status == "" {return nil
		return errors.New("status is required")}
	}
	
	if !isValidParticipantStatus(r.Status) {equest) Validate() error {
		return ErrInvalidStatus
	}return errors.New("content is required")
	}
	if r.Reason != "" && len(r.Reason) > MaxReasonLength {
		return errors.New("reason too long")gth {
	}return errors.New("content too long")
	}
	return nil
}ValidMessageType(r.MessageType) {
return ErrInvalidMessageType
// Validate validates a BulkMessageRequest.}
func (r *BulkMessageRequest) Validate() error {
	if r.Content == "" {
		return errors.New("content is required")
	}return errors.New("invalid schedule_at format, must be RFC3339")
	}
	if len(r.Content) > MaxResponseTextLength {}
		return errors.New("content too long")
	}
	return errors.New("too many participant IDs in single request (max 1000)")
	if r.MessageType != "" && !isValidMessageType(r.MessageType) {}
		return ErrInvalidMessageType
	}return nil
	}
	if r.ScheduleAt != "" {
		if _, err := time.Parse(time.RFC3339, r.ScheduleAt); err != nil {
			return errors.New("invalid schedule_at format, must be RFC3339")eRequest) Validate() error {
		}
	}tion(r.Timezone); err != nil {
	return ErrInvalidTimezone
	if len(r.ParticipantIDs) > 1000 { // Reasonable limit for bulk operations}
		return errors.New("too many participant IDs in single request (max 1000)")}
	}
	meRegex.MatchString(r.ScheduleTime) {
	return nilreturn ErrInvalidScheduleTime
}}

// Validate validates a ScheduleUpdateRequest.return nil
func (r *ScheduleUpdateRequest) Validate() error {}
	if r.Timezone != "" {
		if _, err := time.LoadLocation(r.Timezone); err != nil {
			return ErrInvalidTimezoneest) Validate() error {
		}
	}
	return errors.New("invalid start_date format, must be RFC3339")
	if r.ScheduleTime != "" && !timeRegex.MatchString(r.ScheduleTime) {}
		return ErrInvalidScheduleTime}
	}
	
	return nil
}return errors.New("invalid end_date format, must be RFC3339")
}
// Validate validates a DataExportRequest.}
func (r *DataExportRequest) Validate() error {
	if r.StartDate != "" {{
		if _, err := time.Parse(time.RFC3339, r.StartDate); err != nil {return errors.New("invalid format, must be 'json' or 'csv'")
			return errors.New("invalid start_date format, must be RFC3339")}
		}
	}return nil
	}
	if r.EndDate != "" {
		if _, err := time.Parse(time.RFC3339, r.EndDate); err != nil {
			return errors.New("invalid end_date format, must be RFC3339")Validate() error {
		}
	}tion(c.DefaultTimezone); err != nil {
	return ErrInvalidTimezone
	if r.Format != "" && r.Format != "json" && r.Format != "csv" {}
		return errors.New("invalid format, must be 'json' or 'csv'")}
	}
	 && !timeRegex.MatchString(c.DefaultScheduleTime) {
	return nilreturn ErrInvalidScheduleTime
}}

// Validate validates an InterventionConfig.
func (c *InterventionConfig) Validate() error {
	if c.DefaultTimezone != "" {return errors.New("commitment_timeout_hours must be between 1 and 24")
		if _, err := time.LoadLocation(c.DefaultTimezone); err != nil {}
			return ErrInvalidTimezone
		}
	}return errors.New("feeling_timeout_minutes must be between 1 and 60")
	}
	if c.DefaultScheduleTime != "" && !timeRegex.MatchString(c.DefaultScheduleTime) {
		return ErrInvalidScheduleTime
	}return errors.New("completion_timeout_minutes must be between 1 and 120")
	}
	// Validate timeout values are reasonable
	if c.CommitmentTimeout < 1 || c.CommitmentTimeout > 24 {return nil
		return errors.New("commitment_timeout_hours must be between 1 and 24")}
	}
	 valid.
	if c.FeelingTimeout < 1 || c.FeelingTimeout > 60 {icipantStatus(status string) bool {
		return errors.New("feeling_timeout_minutes must be between 1 and 60")
	}pantStatusActive, ParticipantStatusPaused, ParticipantStatusCompleted, ParticipantStatusWithdrawn:
	true
	if c.CompletionTimeout < 1 || c.CompletionTimeout > 120 {
		return errors.New("completion_timeout_minutes must be between 1 and 120")return false
	}}
	}
	return nil
}s valid.
pe(responseType string) bool {
// isValidParticipantStatus checks if the status is valid.
func isValidParticipantStatus(status string) bool {eTypePollOption, ResponseTypeFreeText, ResponseTypeReady, ResponseTypeTimeout:
	switch status {true
	case ParticipantStatusActive, ParticipantStatusPaused, ParticipantStatusCompleted, ParticipantStatusWithdrawn:
		return truereturn false
	default:}
		return false}
	}
}s valid.
pe(messageType string) bool {
// isValidResponseType checks if the response type is valid.
func isValidResponseType(responseType string) bool {
	switch responseType {eIntervention, MessageTypeFollowUp, MessageTypeWeeklySummary:
	case ResponseTypePollOption, ResponseTypeFreeText, ResponseTypeReady, ResponseTypeTimeout:true
		return true
	default:return false
		return false}
	}}
}
 summary view of a participant for listing endpoints.
// isValidMessageType checks if the message type is valid.
func isValidMessageType(messageType string) bool {
	switch messageType {
	case MessageTypeOrientation, MessageTypeCommitment, MessageTypeFeeling, tempty"`
		 MessageTypeIntervention, MessageTypeFollowUp, MessageTypeWeeklySummary:
		return true"`
	default:
		return false
	}CompletionRate  float64   `json:"completion_rate"` // Calculated field
}}

// ParticipantSummary provides a summary view of a participant for listing endpoints.tatistics about the intervention.
type ParticipantSummary struct {
	ID              string    `json:"id"`
	PhoneNumber     string    `json:"phone_number"`
	Name            string    `json:"name,omitempty"`
	Status          string    `json:"status"`
	CurrentState    string    `json:"current_state"`te"`
	EnrolledAt      time.Time `json:"enrolled_at"`n"`
	LastPromptDate  time.Time `json:"last_prompt_date"``
	CompletionRate  float64   `json:"completion_rate"` // Calculated fieldTotalMessages        int                    `json:"total_messages"`
}}













}	TotalMessages        int                    `json:"total_messages"`	TotalResponses       int                    `json:"total_responses"`	StateDistribution    map[string]int         `json:"state_distribution"`	AverageCompletionRate float64              `json:"average_completion_rate"`	WithdrawnParticipants int                   `json:"withdrawn_participants"`	CompletedParticipants int                   `json:"completed_participants"`	ActiveParticipants   int                    `json:"active_participants"`	TotalParticipants    int                    `json:"total_participants"`type InterventionStats struct {// InterventionStats provides statistics about the intervention.