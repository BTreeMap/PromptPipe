// Package models defines the core data structures for PromptPipe.
//
// It includes types for prompts and delivery/read receipts, which are shared across modules.
package models

import (
	"errors"
	"fmt"
	"time"
)

// PromptType defines how the prompt content is determined.
type PromptType string

const (
	// PromptTypeStatic sends a static message body.
	PromptTypeStatic PromptType = "static"
	// PromptTypeGenAI generates message content using GenAI.
	PromptTypeGenAI PromptType = "genai"
	// PromptTypeBranch sends a prompt with selectable branches.
	PromptTypeBranch PromptType = "branch"
	// PromptTypeCustom allows pluggable custom flow generators
	PromptTypeCustom PromptType = "custom"
	// PromptTypeConversation enables persistent conversation flows
	PromptTypeConversation PromptType = "conversation"
)

// Validation constants for input validation
const (
	// MaxPromptBodyLength defines the maximum allowed length for prompt body content
	MaxPromptBodyLength = 4096
	// MaxBranchLabelLength defines the maximum allowed length for branch option labels
	MaxBranchLabelLength = 100
	// MaxBranchBodyLength defines the maximum allowed length for branch option body content
	MaxBranchBodyLength = 1000
	// MaxBranchOptionsCount defines the maximum number of branch options allowed
	MaxBranchOptionsCount = 10
	// MinBranchOptionsCount defines the minimum number of branch options required
	MinBranchOptionsCount = 2
)

// Error variables for better error handling and testability
var (
	ErrEmptyRecipient            = errors.New("recipient cannot be empty")
	ErrInvalidPromptType         = errors.New("invalid prompt type")
	ErrEmptyBody                 = errors.New("body is required for static prompts")
	ErrPromptBodyTooLong         = errors.New("prompt body exceeds maximum length")
	ErrMissingSystemPrompt       = errors.New("system prompt is required for GenAI prompts")
	ErrMissingUserPrompt         = errors.New("user prompt is required for GenAI prompts")
	ErrMissingBranchOptions      = errors.New("branch options are required for branch prompts")
	ErrInsufficientBranchOptions = errors.New("insufficient branch options")
	ErrTooManyBranchOptions      = errors.New("too many branch options")
	ErrEmptyBranchLabel          = errors.New("branch label cannot be empty")
	ErrBranchLabelTooLong        = errors.New("branch label exceeds maximum length")
	ErrEmptyBranchBody           = errors.New("branch body cannot be empty")
	ErrBranchBodyTooLong         = errors.New("branch body exceeds maximum length")
)

// IsValidPromptType checks if the given prompt type is supported.
func IsValidPromptType(pt PromptType) bool {
	switch pt {
	case PromptTypeStatic, PromptTypeGenAI, PromptTypeBranch, PromptTypeCustom, PromptTypeConversation:
		return true
	default:
		return false
	}
}

// BranchOption represents a selectable option for branch-type prompts.
type BranchOption struct {
	Label string `json:"label"` // option identifier shown to user
	Body  string `json:"body"`  // message content if selected
}

// Branch represents a structured branching prompt with a main body and selectable options.
// This provides a common structure for both branch prompts and micro health intervention flows.
type Branch struct {
	Body    string         `json:"body"`    // main prompt text
	Options []BranchOption `json:"options"` // selectable options
}

// Generate formats the branch into a complete message with numbered options and instructions.
func (b *Branch) Generate() string {
	if len(b.Options) == 0 {
		return b.Body
	}

	result := b.Body

	// Add a separator line if body doesn't end with newline
	if len(result) > 0 && result[len(result)-1] != '\n' {
		result += "\n"
	}

	// Add options with better formatting
	result += "\n"
	for i, opt := range b.Options {
		result += fmt.Sprintf("%d. %s: %s\n", i+1, opt.Label, opt.Body)
	}

	// Add user instruction with proper grammar
	result += "\n"
	if len(b.Options) == 2 {
		result += "(Reply with '1' or '2')"
	} else {
		// Generate the range dynamically for multiple options
		result += "(Reply with '1'"
		for i := 2; i < len(b.Options); i++ {
			result += fmt.Sprintf(", '%d'", i)
		}
		result += fmt.Sprintf(", or '%d')", len(b.Options))
	}

	return result
}

// Validate ensures the branch has valid structure.
func (b *Branch) Validate() error {
	if len(b.Options) == 0 {
		return ErrMissingBranchOptions
	}
	if len(b.Options) < MinBranchOptionsCount {
		return ErrInsufficientBranchOptions
	}
	if len(b.Options) > MaxBranchOptionsCount {
		return ErrTooManyBranchOptions
	}

	for _, option := range b.Options {
		if option.Label == "" {
			return ErrEmptyBranchLabel
		}
		if len(option.Label) > MaxBranchLabelLength {
			return ErrBranchLabelTooLong
		}
		if option.Body == "" {
			return ErrEmptyBranchBody
		}
		if len(option.Body) > MaxBranchBodyLength {
			return ErrBranchBodyTooLong
		}
	}

	return nil
}

// Prompt represents a message to be sent, supporting static, GenAI, or branch types.
type Prompt struct {
	To            string         `json:"to"`
	Schedule      *Schedule      `json:"schedule,omitempty"`
	Type          PromptType     `json:"type,omitempty"`
	State         StateType      `json:"state,omitempty"` // current state for custom flows
	Body          string         `json:"body,omitempty"`
	SystemPrompt  string         `json:"system_prompt,omitempty"`
	UserPrompt    string         `json:"user_prompt,omitempty"`
	BranchOptions []BranchOption `json:"branch_options,omitempty"`
}

// Validate performs comprehensive validation on a Prompt structure.
func (p *Prompt) Validate() error {
	// Check recipient
	if p.To == "" {
		return ErrEmptyRecipient
	}

	// Check prompt type
	if !IsValidPromptType(p.Type) {
		return ErrInvalidPromptType
	}

	// Type-specific validation
	switch p.Type {
	case PromptTypeStatic:
		return p.validateStatic()
	case PromptTypeGenAI:
		return p.validateGenAI()
	case PromptTypeBranch:
		return p.validateBranch()
	case PromptTypeCustom:
		// Custom types may have different validation requirements
		return nil
	case PromptTypeConversation:
		return p.validateConversation()
	}

	return nil
}

// validateStatic validates static prompt requirements.
func (p *Prompt) validateStatic() error {
	if p.Body == "" {
		return ErrEmptyBody
	}
	if len(p.Body) > MaxPromptBodyLength {
		return ErrPromptBodyTooLong
	}
	return nil
}

// validateGenAI validates GenAI prompt requirements.
func (p *Prompt) validateGenAI() error {
	if p.SystemPrompt == "" {
		return ErrMissingSystemPrompt
	}
	if p.UserPrompt == "" {
		return ErrMissingUserPrompt
	}
	return nil
}

// validateBranch validates branch prompt requirements.
func (p *Prompt) validateBranch() error {
	if len(p.BranchOptions) == 0 {
		return ErrMissingBranchOptions
	}
	if len(p.BranchOptions) < MinBranchOptionsCount {
		return ErrInsufficientBranchOptions
	}
	if len(p.BranchOptions) > MaxBranchOptionsCount {
		return ErrTooManyBranchOptions
	}

	for _, option := range p.BranchOptions {
		if option.Label == "" {
			return ErrEmptyBranchLabel
		}
		if len(option.Label) > MaxBranchLabelLength {
			return ErrBranchLabelTooLong
		}
		if option.Body == "" {
			return ErrEmptyBranchBody
		}
		if len(option.Body) > MaxBranchBodyLength {
			return ErrBranchBodyTooLong
		}
	}

	return nil
}

// validateConversation validates conversation prompt requirements.
func (p *Prompt) validateConversation() error {
	// Conversation prompts require a user prompt but system prompt can be loaded from file
	if p.UserPrompt == "" {
		return ErrMissingUserPrompt
	}
	return nil
}

// MessageStatus represents the delivery status of a message.
type MessageStatus string

const (
	// MessageStatusSent indicates the message was sent.
	MessageStatusSent MessageStatus = "sent"
	// MessageStatusDelivered indicates the message was delivered.
	MessageStatusDelivered MessageStatus = "delivered"
	// MessageStatusRead indicates the message was read.
	MessageStatusRead MessageStatus = "read"
	// MessageStatusFailed indicates the message failed to send.
	MessageStatusFailed MessageStatus = "failed"
	// MessageStatusCancelled indicates the message was cancelled.
	MessageStatusCancelled MessageStatus = "cancelled"
)

// APIStatus represents the status of an API response.
type APIStatus string

const (
	// APIStatusOK indicates an API request completed successfully.
	APIStatusOK APIStatus = "ok"
	// APIStatusError indicates an API request failed with an error.
	APIStatusError APIStatus = "error"
)

type Receipt struct {
	To     string        `json:"to"`
	Status MessageStatus `json:"status"`
	Time   int64         `json:"time"`
}

// Response represents an incoming message response from a participant.
type Response struct {
	From string `json:"from"`
	Body string `json:"body"`
	Time int64  `json:"time"`
}

// API Response types for consistent JSON responses

// APIResponse represents a standard API response with a status and optional data.
type APIResponse struct {
	Status  string      `json:"status"`            // status of the API response
	Message string      `json:"message,omitempty"` // optional message for error responses or additional info
	Result  interface{} `json:"result,omitempty"`  // optional result data for successful responses
}

// APIResponseBuilder provides a fluent interface for building API responses.
type APIResponseBuilder struct {
	response APIResponse
}

// NewAPIResponseBuilder creates a new APIResponseBuilder instance.
func NewAPIResponseBuilder() *APIResponseBuilder {
	return &APIResponseBuilder{
		response: APIResponse{},
	}
}

// WithStatus sets the status of the API response.
func (b *APIResponseBuilder) WithStatus(status APIStatus) *APIResponseBuilder {
	b.response.Status = string(status)
	return b
}

// WithMessage sets the message of the API response.
func (b *APIResponseBuilder) WithMessage(message string) *APIResponseBuilder {
	b.response.Message = message
	return b
}

// WithResult sets the result data of the API response.
func (b *APIResponseBuilder) WithResult(result interface{}) *APIResponseBuilder {
	b.response.Result = result
	return b
}

// Build constructs and returns the final APIResponse.
func (b *APIResponseBuilder) Build() APIResponse {
	return b.response
}

// Convenience functions for common response patterns

// Success creates a successful API response with optional result data.
func Success(result interface{}) APIResponse {
	return NewAPIResponseBuilder().
		WithStatus(APIStatusOK).
		WithResult(result).
		Build()
}

// SuccessWithMessage creates a successful API response with a message and optional result data.
func SuccessWithMessage(message string, result interface{}) APIResponse {
	return NewAPIResponseBuilder().
		WithStatus(APIStatusOK).
		WithMessage(message).
		WithResult(result).
		Build()
}

// Error creates an error API response with a message.
func Error(message string) APIResponse {
	return NewAPIResponseBuilder().
		WithStatus(APIStatusError).
		WithMessage(message).
		Build()
}

// InterventionParticipantStatus represents the enrollment status of a participant.
type InterventionParticipantStatus string

const (
	// ParticipantStatusActive indicates the participant is actively enrolled.
	ParticipantStatusActive InterventionParticipantStatus = "active"
	// ParticipantStatusPaused indicates the participant is temporarily paused.
	ParticipantStatusPaused InterventionParticipantStatus = "paused"
	// ParticipantStatusCompleted indicates the participant has completed the intervention.
	ParticipantStatusCompleted InterventionParticipantStatus = "completed"
	// ParticipantStatusWithdrawn indicates the participant has withdrawn.
	ParticipantStatusWithdrawn InterventionParticipantStatus = "withdrawn"
)

// InterventionParticipant represents a participant in the micro health intervention study.
type InterventionParticipant struct {
	ID              string                        `json:"id"`
	PhoneNumber     string                        `json:"phone_number"`
	Name            string                        `json:"name,omitempty"`
	Timezone        string                        `json:"timezone,omitempty"` // e.g., "America/New_York"
	Status          InterventionParticipantStatus `json:"status"`
	EnrolledAt      time.Time                     `json:"enrolled_at"`
	DailyPromptTime string                        `json:"daily_prompt_time"`      // e.g., "10:00"
	WeeklyReset     time.Time                     `json:"weekly_reset,omitempty"` // When to send weekly summary
	CreatedAt       time.Time                     `json:"created_at"`
	UpdatedAt       time.Time                     `json:"updated_at"`
}

// InterventionResponse represents a participant's response in the intervention flow.
type InterventionResponse struct {
	ID            string    `json:"id"`
	ParticipantID string    `json:"participant_id"`
	State         string    `json:"state"`         // Which state they were in when responding
	ResponseText  string    `json:"response_text"` // The actual response
	ResponseType  string    `json:"response_type"` // e.g., "commitment", "feeling", "completion"
	Timestamp     time.Time `json:"timestamp"`
}

// InterventionEnrollmentRequest represents the payload for enrolling a participant.
type InterventionEnrollmentRequest struct {
	PhoneNumber     string `json:"phone_number" validate:"required"`
	Name            string `json:"name,omitempty"`
	Timezone        string `json:"timezone,omitempty"`
	DailyPromptTime string `json:"daily_prompt_time,omitempty"` // defaults to "10:00"
}

// InterventionResponseRequest represents the payload for processing a participant response.
type InterventionResponseRequest struct {
	ResponseText string `json:"response_text" validate:"required"`
	Context      string `json:"context,omitempty"` // Optional context about how the response was received
}

// InterventionStateAdvanceRequest represents the payload for manually advancing participant state.
type InterventionStateAdvanceRequest struct {
	ToState string `json:"to_state" validate:"required"`
	Reason  string `json:"reason,omitempty"` // Optional reason for manual advancement
}

// InterventionParticipantUpdate represents the payload for updating a participant.
type InterventionParticipantUpdate struct {
	Name            *string                        `json:"name,omitempty"`
	Timezone        *string                        `json:"timezone,omitempty"`
	DailyPromptTime *string                        `json:"daily_prompt_time,omitempty"`
	Status          *InterventionParticipantStatus `json:"status,omitempty"`
}

// Validate validates an InterventionParticipantUpdate request.
func (u *InterventionParticipantUpdate) Validate() error {
	// Validate timezone if provided
	if u.Timezone != nil && *u.Timezone != "" {
		if _, err := time.LoadLocation(*u.Timezone); err != nil {
			return errors.New("invalid timezone")
		}
	}

	// Validate daily prompt time format if provided
	if u.DailyPromptTime != nil && *u.DailyPromptTime != "" {
		if _, err := time.Parse("15:04", *u.DailyPromptTime); err != nil {
			return errors.New("daily_prompt_time must be in HH:MM format")
		}
	}

	// Validate status if provided
	if u.Status != nil {
		if !IsValidParticipantStatus(*u.Status) {
			return errors.New("invalid participant status")
		}
	}

	return nil
}

// InterventionStats represents statistics about the intervention.
type InterventionStats struct {
	TotalParticipants    int                                   `json:"total_participants"`
	ParticipantsByStatus map[InterventionParticipantStatus]int `json:"participants_by_status"`
	ParticipantsByState  map[string]int                        `json:"participants_by_state"`
	TotalResponses       int                                   `json:"total_responses"`
	ResponsesByType      map[string]int                        `json:"responses_by_type"`
	CompletionRate       float64                               `json:"completion_rate"`
	AverageResponseTime  float64                               `json:"average_response_time_minutes"`
}

// Validate validates an InterventionEnrollmentRequest.
func (r *InterventionEnrollmentRequest) Validate() error {
	if r.PhoneNumber == "" {
		return errors.New("phone_number is required")
	}

	// Validate timezone if provided
	if r.Timezone != "" {
		if _, err := time.LoadLocation(r.Timezone); err != nil {
			return errors.New("invalid timezone")
		}
	}

	// Validate daily prompt time format if provided
	if r.DailyPromptTime != "" {
		if _, err := time.Parse("15:04", r.DailyPromptTime); err != nil {
			return errors.New("daily_prompt_time must be in HH:MM format")
		}
	}

	return nil
}

// IsValidParticipantStatus checks if the given participant status is valid.
func IsValidParticipantStatus(status InterventionParticipantStatus) bool {
	switch status {
	case ParticipantStatusActive, ParticipantStatusPaused, ParticipantStatusCompleted, ParticipantStatusWithdrawn:
		return true
	default:
		return false
	}
}

// ConversationParticipantStatus represents the enrollment status of a conversation participant.
type ConversationParticipantStatus string

const (
	// ConversationStatusActive indicates the participant is actively engaged in conversations.
	ConversationStatusActive ConversationParticipantStatus = "active"
	// ConversationStatusPaused indicates the participant has temporarily paused conversations.
	ConversationStatusPaused ConversationParticipantStatus = "paused"
	// ConversationStatusCompleted indicates the participant has completed their conversation session.
	ConversationStatusCompleted ConversationParticipantStatus = "completed"
	// ConversationStatusWithdrawn indicates the participant has withdrawn from conversations.
	ConversationStatusWithdrawn ConversationParticipantStatus = "withdrawn"
)

// ConversationParticipant represents a participant in the conversation flow.
type ConversationParticipant struct {
	ID          string                        `json:"id"`
	PhoneNumber string                        `json:"phone_number"`
	Name        string                        `json:"name,omitempty"`
	Gender      string                        `json:"gender,omitempty"`     // Optional demographic info
	Ethnicity   string                        `json:"ethnicity,omitempty"`  // Optional demographic info
	Background  string                        `json:"background,omitempty"` // Optional cultural/mental health background
	Timezone    string                        `json:"timezone,omitempty"`   // e.g., "America/New_York"
	Status      ConversationParticipantStatus `json:"status"`
	EnrolledAt  time.Time                     `json:"enrolled_at"`
	CreatedAt   time.Time                     `json:"created_at"`
	UpdatedAt   time.Time                     `json:"updated_at"`
}

// ConversationEnrollmentRequest represents the payload for enrolling a conversation participant.
type ConversationEnrollmentRequest struct {
	PhoneNumber string `json:"phone_number" validate:"required"`
	Name        string `json:"name,omitempty"`
	Gender      string `json:"gender,omitempty"`     // Optional: e.g., "male", "female", "non-binary", "prefer not to say"
	Ethnicity   string `json:"ethnicity,omitempty"`  // Optional: user's ethnic background
	Background  string `json:"background,omitempty"` // Optional: cultural/mental health background for context
	Timezone    string `json:"timezone,omitempty"`
}

// ConversationParticipantUpdate represents the payload for updating a conversation participant.
type ConversationParticipantUpdate struct {
	Name       *string                        `json:"name,omitempty"`
	Gender     *string                        `json:"gender,omitempty"`
	Ethnicity  *string                        `json:"ethnicity,omitempty"`
	Background *string                        `json:"background,omitempty"`
	Timezone   *string                        `json:"timezone,omitempty"`
	Status     *ConversationParticipantStatus `json:"status,omitempty"`
}

// Validate validates a ConversationEnrollmentRequest.
func (r *ConversationEnrollmentRequest) Validate() error {
	if r.PhoneNumber == "" {
		return errors.New("phone_number is required")
	}

	// Validate timezone if provided
	if r.Timezone != "" {
		if _, err := time.LoadLocation(r.Timezone); err != nil {
			return errors.New("invalid timezone")
		}
	}

	return nil
}

// Validate validates a ConversationParticipantUpdate request.
func (u *ConversationParticipantUpdate) Validate() error {
	// Validate timezone if provided
	if u.Timezone != nil && *u.Timezone != "" {
		if _, err := time.LoadLocation(*u.Timezone); err != nil {
			return errors.New("invalid timezone")
		}
	}

	// Validate status if provided
	if u.Status != nil && !IsValidConversationParticipantStatus(*u.Status) {
		return errors.New("invalid participant status")
	}

	return nil
}

// IsValidConversationParticipantStatus checks if the given conversation participant status is valid.
func IsValidConversationParticipantStatus(status ConversationParticipantStatus) bool {
	switch status {
	case ConversationStatusActive, ConversationStatusPaused, ConversationStatusCompleted, ConversationStatusWithdrawn:
		return true
	default:
		return false
	}
}

// TimerInfo represents information about a scheduled timer
type TimerInfo struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"` // "once", "recurring"
	ScheduledAt time.Time `json:"scheduled_at"`
	ExpiresAt   time.Time `json:"expires_at,omitempty"` // For one-time timers
	Schedule    *Schedule `json:"schedule,omitempty"`   // For recurring timers
	NextRun     time.Time `json:"next_run,omitempty"`   // For recurring timers
	Remaining   string    `json:"remaining"`
	Description string    `json:"description,omitempty"`
}

// Timer defines the interface for scheduling delayed actions.
type Timer interface {
	// ScheduleAfter schedules a function to run after a delay and returns a timer ID
	ScheduleAfter(delay time.Duration, fn func()) (string, error)

	// ScheduleAt schedules a function to run at a specific time and returns a timer ID
	ScheduleAt(when time.Time, fn func()) (string, error)

	// ScheduleWithSchedule schedules a function to run according to a Schedule and returns a timer ID
	ScheduleWithSchedule(schedule *Schedule, fn func()) (string, error)

	// Cancel cancels a scheduled function by ID
	Cancel(id string) error

	// Stop cancels all scheduled timers
	Stop()

	// ListActive returns information about all active timers
	ListActive() []TimerInfo

	// GetTimer returns information about a specific timer by ID
	GetTimer(id string) (*TimerInfo, error)
}

// Schedule defines when and how often something should be scheduled.
// This replaces cron expressions with a more type-safe and clear structure.
// Fields correspond to cron fields: minute hour day month weekday
// nil values mean "any" (equivalent to * in cron)
type Schedule struct {
	Minute   *int   `json:"minute,omitempty"`   // 0-59, nil means any minute
	Hour     *int   `json:"hour,omitempty"`     // 0-23, nil means any hour
	Day      *int   `json:"day,omitempty"`      // 1-31, nil means any day of month
	Month    *int   `json:"month,omitempty"`    // 1-12, nil means any month
	Weekday  *int   `json:"weekday,omitempty"`  // 0-6 (Sunday=0), nil means any weekday
	Timezone string `json:"timezone,omitempty"` // IANA timezone (e.g., "America/Toronto")
}

// Validate ensures the schedule has valid field values.
func (s *Schedule) Validate() error {
	if s.Minute != nil && (*s.Minute < 0 || *s.Minute > 59) {
		return errors.New("minute must be between 0 and 59")
	}
	if s.Hour != nil && (*s.Hour < 0 || *s.Hour > 23) {
		return errors.New("hour must be between 0 and 23")
	}
	if s.Day != nil && (*s.Day < 1 || *s.Day > 31) {
		return errors.New("day must be between 1 and 31")
	}
	if s.Month != nil && (*s.Month < 1 || *s.Month > 12) {
		return errors.New("month must be between 1 and 12")
	}
	if s.Weekday != nil && (*s.Weekday < 0 || *s.Weekday > 6) {
		return errors.New("weekday must be between 0 and 6 (Sunday=0)")
	}
	if s.Timezone != "" {
		if _, err := time.LoadLocation(s.Timezone); err != nil {
			return fmt.Errorf("invalid timezone: %w", err)
		}
	}
	return nil
}

// String returns a human-readable description of the schedule.
func (s *Schedule) String() string {
	parts := []string{}

	if s.Minute != nil {
		parts = append(parts, fmt.Sprintf("minute=%d", *s.Minute))
	}
	if s.Hour != nil {
		parts = append(parts, fmt.Sprintf("hour=%d", *s.Hour))
	}
	if s.Day != nil {
		parts = append(parts, fmt.Sprintf("day=%d", *s.Day))
	}
	if s.Month != nil {
		parts = append(parts, fmt.Sprintf("month=%d", *s.Month))
	}
	if s.Weekday != nil {
		weekdays := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
		parts = append(parts, fmt.Sprintf("weekday=%s", weekdays[*s.Weekday]))
	}

	if len(parts) == 0 {
		return "every minute"
	}

	return fmt.Sprintf("schedule(%s)", fmt.Sprintf("%s", fmt.Sprintf("%v", parts)))
}

// ToCronString converts the schedule to a cron-like string (minute hour day month weekday).
func (s *Schedule) ToCronString() string {
	minute := "*"
	if s.Minute != nil {
		minute = fmt.Sprintf("%d", *s.Minute)
	}

	hour := "*"
	if s.Hour != nil {
		hour = fmt.Sprintf("%d", *s.Hour)
	}

	day := "*"
	if s.Day != nil {
		day = fmt.Sprintf("%d", *s.Day)
	}

	month := "*"
	if s.Month != nil {
		month = fmt.Sprintf("%d", *s.Month)
	}

	weekday := "*"
	if s.Weekday != nil {
		weekday = fmt.Sprintf("%d", *s.Weekday)
	}

	return fmt.Sprintf("%s %s %s %s %s", minute, hour, day, month, weekday)
}

// Helper functions for creating common schedules

// NewDailySchedule creates a schedule that runs daily at the specified time.
func NewDailySchedule(hour, minute int) *Schedule {
	return &Schedule{
		Hour:   &hour,
		Minute: &minute,
	}
}

// NewWeeklySchedule creates a schedule that runs weekly on the specified weekday and time.
func NewWeeklySchedule(weekday, hour, minute int) *Schedule {
	return &Schedule{
		Weekday: &weekday,
		Hour:    &hour,
		Minute:  &minute,
	}
}

// NewMonthlySchedule creates a schedule that runs monthly on the specified day and time.
func NewMonthlySchedule(day, hour, minute int) *Schedule {
	return &Schedule{
		Day:    &day,
		Hour:   &hour,
		Minute: &minute,
	}
}

// NewHourlySchedule creates a schedule that runs every hour at the specified minute.
func NewHourlySchedule(minute int) *Schedule {
	return &Schedule{
		Minute: &minute,
	}
}
