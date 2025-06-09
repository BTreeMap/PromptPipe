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
	"path/filepath"

	_ "embed"

	"github.com/BTreeMap/PromptPipe/internal/models"
	_ "github.com/mattn/go-sqlite3"
)

// Constants for SQLite store configuration
const (
	// DefaultDirPermissions defines the default permissions for database directories
	DefaultDirPermissions = 0755
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
	slog.Debug("NewSQLiteStore invoked", "DSN_set", cfg.DSN != "")

	// Determine DSN (required)
	dsn := cfg.DSN
	if dsn == "" {
		slog.Error("SQLiteStore DSN not set")
		return nil, fmt.Errorf("database DSN not set")
	}

	// Ensure the directory exists
	dir := filepath.Dir(dsn)
	if err := os.MkdirAll(dir, DefaultDirPermissions); err != nil {
		slog.Error("Failed to create database directory", "error", err, "dir", dir)
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}
	slog.Debug("SQLite database directory verified/created", "dir", dir)

	slog.Debug("Opening SQLite database connection")
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		slog.Error("Failed to open SQLite connection", "error", err)
		return nil, err
	}
	slog.Debug("SQLite database opened")

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
	if state.StateData != nil && len(state.StateData) > 0 {
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
		state.StateData = make(map[string]string)
		if err := json.Unmarshal([]byte(stateDataJSON), &state.StateData); err != nil {
			slog.Error("SQLiteStore GetFlowState JSON unmarshal failed", "error", err, "participantID", participantID)
			// Continue with empty map rather than failing
			state.StateData = make(map[string]string)
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
