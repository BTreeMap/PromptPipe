// Package store provides storage backends for PromptPipe.
//
// This file implements a PostgreSQL-backed store for receipts.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	_ "embed"

	"github.com/BTreeMap/PromptPipe/internal/models"
	_ "github.com/lib/pq"
)

// Database connection pool configuration constants
const (
	// DefaultMaxOpenConns is the default maximum number of open connections to the database
	DefaultMaxOpenConns = 25
	// DefaultMaxIdleConns is the default maximum number of idle connections in the pool
	DefaultMaxIdleConns = 25
	// DefaultConnMaxLifetime is the default maximum amount of time a connection may be reused
	DefaultConnMaxLifetime = 5 * time.Minute
)

//go:embed migrations_postgres.sql
var postgresMigrations string

type PostgresStore struct {
	db *sql.DB
}

// NewPostgresStore creates a new Postgres store based on provided options.
func NewPostgresStore(opts ...Option) (*PostgresStore, error) {
	// Apply options
	var cfg Opts
	for _, opt := range opts {
		opt(&cfg)
	}
	slog.Debug("PostgresStore.NewPostgresStore: creating Postgres store", "DSN_set", cfg.DSN != "")
	// Determine DSN (required)
	dsn := cfg.DSN
	if dsn == "" {
		slog.Error("PostgresStore DSN not set")
		return nil, fmt.Errorf("database DSN not set")
	}

	slog.Debug("Opening Postgres database connection")
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		slog.Error("Failed to open Postgres connection", "error", err)
		return nil, err
	}
	slog.Debug("Postgres database opened")

	// Configure connection pool for better performance
	db.SetMaxOpenConns(DefaultMaxOpenConns)
	db.SetMaxIdleConns(DefaultMaxIdleConns)
	db.SetConnMaxLifetime(DefaultConnMaxLifetime)

	if err := db.Ping(); err != nil {
		slog.Error("Postgres ping failed", "error", err)
		return nil, err
	}
	// Run migrations to ensure receipts table exists
	slog.Debug("Running Postgres migrations", "dsn", dsn)
	if _, err := db.Exec(postgresMigrations); err != nil {
		// Check if error is due to duplicate column (expected for ALTER TABLE on existing schema)
		// PostgreSQL 9.6+ supports "IF NOT EXISTS" for ALTER TABLE, but handle errors for older versions
		errMsg := err.Error()
		if strings.Contains(errMsg, "already exists") || strings.Contains(errMsg, "duplicate column") {
			slog.Debug("Postgres migration produced expected duplicate column warning (schema already up-to-date)", "error", err)
			// Continue - this is expected when ALTER TABLE runs on updated schema
		} else {
			slog.Error("Failed to run migrations", "dsn", dsn, "error", err)
			return nil, fmt.Errorf("failed to run migrations: %w", err)
		}
	}
	slog.Debug("Postgres migrations applied successfully", "dsn", dsn)
	slog.Debug("Postgres migrations applied successfully", "dsn", dsn)
	return &PostgresStore{db: db}, nil
}

func (s *PostgresStore) AddReceipt(r models.Receipt) error {
	_, err := s.db.Exec(`INSERT INTO receipts (recipient, status, time) VALUES ($1, $2, $3)`, r.To, r.Status, r.Time)
	if err != nil {
		slog.Error("PostgresStore AddReceipt failed", "error", err, "to", r.To)
		return fmt.Errorf("failed to insert receipt for %s: %w", r.To, err)
	}
	slog.Debug("PostgresStore AddReceipt succeeded", "to", r.To, "status", r.Status)
	return nil
}

func (s *PostgresStore) GetReceipts() ([]models.Receipt, error) {
	rows, err := s.db.Query(`SELECT recipient, status, time FROM receipts`)
	if err != nil {
		slog.Error("PostgresStore GetReceipts query failed", "error", err)
		return nil, fmt.Errorf("failed to query receipts: %w", err)
	}
	defer rows.Close()
	var receipts []models.Receipt
	for rows.Next() {
		var r models.Receipt
		if err := rows.Scan(&r.To, &r.Status, &r.Time); err != nil {
			slog.Error("PostgresStore GetReceipts scan failed", "error", err)
			return nil, fmt.Errorf("failed to scan receipt row: %w", err)
		}
		receipts = append(receipts, r)
	}
	if err := rows.Err(); err != nil {
		slog.Error("PostgresStore GetReceipts rows iteration failed", "error", err)
		return nil, fmt.Errorf("failed to iterate receipt rows: %w", err)
	}
	slog.Debug("PostgresStore GetReceipts succeeded", "count", len(receipts))
	return receipts, nil
}

// AddResponse stores an incoming response in Postgres.
func (s *PostgresStore) AddResponse(r models.Response) error {
	_, err := s.db.Exec(`INSERT INTO responses (sender, body, time) VALUES ($1, $2, $3)`, r.From, r.Body, r.Time)
	if err != nil {
		slog.Error("PostgresStore AddResponse failed", "error", err, "from", r.From)
		return fmt.Errorf("failed to insert response from %s: %w", r.From, err)
	}
	slog.Debug("PostgresStore AddResponse succeeded", "from", r.From)
	return nil
}

// GetResponses retrieves all stored responses from Postgres.
func (s *PostgresStore) GetResponses() ([]models.Response, error) {
	rows, err := s.db.Query(`SELECT sender, body, time FROM responses`)
	if err != nil {
		slog.Error("PostgresStore GetResponses query failed", "error", err)
		return nil, fmt.Errorf("failed to query responses: %w", err)
	}
	defer rows.Close()
	var responses []models.Response
	for rows.Next() {
		var r models.Response
		if err := rows.Scan(&r.From, &r.Body, &r.Time); err != nil {
			slog.Error("PostgresStore GetResponses scan failed", "error", err)
			return nil, fmt.Errorf("failed to scan response row: %w", err)
		}
		responses = append(responses, r)
	}
	if err := rows.Err(); err != nil {
		slog.Error("PostgresStore GetResponses rows iteration failed", "error", err)
		return nil, fmt.Errorf("failed to iterate response rows: %w", err)
	}
	slog.Debug("PostgresStore GetResponses succeeded", "count", len(responses))
	return responses, nil
}

// ClearReceipts deletes all records in receipts table (for tests).
func (s *PostgresStore) ClearReceipts() error {
	_, err := s.db.Exec("DELETE FROM receipts")
	if err != nil {
		slog.Error("PostgresStore ClearReceipts failed", "error", err)
		return err
	}
	slog.Debug("PostgresStore ClearReceipts succeeded")
	return nil
}

// ClearResponses deletes all records in responses table (for tests).
func (s *PostgresStore) ClearResponses() error {
	_, err := s.db.Exec("DELETE FROM responses")
	if err != nil {
		slog.Error("PostgresStore ClearResponses failed", "error", err)
		return err
	}
	slog.Debug("PostgresStore ClearResponses succeeded")
	return nil
}

// Close closes the PostgreSQL database connection.
func (s *PostgresStore) Close() error {
	slog.Debug("Closing PostgreSQL database connection")
	err := s.db.Close()
	if err != nil {
		slog.Error("Failed to close PostgreSQL database", "error", err)
	} else {
		slog.Debug("PostgreSQL database connection closed successfully")
	}
	return err
}

// SaveFlowState stores or updates flow state for a participant.
func (s *PostgresStore) SaveFlowState(state models.FlowState) error {
	query := `
		INSERT INTO flow_states (participant_id, flow_type, current_state, state_data, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (participant_id, flow_type)
		DO UPDATE SET 
			current_state = EXCLUDED.current_state,
			state_data = EXCLUDED.state_data,
			updated_at = EXCLUDED.updated_at`

	// Convert state_data map to JSON bytes
	var stateDataJSON []byte
	var err error
	if len(state.StateData) > 0 {
		stateDataJSON, err = json.Marshal(state.StateData)
		if err != nil {
			slog.Error("PostgresStore SaveFlowState JSON marshal failed", "error", err, "participantID", state.ParticipantID)
			return err
		}
	}

	_, err = s.db.Exec(query, state.ParticipantID, state.FlowType, state.CurrentState,
		stateDataJSON, state.CreatedAt, state.UpdatedAt)
	if err != nil {
		slog.Error("PostgresStore SaveFlowState failed", "error", err, "participantID", state.ParticipantID, "flowType", state.FlowType)
		return err
	}
	slog.Debug("PostgresStore SaveFlowState succeeded", "participantID", state.ParticipantID, "flowType", state.FlowType, "state", state.CurrentState)
	return nil
}

// GetFlowState retrieves flow state for a participant.
func (s *PostgresStore) GetFlowState(participantID, flowType string) (*models.FlowState, error) {
	query := `SELECT participant_id, flow_type, current_state, state_data, created_at, updated_at 
			  FROM flow_states WHERE participant_id = $1 AND flow_type = $2`

	var state models.FlowState
	var stateDataJSON []byte

	err := s.db.QueryRow(query, participantID, flowType).Scan(
		&state.ParticipantID, &state.FlowType, &state.CurrentState,
		&stateDataJSON, &state.CreatedAt, &state.UpdatedAt)

	if err == sql.ErrNoRows {
		slog.Debug("PostgresStore GetFlowState not found", "participantID", participantID, "flowType", flowType)
		return nil, nil
	}
	if err != nil {
		slog.Error("PostgresStore GetFlowState failed", "error", err, "participantID", participantID, "flowType", flowType)
		return nil, err
	}

	// Convert JSON back to map[string]string
	if len(stateDataJSON) > 0 {
		state.StateData = make(map[models.DataKey]string)
		if err := json.Unmarshal(stateDataJSON, &state.StateData); err != nil {
			slog.Error("PostgresStore GetFlowState JSON unmarshal failed", "error", err, "participantID", participantID)
			// Continue with empty map rather than failing
			state.StateData = make(map[models.DataKey]string)
		}
	}

	slog.Debug("PostgresStore GetFlowState found", "participantID", participantID, "flowType", flowType, "state", state.CurrentState)
	return &state, nil
}

// DeleteFlowState removes flow state for a participant.
func (s *PostgresStore) DeleteFlowState(participantID, flowType string) error {
	query := `DELETE FROM flow_states WHERE participant_id = $1 AND flow_type = $2`

	_, err := s.db.Exec(query, participantID, flowType)
	if err != nil {
		slog.Error("PostgresStore DeleteFlowState failed", "error", err, "participantID", participantID, "flowType", flowType)
		return err
	}
	slog.Debug("PostgresStore DeleteFlowState succeeded", "participantID", participantID, "flowType", flowType)
	return nil
}

// Conversation participant management methods - PostgreSQL implementation

// SaveConversationParticipant stores or updates a conversation participant.
func (s *PostgresStore) SaveConversationParticipant(participant models.ConversationParticipant) error {
	query := `
		INSERT INTO conversation_participants (id, phone_number, name, gender, ethnicity, background, timezone, status, enrolled_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (id) DO UPDATE SET
			phone_number = EXCLUDED.phone_number,
			name = EXCLUDED.name,
			gender = EXCLUDED.gender,
			ethnicity = EXCLUDED.ethnicity,
			background = EXCLUDED.background,
			timezone = EXCLUDED.timezone,
			status = EXCLUDED.status,
			enrolled_at = EXCLUDED.enrolled_at,
			updated_at = EXCLUDED.updated_at`

	_, err := s.db.Exec(query, participant.ID, participant.PhoneNumber, participant.Name, participant.Gender,
		participant.Ethnicity, participant.Background, participant.Timezone, string(participant.Status), participant.EnrolledAt,
		participant.CreatedAt, participant.UpdatedAt)
	if err != nil {
		slog.Error("PostgresStore SaveConversationParticipant failed", "error", err, "id", participant.ID)
		return err
	}
	slog.Debug("PostgresStore SaveConversationParticipant succeeded", "id", participant.ID, "phone", participant.PhoneNumber)
	return nil
}

// GetConversationParticipant retrieves a conversation participant by ID.
func (s *PostgresStore) GetConversationParticipant(id string) (*models.ConversationParticipant, error) {
	query := `SELECT id, phone_number, name, gender, ethnicity, background, timezone, status, enrolled_at, created_at, updated_at 
			  FROM conversation_participants WHERE id = $1`

	var participant models.ConversationParticipant
	var status string

	err := s.db.QueryRow(query, id).Scan(
		&participant.ID, &participant.PhoneNumber, &participant.Name, &participant.Gender,
		&participant.Ethnicity, &participant.Background, &participant.Timezone, &status, &participant.EnrolledAt,
		&participant.CreatedAt, &participant.UpdatedAt)

	if err == sql.ErrNoRows {
		slog.Debug("PostgresStore GetConversationParticipant not found", "id", id)
		return nil, nil
	}
	if err != nil {
		slog.Error("PostgresStore GetConversationParticipant failed", "error", err, "id", id)
		return nil, err
	}

	participant.Status = models.ConversationParticipantStatus(status)
	slog.Debug("PostgresStore GetConversationParticipant found", "id", id)
	return &participant, nil
}

// GetConversationParticipantByPhone retrieves a conversation participant by phone number.
func (s *PostgresStore) GetConversationParticipantByPhone(phoneNumber string) (*models.ConversationParticipant, error) {
	query := `SELECT id, phone_number, name, gender, ethnicity, background, timezone, status, enrolled_at, created_at, updated_at 
			  FROM conversation_participants WHERE phone_number = $1`

	var participant models.ConversationParticipant
	var status string

	err := s.db.QueryRow(query, phoneNumber).Scan(
		&participant.ID, &participant.PhoneNumber, &participant.Name, &participant.Gender,
		&participant.Ethnicity, &participant.Background, &participant.Timezone, &status, &participant.EnrolledAt,
		&participant.CreatedAt, &participant.UpdatedAt)

	if err == sql.ErrNoRows {
		slog.Debug("PostgresStore GetConversationParticipantByPhone not found", "phone", phoneNumber)
		return nil, nil
	}
	if err != nil {
		slog.Error("PostgresStore GetConversationParticipantByPhone failed", "error", err, "phone", phoneNumber)
		return nil, err
	}

	participant.Status = models.ConversationParticipantStatus(status)
	slog.Debug("PostgresStore GetConversationParticipantByPhone found", "phone", phoneNumber, "id", participant.ID)
	return &participant, nil
}

// ListConversationParticipants retrieves all conversation participants.
func (s *PostgresStore) ListConversationParticipants() ([]models.ConversationParticipant, error) {
	query := `SELECT id, phone_number, name, gender, ethnicity, background, timezone, status, enrolled_at, created_at, updated_at 
			  FROM conversation_participants ORDER BY created_at DESC`

	rows, err := s.db.Query(query)
	if err != nil {
		slog.Error("PostgresStore ListConversationParticipants failed", "error", err)
		return nil, err
	}
	defer rows.Close()

	var participants []models.ConversationParticipant
	for rows.Next() {
		var participant models.ConversationParticipant
		var status string

		err := rows.Scan(
			&participant.ID, &participant.PhoneNumber, &participant.Name, &participant.Gender,
			&participant.Ethnicity, &participant.Background, &participant.Timezone, &status, &participant.EnrolledAt,
			&participant.CreatedAt, &participant.UpdatedAt)
		if err != nil {
			slog.Error("PostgresStore ListConversationParticipants scan failed", "error", err)
			return nil, err
		}

		participant.Status = models.ConversationParticipantStatus(status)
		participants = append(participants, participant)
	}

	if err := rows.Err(); err != nil {
		slog.Error("PostgresStore ListConversationParticipants rows error", "error", err)
		return nil, err
	}

	slog.Debug("PostgresStore ListConversationParticipants succeeded", "count", len(participants))
	return participants, nil
}

// DeleteConversationParticipant removes a conversation participant.
func (s *PostgresStore) DeleteConversationParticipant(id string) error {
	query := `DELETE FROM conversation_participants WHERE id = $1`

	_, err := s.db.Exec(query, id)
	if err != nil {
		slog.Error("PostgresStore DeleteConversationParticipant failed", "error", err, "id", id)
		return err
	}
	slog.Debug("PostgresStore DeleteConversationParticipant succeeded", "id", id)
	return nil
}

// Hook persistence management methods - PostgreSQL implementation

// SaveRegisteredHook stores or updates a registered hook.
func (s *PostgresStore) SaveRegisteredHook(hook models.RegisteredHook) error {
	if err := hook.Validate(); err != nil {
		return fmt.Errorf("invalid hook: %w", err)
	}

	// Serialize parameters to JSON
	paramsJSON, err := json.Marshal(hook.Parameters)
	if err != nil {
		return fmt.Errorf("failed to serialize hook parameters: %w", err)
	}

	query := `
		INSERT INTO registered_hooks 
		(phone_number, hook_type, parameters, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (phone_number) 
		DO UPDATE SET 
			hook_type = EXCLUDED.hook_type,
			parameters = EXCLUDED.parameters,
			updated_at = EXCLUDED.updated_at
	`

	_, err = s.db.Exec(query, hook.PhoneNumber, hook.HookType, string(paramsJSON), hook.CreatedAt, hook.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to save registered hook: %w", err)
	}

	slog.Debug("PostgresStore registered hook saved", "phoneNumber", hook.PhoneNumber, "hookType", hook.HookType)
	return nil
}

// GetRegisteredHook retrieves a registered hook by phone number
func (s *PostgresStore) GetRegisteredHook(phoneNumber string) (*models.RegisteredHook, error) {
	query := `
		SELECT phone_number, hook_type, parameters, created_at, updated_at 
		FROM registered_hooks 
		WHERE phone_number = $1
	`

	var hook models.RegisteredHook
	var paramsJSON string

	err := s.db.QueryRow(query, phoneNumber).Scan(
		&hook.PhoneNumber,
		&hook.HookType,
		&paramsJSON,
		&hook.CreatedAt,
		&hook.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Not found
		}
		return nil, fmt.Errorf("failed to get registered hook: %w", err)
	}

	// Deserialize parameters from JSON
	if err := json.Unmarshal([]byte(paramsJSON), &hook.Parameters); err != nil {
		return nil, fmt.Errorf("failed to deserialize hook parameters: %w", err)
	}

	return &hook, nil
}

// ListRegisteredHooks retrieves all registered hooks
func (s *PostgresStore) ListRegisteredHooks() ([]models.RegisteredHook, error) {
	query := `
		SELECT phone_number, hook_type, parameters, created_at, updated_at 
		FROM registered_hooks 
		ORDER BY created_at DESC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list registered hooks: %w", err)
	}
	defer rows.Close()

	var hooks []models.RegisteredHook
	for rows.Next() {
		var hook models.RegisteredHook
		var paramsJSON string

		err := rows.Scan(
			&hook.PhoneNumber,
			&hook.HookType,
			&paramsJSON,
			&hook.CreatedAt,
			&hook.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan registered hook: %w", err)
		}

		// Deserialize parameters from JSON
		if err := json.Unmarshal([]byte(paramsJSON), &hook.Parameters); err != nil {
			return nil, fmt.Errorf("failed to deserialize hook parameters: %w", err)
		}

		hooks = append(hooks, hook)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading registered hooks: %w", err)
	}

	return hooks, nil
}

// DeleteRegisteredHook deletes a registered hook by phone number
func (s *PostgresStore) DeleteRegisteredHook(phoneNumber string) error {
	query := `DELETE FROM registered_hooks WHERE phone_number = $1`

	result, err := s.db.Exec(query, phoneNumber)
	if err != nil {
		return fmt.Errorf("failed to delete registered hook: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		slog.Debug("PostgresStore registered hook not found for deletion", "phoneNumber", phoneNumber)
	} else {
		slog.Debug("PostgresStore registered hook deleted", "phoneNumber", phoneNumber)
	}

	return nil
}

// SaveTimer stores or updates a timer record.
func (s *PostgresStore) SaveTimer(timer models.TimerRecord) error {
	// Convert callback params map to JSON
	paramsJSON, err := json.Marshal(timer.CallbackParams)
	if err != nil {
		return fmt.Errorf("failed to marshal callback params: %w", err)
	}

	query := `
		INSERT INTO active_timers 
		(id, participant_id, flow_type, timer_type, state_type, data_key, callback_type, callback_params,
		 scheduled_at, expires_at, original_delay_seconds, schedule_json, next_run, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		ON CONFLICT (id) DO UPDATE SET
			participant_id = EXCLUDED.participant_id,
			flow_type = EXCLUDED.flow_type,
			timer_type = EXCLUDED.timer_type,
			state_type = EXCLUDED.state_type,
			data_key = EXCLUDED.data_key,
			callback_type = EXCLUDED.callback_type,
			callback_params = EXCLUDED.callback_params,
			scheduled_at = EXCLUDED.scheduled_at,
			expires_at = EXCLUDED.expires_at,
			original_delay_seconds = EXCLUDED.original_delay_seconds,
			schedule_json = EXCLUDED.schedule_json,
			next_run = EXCLUDED.next_run,
			updated_at = EXCLUDED.updated_at`

	// Handle nullable state_type and data_key
	var stateType, dataKey interface{}
	if timer.StateType != "" {
		stateType = timer.StateType
	}
	if timer.DataKey != "" {
		dataKey = timer.DataKey
	}

	// Handle nullable schedule_json
	var scheduleJSON interface{}
	if timer.ScheduleJSON != "" {
		scheduleJSON = timer.ScheduleJSON
	}

	_, err = s.db.Exec(query,
		timer.ID, timer.ParticipantID, timer.FlowType, timer.TimerType, stateType, dataKey,
		timer.CallbackType, string(paramsJSON), timer.ScheduledAt, timer.ExpiresAt, timer.OriginalDelaySeconds,
		scheduleJSON, timer.NextRun, timer.CreatedAt, timer.UpdatedAt)
	if err != nil {
		slog.Error("PostgresStore SaveTimer failed", "error", err, "id", timer.ID)
		return fmt.Errorf("failed to save timer: %w", err)
	}
	slog.Debug("PostgresStore SaveTimer succeeded", "id", timer.ID, "participantID", timer.ParticipantID)
	return nil
}

// GetTimer retrieves a timer record by ID.
func (s *PostgresStore) GetTimer(id string) (*models.TimerRecord, error) {
	query := `
		SELECT id, participant_id, flow_type, timer_type, state_type, data_key, callback_type, callback_params,
		       scheduled_at, expires_at, original_delay_seconds, schedule_json, next_run, created_at, updated_at
		FROM active_timers WHERE id = $1`

	var timer models.TimerRecord
	var paramsJSON string
	var scheduleJSON sql.NullString
	var stateType, dataKey sql.NullString

	err := s.db.QueryRow(query, id).Scan(
		&timer.ID, &timer.ParticipantID, &timer.FlowType, &timer.TimerType, &stateType, &dataKey,
		&timer.CallbackType, &paramsJSON, &timer.ScheduledAt, &timer.ExpiresAt, &timer.OriginalDelaySeconds,
		&scheduleJSON, &timer.NextRun, &timer.CreatedAt, &timer.UpdatedAt)

	if err == sql.ErrNoRows {
		slog.Debug("PostgresStore GetTimer not found", "id", id)
		return nil, nil
	}
	if err != nil {
		slog.Error("PostgresStore GetTimer failed", "error", err, "id", id)
		return nil, fmt.Errorf("failed to get timer: %w", err)
	}

	// Handle nullable fields
	if stateType.Valid {
		timer.StateType = models.StateType(stateType.String)
	}
	if dataKey.Valid {
		timer.DataKey = models.DataKey(dataKey.String)
	}
	if scheduleJSON.Valid {
		timer.ScheduleJSON = scheduleJSON.String
	}

	// Parse callback params JSON
	if paramsJSON != "" {
		timer.CallbackParams = make(map[string]string)
		if err := json.Unmarshal([]byte(paramsJSON), &timer.CallbackParams); err != nil {
			slog.Warn("PostgresStore GetTimer: failed to unmarshal callback params", "error", err, "id", id)
		}
	}

	slog.Debug("PostgresStore GetTimer found", "id", id)
	return &timer, nil
}

// ListTimers retrieves all timer records.
func (s *PostgresStore) ListTimers() ([]models.TimerRecord, error) {
	query := `
		SELECT id, participant_id, flow_type, timer_type, state_type, data_key, callback_type, callback_params,
		       scheduled_at, expires_at, original_delay_seconds, schedule_json, next_run, created_at, updated_at
		FROM active_timers ORDER BY created_at DESC`

	rows, err := s.db.Query(query)
	if err != nil {
		slog.Error("PostgresStore ListTimers failed", "error", err)
		return nil, fmt.Errorf("failed to list timers: %w", err)
	}
	defer rows.Close()

	var timers []models.TimerRecord
	for rows.Next() {
		var timer models.TimerRecord
		var paramsJSON string
		var scheduleJSON sql.NullString
		var stateType, dataKey sql.NullString

		err := rows.Scan(
			&timer.ID, &timer.ParticipantID, &timer.FlowType, &timer.TimerType, &stateType, &dataKey,
			&timer.CallbackType, &paramsJSON, &timer.ScheduledAt, &timer.ExpiresAt, &timer.OriginalDelaySeconds,
			&scheduleJSON, &timer.NextRun, &timer.CreatedAt, &timer.UpdatedAt)
		if err != nil {
			slog.Error("PostgresStore ListTimers scan failed", "error", err)
			return nil, fmt.Errorf("failed to scan timer: %w", err)
		}

		// Handle nullable fields
		if stateType.Valid {
			timer.StateType = models.StateType(stateType.String)
		}
		if dataKey.Valid {
			timer.DataKey = models.DataKey(dataKey.String)
		}
		if scheduleJSON.Valid {
			timer.ScheduleJSON = scheduleJSON.String
		}

		// Parse callback params JSON
		if paramsJSON != "" {
			timer.CallbackParams = make(map[string]string)
			if err := json.Unmarshal([]byte(paramsJSON), &timer.CallbackParams); err != nil {
				slog.Warn("PostgresStore ListTimers: failed to unmarshal callback params", "error", err, "id", timer.ID)
			}
		}

		timers = append(timers, timer)
	}

	if err := rows.Err(); err != nil {
		slog.Error("PostgresStore ListTimers rows error", "error", err)
		return nil, fmt.Errorf("error reading timers: %w", err)
	}

	slog.Debug("PostgresStore ListTimers succeeded", "count", len(timers))
	return timers, nil
}

// ListTimersByParticipant retrieves all timer records for a participant.
func (s *PostgresStore) ListTimersByParticipant(participantID string) ([]models.TimerRecord, error) {
	query := `
		SELECT id, participant_id, flow_type, timer_type, state_type, data_key, callback_type, callback_params,
		       scheduled_at, expires_at, original_delay_seconds, schedule_json, next_run, created_at, updated_at
		FROM active_timers WHERE participant_id = $1 ORDER BY created_at DESC`

	rows, err := s.db.Query(query, participantID)
	if err != nil {
		slog.Error("PostgresStore ListTimersByParticipant failed", "error", err, "participantID", participantID)
		return nil, fmt.Errorf("failed to list timers by participant: %w", err)
	}
	defer rows.Close()

	var timers []models.TimerRecord
	for rows.Next() {
		var timer models.TimerRecord
		var paramsJSON string
		var scheduleJSON sql.NullString
		var stateType, dataKey sql.NullString

		err := rows.Scan(
			&timer.ID, &timer.ParticipantID, &timer.FlowType, &timer.TimerType, &stateType, &dataKey,
			&timer.CallbackType, &paramsJSON, &timer.ScheduledAt, &timer.ExpiresAt, &timer.OriginalDelaySeconds,
			&scheduleJSON, &timer.NextRun, &timer.CreatedAt, &timer.UpdatedAt)
		if err != nil {
			slog.Error("PostgresStore ListTimersByParticipant scan failed", "error", err)
			return nil, fmt.Errorf("failed to scan timer: %w", err)
		}

		// Handle nullable fields
		if stateType.Valid {
			timer.StateType = models.StateType(stateType.String)
		}
		if dataKey.Valid {
			timer.DataKey = models.DataKey(dataKey.String)
		}
		if scheduleJSON.Valid {
			timer.ScheduleJSON = scheduleJSON.String
		}

		// Parse callback params JSON
		if paramsJSON != "" {
			timer.CallbackParams = make(map[string]string)
			if err := json.Unmarshal([]byte(paramsJSON), &timer.CallbackParams); err != nil {
				slog.Warn("PostgresStore ListTimersByParticipant: failed to unmarshal callback params", "error", err, "id", timer.ID)
			}
		}

		timers = append(timers, timer)
	}

	if err := rows.Err(); err != nil {
		slog.Error("PostgresStore ListTimersByParticipant rows error", "error", err)
		return nil, fmt.Errorf("error reading timers: %w", err)
	}

	slog.Debug("PostgresStore ListTimersByParticipant succeeded", "participantID", participantID, "count", len(timers))
	return timers, nil
}

// ListTimersByFlowType retrieves all timer records for a flow type.
func (s *PostgresStore) ListTimersByFlowType(flowType string) ([]models.TimerRecord, error) {
	query := `
		SELECT id, participant_id, flow_type, timer_type, state_type, data_key, callback_type, callback_params,
		       scheduled_at, expires_at, original_delay_seconds, schedule_json, next_run, created_at, updated_at
		FROM active_timers WHERE flow_type = $1 ORDER BY created_at DESC`

	rows, err := s.db.Query(query, flowType)
	if err != nil {
		slog.Error("PostgresStore ListTimersByFlowType failed", "error", err, "flowType", flowType)
		return nil, fmt.Errorf("failed to list timers by flow type: %w", err)
	}
	defer rows.Close()

	var timers []models.TimerRecord
	for rows.Next() {
		var timer models.TimerRecord
		var paramsJSON string
		var scheduleJSON sql.NullString
		var stateType, dataKey sql.NullString

		err := rows.Scan(
			&timer.ID, &timer.ParticipantID, &timer.FlowType, &timer.TimerType, &stateType, &dataKey,
			&timer.CallbackType, &paramsJSON, &timer.ScheduledAt, &timer.ExpiresAt, &timer.OriginalDelaySeconds,
			&scheduleJSON, &timer.NextRun, &timer.CreatedAt, &timer.UpdatedAt)
		if err != nil {
			slog.Error("PostgresStore ListTimersByFlowType scan failed", "error", err)
			return nil, fmt.Errorf("failed to scan timer: %w", err)
		}

		// Handle nullable fields
		if stateType.Valid {
			timer.StateType = models.StateType(stateType.String)
		}
		if dataKey.Valid {
			timer.DataKey = models.DataKey(dataKey.String)
		}
		if scheduleJSON.Valid {
			timer.ScheduleJSON = scheduleJSON.String
		}

		// Parse callback params JSON
		if paramsJSON != "" {
			timer.CallbackParams = make(map[string]string)
			if err := json.Unmarshal([]byte(paramsJSON), &timer.CallbackParams); err != nil {
				slog.Warn("PostgresStore ListTimersByFlowType: failed to unmarshal callback params", "error", err, "id", timer.ID)
			}
		}

		timers = append(timers, timer)
	}

	if err := rows.Err(); err != nil {
		slog.Error("PostgresStore ListTimersByFlowType rows error", "error", err)
		return nil, fmt.Errorf("error reading timers: %w", err)
	}

	slog.Debug("PostgresStore ListTimersByFlowType succeeded", "flowType", flowType, "count", len(timers))
	return timers, nil
}

// DeleteTimer removes a timer record.
func (s *PostgresStore) DeleteTimer(id string) error {
	query := `DELETE FROM active_timers WHERE id = $1`

	result, err := s.db.Exec(query, id)
	if err != nil {
		slog.Error("PostgresStore DeleteTimer failed", "error", err, "id", id)
		return fmt.Errorf("failed to delete timer: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		slog.Debug("PostgresStore DeleteTimer: timer not found", "id", id)
	} else {
		slog.Debug("PostgresStore DeleteTimer succeeded", "id", id)
	}

	return nil
}

// DeleteExpiredTimers removes expired one-time timers.
func (s *PostgresStore) DeleteExpiredTimers() error {
	query := `DELETE FROM active_timers WHERE timer_type = 'once' AND expires_at < $1`

	result, err := s.db.Exec(query, time.Now())
	if err != nil {
		slog.Error("PostgresStore DeleteExpiredTimers failed", "error", err)
		return fmt.Errorf("failed to delete expired timers: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected > 0 {
		slog.Debug("PostgresStore DeleteExpiredTimers succeeded", "deleted", rowsAffected)
	}

	return nil
}
