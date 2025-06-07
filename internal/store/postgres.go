// Package store provides storage backends for PromptPipe.
//
// This file implements a PostgreSQL-backed store for receipts.
package store

import (
	"database/sql"
	_ "embed"
	"fmt"
	"log/slog"

	"github.com/BTreeMap/PromptPipe/internal/models"
	_ "github.com/lib/pq"
)

//go:embed migrations.sql
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

	slog.Debug("Opening Postgres database connection")
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		slog.Error("Failed to open Postgres connection", "error", err)
		return nil, err
	}
	slog.Debug("Postgres database opened")
	if err := db.Ping(); err != nil {
		slog.Error("Postgres ping failed", "error", err)
		return nil, err
	}
	slog.Debug("Postgres ping successful")
	// Run migrations to ensure receipts table exists
	slog.Debug("Running Postgres migrations")
	if _, err := db.Exec(postgresMigrations); err != nil {
		slog.Error("Failed to run migrations", "error", err)
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}
	slog.Debug("Postgres migrations applied successfully")
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
	return err
}
