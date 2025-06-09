// Package store provides storage backends for PromptPipe.
//
// This file implements a PostgreSQL-backed store for receipts.
package store

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/BTreeMap/PromptPipe/internal/models"
	_ "github.com/lib/pq"
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
	slog.Debug("NewPostgresStore invoked", "DSN_set", cfg.DSN != "")
	// Determine DSN (required)
	dsn := cfg.DSN
	if dsn == "" {
		slog.Error("PostgresStore DSN not set")
		return nil, fmt.Errorf("database DSN not set")
	}

	slog.Debug("Opening Postgres database connection", "dsn", dsn) // Log the DSN for observability
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		slog.Error("Failed to open Postgres connection", "dsn", dsn, "error", err)
		return nil, err
	}
	slog.Debug("Postgres database opened", "dsn", dsn)
	if err := db.Ping(); err != nil {
		slog.Error("Postgres ping failed", "dsn", dsn, "error", err)
		return nil, err
	}
	slog.Debug("Postgres ping successful", "dsn", dsn)
	// Run migrations to ensure receipts table exists
	slog.Debug("Running Postgres migrations", "dsn", dsn)
	if _, err := db.Exec(postgresMigrations); err != nil {
		slog.Error("Failed to run migrations", "dsn", dsn, "error", err)
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}
	slog.Debug("Postgres migrations applied successfully", "dsn", dsn)
	return &PostgresStore{db: db}, nil
}

func (s *PostgresStore) AddReceipt(r models.Receipt) error {
	_, err := s.db.Exec(`INSERT INTO receipts (recipient, status, time) VALUES ($1, $2, $3)`, r.To, r.Status, r.Time)
	if err != nil {
		slog.Error("PostgresStore AddReceipt failed", "error", err, "to", r.To)
		return err
	}
	slog.Debug("PostgresStore AddReceipt succeeded", "to", r.To, "status", r.Status)
	return nil
}

func (s *PostgresStore) GetReceipts() ([]models.Receipt, error) {
	rows, err := s.db.Query(`SELECT recipient, status, time FROM receipts`)
	if err != nil {
		slog.Error("PostgresStore GetReceipts query failed", "error", err)
		return nil, err
	}
	defer rows.Close()
	var receipts []models.Receipt
	for rows.Next() {
		var r models.Receipt
		if err := rows.Scan(&r.To, &r.Status, &r.Time); err != nil {
			slog.Error("PostgresStore GetReceipts scan failed", "error", err)
			return nil, err
		}
		receipts = append(receipts, r)
	}
	slog.Debug("PostgresStore GetReceipts succeeded", "count", len(receipts))
	return receipts, nil
}

// AddResponse stores an incoming response in Postgres.
func (s *PostgresStore) AddResponse(r models.Response) error {
	_, err := s.db.Exec(`INSERT INTO responses (sender, body, time) VALUES ($1, $2, $3)`, r.From, r.Body, r.Time)
	if err != nil {
		slog.Error("PostgresStore AddResponse failed", "error", err, "from", r.From)
		return err
	}
	slog.Debug("PostgresStore AddResponse succeeded", "from", r.From)
	return nil
}

// GetResponses retrieves all stored responses from Postgres.
func (s *PostgresStore) GetResponses() ([]models.Response, error) {
	rows, err := s.db.Query(`SELECT sender, body, time FROM responses`)
	if err != nil {
		slog.Error("PostgresStore GetResponses query failed", "error", err)
		return nil, err
	}
	defer rows.Close()
	var responses []models.Response
	for rows.Next() {
		var r models.Response
		if err := rows.Scan(&r.From, &r.Body, &r.Time); err != nil {
			slog.Error("PostgresStore GetResponses scan failed", "error", err)
			return nil, err
		}
		responses = append(responses, r)
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
	if state.StateData != nil && len(state.StateData) > 0 {
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
		state.StateData = make(map[string]string)
		if err := json.Unmarshal(stateDataJSON, &state.StateData); err != nil {
			slog.Error("PostgresStore GetFlowState JSON unmarshal failed", "error", err, "participantID", participantID)
			// Continue with empty map rather than failing
			state.StateData = make(map[string]string)
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
