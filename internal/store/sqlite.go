// Package store provides storage backends for PromptPipe.
//
// This file implements an SQLite-backed store for receipts and responses.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	_ "embed"

	"github.com/BTreeMap/PromptPipe/internal/models"
	_ "github.com/mattn/go-sqlite3"
)

// Constants for SQLite store configuration
const (
	// DefaultDirPermissions defines the default permissions for database directories
	DefaultDirPermissions = 0755
	// SQLiteMaxOpenConns is the maximum number of open connections for SQLite (should be 1 for WAL mode safety)
	SQLiteMaxOpenConns = 1
	// SQLiteMaxIdleConns is the maximum number of idle connections for SQLite
	SQLiteMaxIdleConns = 1
	// SQLiteConnMaxLifetime is the maximum amount of time a SQLite connection may be reused
	SQLiteConnMaxLifetime = 30 * time.Minute
)

//go:embed migrations_sqlite.sql
var sqliteMigrations string

type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore creates a new SQLite store with the given DSN.
// The DSN should be a file path to the SQLite database file.
// If the directory doesn't exist, it will be created.
func NewSQLiteStore(opts ...Option) (*SQLiteStore, error) {
	// Apply options
	var cfg Opts
	for _, opt := range opts {
		opt(&cfg)
	}
	slog.Debug("SQLiteStore.NewSQLiteStore: creating SQLite store", "DSN_set", cfg.DSN != "")

	// Determine DSN (required)
	dsn := cfg.DSN
	if dsn == "" {
		slog.Error("SQLiteStore DSN not set")
		return nil, fmt.Errorf("database DSN not set")
	}

	// Ensure the directory exists using unified SQLite DSN directory extraction
	if dir, err := ExtractDirFromSQLiteDSN(dsn); err != nil {
		slog.Error("Failed to extract directory from SQLite DSN", "error", err, "dsn", dsn)
		return nil, fmt.Errorf("failed to extract directory from SQLite DSN: %w", err)
	} else if dir != "" {
		if err := os.MkdirAll(dir, DefaultDirPermissions); err != nil {
			slog.Error("Failed to create database directory", "error", err, "dir", dir)
			return nil, fmt.Errorf("failed to create database directory: %w", err)
		}
		slog.Debug("SQLite database directory verified/created", "dir", dir)
	} else {
		slog.Debug("No directory creation needed for SQLite DSN", "dsn", dsn)
	}

	slog.Debug("Opening SQLite database connection")
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		slog.Error("Failed to open SQLite connection", "error", err)
		return nil, err
	}
	slog.Debug("SQLite database opened")

	// Configure connection pool for SQLite
	// Note: SQLite should use minimal connections to avoid database lock issues
	db.SetMaxOpenConns(SQLiteMaxOpenConns)
	db.SetMaxIdleConns(SQLiteMaxIdleConns)
	db.SetConnMaxLifetime(SQLiteConnMaxLifetime)

	if err := db.Ping(); err != nil {
		slog.Error("SQLite ping failed", "error", err)
		return nil, err
	}
	slog.Debug("SQLite ping successful")

	// Run migrations to ensure tables exist
	slog.Debug("Running SQLite migrations")
	if _, err := db.Exec(sqliteMigrations); err != nil {
		slog.Error("Failed to run migrations", "error", err)
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}
	slog.Debug("SQLite migrations applied successfully")

	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) AddReceipt(r models.Receipt) error {
	_, err := s.db.Exec(`INSERT INTO receipts (recipient, status, time) VALUES (?, ?, ?)`, r.To, r.Status, r.Time)
	if err != nil {
		slog.Error("SQLiteStore AddReceipt failed", "error", err, "to", r.To)
		return fmt.Errorf("failed to insert receipt for %s: %w", r.To, err)
	}
	slog.Debug("SQLiteStore AddReceipt succeeded", "to", r.To, "status", r.Status)
	return nil
}

func (s *SQLiteStore) GetReceipts() ([]models.Receipt, error) {
	rows, err := s.db.Query(`SELECT recipient, status, time FROM receipts`)
	if err != nil {
		slog.Error("SQLiteStore GetReceipts query failed", "error", err)
		return nil, fmt.Errorf("failed to query receipts: %w", err)
	}
	defer rows.Close()

	var receipts []models.Receipt
	for rows.Next() {
		var r models.Receipt
		if err := rows.Scan(&r.To, &r.Status, &r.Time); err != nil {
			slog.Error("SQLiteStore GetReceipts scan failed", "error", err)
			return nil, fmt.Errorf("failed to scan receipt row: %w", err)
		}
		receipts = append(receipts, r)
	}
	if err := rows.Err(); err != nil {
		slog.Error("SQLiteStore GetReceipts rows iteration failed", "error", err)
		return nil, fmt.Errorf("failed to iterate receipt rows: %w", err)
	}
	slog.Debug("SQLiteStore GetReceipts succeeded", "count", len(receipts))
	return receipts, nil
}

func (s *SQLiteStore) AddResponse(r models.Response) error {
	_, err := s.db.Exec(`INSERT INTO responses (sender, body, time) VALUES (?, ?, ?)`, r.From, r.Body, r.Time)
	if err != nil {
		slog.Error("SQLiteStore AddResponse failed", "error", err, "from", r.From)
		return fmt.Errorf("failed to insert response from %s: %w", r.From, err)
	}
	slog.Debug("SQLiteStore AddResponse succeeded", "from", r.From)
	return nil
}

func (s *SQLiteStore) GetResponses() ([]models.Response, error) {
	rows, err := s.db.Query(`SELECT sender, body, time FROM responses`)
	if err != nil {
		slog.Error("SQLiteStore GetResponses query failed", "error", err)
		return nil, err
	}
	defer rows.Close()

	var responses []models.Response
	for rows.Next() {
		var r models.Response
		if err := rows.Scan(&r.From, &r.Body, &r.Time); err != nil {
			slog.Error("SQLiteStore GetResponses scan failed", "error", err)
			return nil, err
		}
		responses = append(responses, r)
	}
	slog.Debug("SQLiteStore GetResponses succeeded", "count", len(responses))
	return responses, nil
}

// ClearReceipts deletes all records in receipts table (for tests).
func (s *SQLiteStore) ClearReceipts() error {
	_, err := s.db.Exec("DELETE FROM receipts")
	if err != nil {
		slog.Error("SQLiteStore ClearReceipts failed", "error", err)
		return err
	}
	slog.Debug("SQLiteStore ClearReceipts succeeded")
	return nil
}

// ClearResponses deletes all records in responses table (for tests).
func (s *SQLiteStore) ClearResponses() error {
	_, err := s.db.Exec("DELETE FROM responses")
	if err != nil {
		slog.Error("SQLiteStore ClearResponses failed", "error", err)
		return err
	}
	slog.Debug("SQLiteStore ClearResponses succeeded")
	return nil
}

// Close closes the SQLite database connection.
func (s *SQLiteStore) Close() error {
	slog.Debug("Closing SQLite database connection")
	err := s.db.Close()
	if err != nil {
		slog.Error("Failed to close SQLite database", "error", err)
	} else {
		slog.Debug("SQLite database connection closed successfully")
	}
	return err
}

// SaveFlowState stores or updates flow state for a participant.
func (s *SQLiteStore) SaveFlowState(state models.FlowState) error {
	query := `
		INSERT OR REPLACE INTO flow_states (participant_id, flow_type, current_state, state_data, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`

	// Convert state_data map to JSON string for SQLite
	var stateDataJSON string
	if len(state.StateData) > 0 {
		jsonBytes, err := json.Marshal(state.StateData)
		if err != nil {
			slog.Error("SQLiteStore SaveFlowState JSON marshal failed", "error", err, "participantID", state.ParticipantID)
			return err
		}
		stateDataJSON = string(jsonBytes)
	}

	_, err := s.db.Exec(query, state.ParticipantID, state.FlowType, state.CurrentState,
		stateDataJSON, state.CreatedAt, state.UpdatedAt)
	if err != nil {
		slog.Error("SQLiteStore SaveFlowState failed", "error", err, "participantID", state.ParticipantID, "flowType", state.FlowType)
		return err
	}
	slog.Debug("SQLiteStore SaveFlowState succeeded", "participantID", state.ParticipantID, "flowType", state.FlowType, "state", state.CurrentState)
	return nil
}

// GetFlowState retrieves flow state for a participant.
func (s *SQLiteStore) GetFlowState(participantID, flowType string) (*models.FlowState, error) {
	query := `SELECT participant_id, flow_type, current_state, state_data, created_at, updated_at 
			  FROM flow_states WHERE participant_id = ? AND flow_type = ?`

	var state models.FlowState
	var stateDataJSON string

	err := s.db.QueryRow(query, participantID, flowType).Scan(
		&state.ParticipantID, &state.FlowType, &state.CurrentState,
		&stateDataJSON, &state.CreatedAt, &state.UpdatedAt)

	if err == sql.ErrNoRows {
		slog.Debug("SQLiteStore GetFlowState not found", "participantID", participantID, "flowType", flowType)
		return nil, nil
	}
	if err != nil {
		slog.Error("SQLiteStore GetFlowState failed", "error", err, "participantID", participantID, "flowType", flowType)
		return nil, err
	}

	// Convert JSON back to map[string]string
	if stateDataJSON != "" {
		state.StateData = make(map[models.DataKey]string)
		if err := json.Unmarshal([]byte(stateDataJSON), &state.StateData); err != nil {
			slog.Error("SQLiteStore GetFlowState JSON unmarshal failed", "error", err, "participantID", participantID)
			// Continue with empty map rather than failing
			state.StateData = make(map[models.DataKey]string)
		}
	}

	slog.Debug("SQLiteStore GetFlowState found", "participantID", participantID, "flowType", flowType, "state", state.CurrentState)
	return &state, nil
}

// DeleteFlowState removes flow state for a participant.
func (s *SQLiteStore) DeleteFlowState(participantID, flowType string) error {
	query := `DELETE FROM flow_states WHERE participant_id = ? AND flow_type = ?`

	_, err := s.db.Exec(query, participantID, flowType)
	if err != nil {
		slog.Error("SQLiteStore DeleteFlowState failed", "error", err, "participantID", participantID, "flowType", flowType)
		return err
	}
	slog.Debug("SQLiteStore DeleteFlowState succeeded", "participantID", participantID, "flowType", flowType)
	return nil
}

// Intervention participant management methods - SQLite implementation

// SaveInterventionParticipant stores or updates an intervention participant.
func (s *SQLiteStore) SaveInterventionParticipant(participant models.InterventionParticipant) error {
	query := `
		INSERT OR REPLACE INTO intervention_participants (id, phone_number, name, timezone, status, enrolled_at, daily_prompt_time, weekly_reset, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := s.db.Exec(query, participant.ID, participant.PhoneNumber, participant.Name, participant.Timezone,
		string(participant.Status), participant.EnrolledAt, participant.DailyPromptTime, participant.WeeklyReset,
		participant.CreatedAt, participant.UpdatedAt)
	if err != nil {
		slog.Error("SQLiteStore SaveInterventionParticipant failed", "error", err, "id", participant.ID)
		return err
	}
	slog.Debug("SQLiteStore SaveInterventionParticipant succeeded", "id", participant.ID, "phone", participant.PhoneNumber)
	return nil
}

// GetInterventionParticipant retrieves an intervention participant by ID.
func (s *SQLiteStore) GetInterventionParticipant(id string) (*models.InterventionParticipant, error) {
	query := `SELECT id, phone_number, name, timezone, status, enrolled_at, daily_prompt_time, weekly_reset, created_at, updated_at 
			  FROM intervention_participants WHERE id = ?`

	var participant models.InterventionParticipant
	var status string

	err := s.db.QueryRow(query, id).Scan(
		&participant.ID, &participant.PhoneNumber, &participant.Name, &participant.Timezone,
		&status, &participant.EnrolledAt, &participant.DailyPromptTime, &participant.WeeklyReset,
		&participant.CreatedAt, &participant.UpdatedAt)

	if err == sql.ErrNoRows {
		slog.Debug("SQLiteStore GetInterventionParticipant not found", "id", id)
		return nil, nil
	}
	if err != nil {
		slog.Error("SQLiteStore GetInterventionParticipant failed", "error", err, "id", id)
		return nil, err
	}

	participant.Status = models.InterventionParticipantStatus(status)
	slog.Debug("SQLiteStore GetInterventionParticipant found", "id", id)
	return &participant, nil
}

// GetInterventionParticipantByPhone retrieves an intervention participant by phone number.
func (s *SQLiteStore) GetInterventionParticipantByPhone(phoneNumber string) (*models.InterventionParticipant, error) {
	query := `SELECT id, phone_number, name, timezone, status, enrolled_at, daily_prompt_time, weekly_reset, created_at, updated_at 
			  FROM intervention_participants WHERE phone_number = ?`

	var participant models.InterventionParticipant
	var status string

	err := s.db.QueryRow(query, phoneNumber).Scan(
		&participant.ID, &participant.PhoneNumber, &participant.Name, &participant.Timezone,
		&status, &participant.EnrolledAt, &participant.DailyPromptTime, &participant.WeeklyReset,
		&participant.CreatedAt, &participant.UpdatedAt)

	if err == sql.ErrNoRows {
		slog.Debug("SQLiteStore GetInterventionParticipantByPhone not found", "phone", phoneNumber)
		return nil, nil
	}
	if err != nil {
		slog.Error("SQLiteStore GetInterventionParticipantByPhone failed", "error", err, "phone", phoneNumber)
		return nil, err
	}

	participant.Status = models.InterventionParticipantStatus(status)
	slog.Debug("SQLiteStore GetInterventionParticipantByPhone found", "phone", phoneNumber, "id", participant.ID)
	return &participant, nil
}

// ListInterventionParticipants retrieves all intervention participants.
func (s *SQLiteStore) ListInterventionParticipants() ([]models.InterventionParticipant, error) {
	query := `SELECT id, phone_number, name, timezone, status, enrolled_at, daily_prompt_time, weekly_reset, created_at, updated_at 
			  FROM intervention_participants ORDER BY created_at DESC`

	rows, err := s.db.Query(query)
	if err != nil {
		slog.Error("SQLiteStore ListInterventionParticipants failed", "error", err)
		return nil, err
	}
	defer rows.Close()

	var participants []models.InterventionParticipant
	for rows.Next() {
		var participant models.InterventionParticipant
		var status string

		err := rows.Scan(
			&participant.ID, &participant.PhoneNumber, &participant.Name, &participant.Timezone,
			&status, &participant.EnrolledAt, &participant.DailyPromptTime, &participant.WeeklyReset,
			&participant.CreatedAt, &participant.UpdatedAt)
		if err != nil {
			slog.Error("SQLiteStore ListInterventionParticipants scan failed", "error", err)
			return nil, err
		}

		participant.Status = models.InterventionParticipantStatus(status)
		participants = append(participants, participant)
	}

	if err := rows.Err(); err != nil {
		slog.Error("SQLiteStore ListInterventionParticipants rows error", "error", err)
		return nil, err
	}

	slog.Debug("SQLiteStore ListInterventionParticipants succeeded", "count", len(participants))
	return participants, nil
}

// DeleteInterventionParticipant removes an intervention participant.
func (s *SQLiteStore) DeleteInterventionParticipant(id string) error {
	query := `DELETE FROM intervention_participants WHERE id = ?`

	_, err := s.db.Exec(query, id)
	if err != nil {
		slog.Error("SQLiteStore DeleteInterventionParticipant failed", "error", err, "id", id)
		return err
	}
	slog.Debug("SQLiteStore DeleteInterventionParticipant succeeded", "id", id)
	return nil
}

// SaveInterventionResponse stores an intervention response.
func (s *SQLiteStore) SaveInterventionResponse(response models.InterventionResponse) error {
	query := `
		INSERT INTO intervention_responses (id, participant_id, state, response_text, response_type, timestamp)
		VALUES (?, ?, ?, ?, ?, ?)`

	_, err := s.db.Exec(query, response.ID, response.ParticipantID, response.State,
		response.ResponseText, response.ResponseType, response.Timestamp)
	if err != nil {
		slog.Error("SQLiteStore SaveInterventionResponse failed", "error", err, "id", response.ID)
		return err
	}
	slog.Debug("SQLiteStore SaveInterventionResponse succeeded", "id", response.ID, "participantID", response.ParticipantID)
	return nil
}

// GetInterventionResponses retrieves all responses for a participant.
func (s *SQLiteStore) GetInterventionResponses(participantID string) ([]models.InterventionResponse, error) {
	query := `SELECT id, participant_id, state, response_text, response_type, timestamp 
			  FROM intervention_responses WHERE participant_id = ? ORDER BY timestamp DESC`

	rows, err := s.db.Query(query, participantID)
	if err != nil {
		slog.Error("SQLiteStore GetInterventionResponses failed", "error", err, "participantID", participantID)
		return nil, err
	}
	defer rows.Close()

	var responses []models.InterventionResponse
	for rows.Next() {
		var response models.InterventionResponse

		err := rows.Scan(
			&response.ID, &response.ParticipantID, &response.State,
			&response.ResponseText, &response.ResponseType, &response.Timestamp)
		if err != nil {
			slog.Error("SQLiteStore GetInterventionResponses scan failed", "error", err)
			return nil, err
		}

		responses = append(responses, response)
	}

	if err := rows.Err(); err != nil {
		slog.Error("SQLiteStore GetInterventionResponses rows error", "error", err)
		return nil, err
	}

	slog.Debug("SQLiteStore GetInterventionResponses succeeded", "participantID", participantID, "count", len(responses))
	return responses, nil
}

// ListAllInterventionResponses retrieves all intervention responses.
func (s *SQLiteStore) ListAllInterventionResponses() ([]models.InterventionResponse, error) {
	query := `SELECT id, participant_id, state, response_text, response_type, timestamp 
			  FROM intervention_responses ORDER BY timestamp DESC`

	rows, err := s.db.Query(query)
	if err != nil {
		slog.Error("SQLiteStore ListAllInterventionResponses failed", "error", err)
		return nil, err
	}
	defer rows.Close()

	var responses []models.InterventionResponse
	for rows.Next() {
		var response models.InterventionResponse

		err := rows.Scan(
			&response.ID, &response.ParticipantID, &response.State,
			&response.ResponseText, &response.ResponseType, &response.Timestamp)
		if err != nil {
			slog.Error("SQLiteStore ListAllInterventionResponses scan failed", "error", err)
			return nil, err
		}

		responses = append(responses, response)
	}

	if err := rows.Err(); err != nil {
		slog.Error("SQLiteStore ListAllInterventionResponses rows error", "error", err)
		return nil, err
	}

	slog.Debug("SQLiteStore ListAllInterventionResponses succeeded", "count", len(responses))
	return responses, nil
}

// Conversation participant management methods - SQLite implementation

// SaveConversationParticipant stores or updates a conversation participant.
func (s *SQLiteStore) SaveConversationParticipant(participant models.ConversationParticipant) error {
	query := `
		INSERT OR REPLACE INTO conversation_participants (id, phone_number, name, gender, ethnicity, background, status, enrolled_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := s.db.Exec(query, participant.ID, participant.PhoneNumber, participant.Name, participant.Gender,
		participant.Ethnicity, participant.Background, string(participant.Status), participant.EnrolledAt,
		participant.CreatedAt, participant.UpdatedAt)
	if err != nil {
		slog.Error("SQLiteStore SaveConversationParticipant failed", "error", err, "id", participant.ID)
		return err
	}
	slog.Debug("SQLiteStore SaveConversationParticipant succeeded", "id", participant.ID, "phone", participant.PhoneNumber)
	return nil
}

// GetConversationParticipant retrieves a conversation participant by ID.
func (s *SQLiteStore) GetConversationParticipant(id string) (*models.ConversationParticipant, error) {
	query := `SELECT id, phone_number, name, gender, ethnicity, background, status, enrolled_at, created_at, updated_at 
			  FROM conversation_participants WHERE id = ?`

	var participant models.ConversationParticipant
	var status string

	err := s.db.QueryRow(query, id).Scan(
		&participant.ID, &participant.PhoneNumber, &participant.Name, &participant.Gender,
		&participant.Ethnicity, &participant.Background, &status, &participant.EnrolledAt,
		&participant.CreatedAt, &participant.UpdatedAt)

	if err == sql.ErrNoRows {
		slog.Debug("SQLiteStore GetConversationParticipant not found", "id", id)
		return nil, nil
	}
	if err != nil {
		slog.Error("SQLiteStore GetConversationParticipant failed", "error", err, "id", id)
		return nil, err
	}

	participant.Status = models.ConversationParticipantStatus(status)
	slog.Debug("SQLiteStore GetConversationParticipant found", "id", id)
	return &participant, nil
}

// GetConversationParticipantByPhone retrieves a conversation participant by phone number.
func (s *SQLiteStore) GetConversationParticipantByPhone(phoneNumber string) (*models.ConversationParticipant, error) {
	query := `SELECT id, phone_number, name, gender, ethnicity, background, status, enrolled_at, created_at, updated_at 
			  FROM conversation_participants WHERE phone_number = ?`

	var participant models.ConversationParticipant
	var status string

	err := s.db.QueryRow(query, phoneNumber).Scan(
		&participant.ID, &participant.PhoneNumber, &participant.Name, &participant.Gender,
		&participant.Ethnicity, &participant.Background, &status, &participant.EnrolledAt,
		&participant.CreatedAt, &participant.UpdatedAt)

	if err == sql.ErrNoRows {
		slog.Debug("SQLiteStore GetConversationParticipantByPhone not found", "phone", phoneNumber)
		return nil, nil
	}
	if err != nil {
		slog.Error("SQLiteStore GetConversationParticipantByPhone failed", "error", err, "phone", phoneNumber)
		return nil, err
	}

	participant.Status = models.ConversationParticipantStatus(status)
	slog.Debug("SQLiteStore GetConversationParticipantByPhone found", "phone", phoneNumber, "id", participant.ID)
	return &participant, nil
}

// ListConversationParticipants retrieves all conversation participants.
func (s *SQLiteStore) ListConversationParticipants() ([]models.ConversationParticipant, error) {
	query := `SELECT id, phone_number, name, gender, ethnicity, background, status, enrolled_at, created_at, updated_at 
			  FROM conversation_participants ORDER BY created_at DESC`

	rows, err := s.db.Query(query)
	if err != nil {
		slog.Error("SQLiteStore ListConversationParticipants failed", "error", err)
		return nil, err
	}
	defer rows.Close()

	var participants []models.ConversationParticipant
	for rows.Next() {
		var participant models.ConversationParticipant
		var status string

		err := rows.Scan(
			&participant.ID, &participant.PhoneNumber, &participant.Name, &participant.Gender,
			&participant.Ethnicity, &participant.Background, &status, &participant.EnrolledAt,
			&participant.CreatedAt, &participant.UpdatedAt)
		if err != nil {
			slog.Error("SQLiteStore ListConversationParticipants scan failed", "error", err)
			return nil, err
		}

		participant.Status = models.ConversationParticipantStatus(status)
		participants = append(participants, participant)
	}

	if err := rows.Err(); err != nil {
		slog.Error("SQLiteStore ListConversationParticipants rows error", "error", err)
		return nil, err
	}

	slog.Debug("SQLiteStore ListConversationParticipants succeeded", "count", len(participants))
	return participants, nil
}

// DeleteConversationParticipant removes a conversation participant.
func (s *SQLiteStore) DeleteConversationParticipant(id string) error {
	query := `DELETE FROM conversation_participants WHERE id = ?`

	_, err := s.db.Exec(query, id)
	if err != nil {
		slog.Error("SQLiteStore DeleteConversationParticipant failed", "error", err, "id", id)
		return err
	}
	slog.Debug("SQLiteStore DeleteConversationParticipant succeeded", "id", id)
	return nil
}

// Hook persistence management methods - SQLite implementation

// SaveRegisteredHook stores or updates a registered hook.
func (s *SQLiteStore) SaveRegisteredHook(hook models.RegisteredHook) error {
	// Convert parameters map to JSON string for SQLite
	parametersJSON, err := json.Marshal(hook.Parameters)
	if err != nil {
		slog.Error("SQLiteStore SaveRegisteredHook JSON marshal failed", "error", err, "phoneNumber", hook.PhoneNumber)
		return fmt.Errorf("failed to marshal parameters: %w", err)
	}

	query := `
		INSERT OR REPLACE INTO registered_hooks (phone_number, hook_type, parameters, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`

	_, err = s.db.Exec(query, hook.PhoneNumber, string(hook.HookType), string(parametersJSON), hook.CreatedAt, hook.UpdatedAt)
	if err != nil {
		slog.Error("SQLiteStore SaveRegisteredHook failed", "error", err, "phoneNumber", hook.PhoneNumber)
		return err
	}
	slog.Debug("SQLiteStore SaveRegisteredHook succeeded", "phoneNumber", hook.PhoneNumber, "hookType", hook.HookType)
	return nil
}

// GetRegisteredHook retrieves a registered hook by phone number.
func (s *SQLiteStore) GetRegisteredHook(phoneNumber string) (*models.RegisteredHook, error) {
	query := `SELECT phone_number, hook_type, parameters, created_at, updated_at 
			  FROM registered_hooks WHERE phone_number = ?`

	var hook models.RegisteredHook
	var hookType string
	var parametersJSON string

	err := s.db.QueryRow(query, phoneNumber).Scan(
		&hook.PhoneNumber, &hookType, &parametersJSON, &hook.CreatedAt, &hook.UpdatedAt)

	if err == sql.ErrNoRows {
		slog.Debug("SQLiteStore GetRegisteredHook not found", "phoneNumber", phoneNumber)
		return nil, nil
	}
	if err != nil {
		slog.Error("SQLiteStore GetRegisteredHook failed", "error", err, "phoneNumber", phoneNumber)
		return nil, err
	}

	// Parse JSON parameters
	if err := json.Unmarshal([]byte(parametersJSON), &hook.Parameters); err != nil {
		slog.Error("SQLiteStore GetRegisteredHook JSON unmarshal failed", "error", err, "phoneNumber", phoneNumber)
		return nil, fmt.Errorf("failed to unmarshal parameters: %w", err)
	}

	hook.HookType = models.HookType(hookType)
	slog.Debug("SQLiteStore GetRegisteredHook found", "phoneNumber", phoneNumber, "hookType", hook.HookType)
	return &hook, nil
}

// ListRegisteredHooks retrieves all registered hooks.
func (s *SQLiteStore) ListRegisteredHooks() ([]models.RegisteredHook, error) {
	query := `SELECT phone_number, hook_type, parameters, created_at, updated_at 
			  FROM registered_hooks ORDER BY created_at DESC`

	rows, err := s.db.Query(query)
	if err != nil {
		slog.Error("SQLiteStore ListRegisteredHooks failed", "error", err)
		return nil, err
	}
	defer rows.Close()

	var hooks []models.RegisteredHook
	for rows.Next() {
		var hook models.RegisteredHook
		var hookType string
		var parametersJSON string

		err := rows.Scan(
			&hook.PhoneNumber, &hookType, &parametersJSON, &hook.CreatedAt, &hook.UpdatedAt)
		if err != nil {
			slog.Error("SQLiteStore ListRegisteredHooks scan failed", "error", err)
			return nil, err
		}

		// Parse JSON parameters
		if err := json.Unmarshal([]byte(parametersJSON), &hook.Parameters); err != nil {
			slog.Error("SQLiteStore ListRegisteredHooks JSON unmarshal failed", "error", err, "phoneNumber", hook.PhoneNumber)
			continue // Skip this hook and continue with others
		}

		hook.HookType = models.HookType(hookType)
		hooks = append(hooks, hook)
	}

	if err := rows.Err(); err != nil {
		slog.Error("SQLiteStore ListRegisteredHooks rows error", "error", err)
		return nil, err
	}

	slog.Debug("SQLiteStore ListRegisteredHooks succeeded", "count", len(hooks))
	return hooks, nil
}

// DeleteRegisteredHook removes a registered hook by phone number.
func (s *SQLiteStore) DeleteRegisteredHook(phoneNumber string) error {
	query := `DELETE FROM registered_hooks WHERE phone_number = ?`

	result, err := s.db.Exec(query, phoneNumber)
	if err != nil {
		return fmt.Errorf("failed to delete registered hook: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		slog.Debug("SQLiteStore registered hook not found for deletion", "phoneNumber", phoneNumber)
	} else {
		slog.Debug("SQLiteStore registered hook deleted", "phoneNumber", phoneNumber)
	}

	return nil
}
