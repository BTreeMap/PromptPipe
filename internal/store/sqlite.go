// Package store provides storage backends for PromptPipe.
//
// This file implements an SQLite-backed store for receipts and responses.
package store

import (
	"database/sql"
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

//go:embed migrations.sql
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
		slog.Error("Failed to create database directory", "error", err, "dir", dir, "dsn", dsn)
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}
	slog.Debug("SQLite database directory verified/created", "dir", dir, "db_path", dsn)

	slog.Debug("Opening SQLite database connection", "path", dsn)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		slog.Error("Failed to open SQLite connection", "error", err, "dsn", dsn)
		return nil, err
	}
	slog.Debug("SQLite database opened", "dsn", dsn)

	if err := db.Ping(); err != nil {
		slog.Error("SQLite ping failed", "error", err, "dsn", dsn)
		return nil, err
	}
	slog.Debug("SQLite ping successful", "dsn", dsn)

	// Run migrations to ensure tables exist
	slog.Debug("Running SQLite migrations", "dsn", dsn)
	if _, err := db.Exec(sqliteMigrations); err != nil {
		slog.Error("Failed to run migrations", "error", err, "dsn", dsn)
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}
	slog.Debug("SQLite migrations applied successfully", "dsn", dsn)

	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) AddReceipt(r models.Receipt) error {
	_, err := s.db.Exec(`INSERT INTO receipts (recipient, status, time) VALUES (?, ?, ?)`, r.To, r.Status, r.Time)
	if err != nil {
		slog.Error("SQLiteStore AddReceipt failed", "error", err, "to", r.To)
		return err
	}
	slog.Debug("SQLiteStore AddReceipt succeeded", "to", r.To, "status", r.Status)
	return nil
}

func (s *SQLiteStore) GetReceipts() ([]models.Receipt, error) {
	rows, err := s.db.Query(`SELECT recipient, status, time FROM receipts`)
	if err != nil {
		slog.Error("SQLiteStore GetReceipts query failed", "error", err)
		return nil, err
	}
	defer rows.Close()

	var receipts []models.Receipt
	for rows.Next() {
		var r models.Receipt
		if err := rows.Scan(&r.To, &r.Status, &r.Time); err != nil {
			slog.Error("SQLiteStore GetReceipts scan failed", "error", err)
			return nil, err
		}
		receipts = append(receipts, r)
	}
	slog.Debug("SQLiteStore GetReceipts succeeded", "count", len(receipts))
	return receipts, nil
}

func (s *SQLiteStore) AddResponse(r models.Response) error {
	_, err := s.db.Exec(`INSERT INTO responses (sender, body, time) VALUES (?, ?, ?)`, r.From, r.Body, r.Time)
	if err != nil {
		slog.Error("SQLiteStore AddResponse failed", "error", err, "from", r.From)
		return err
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
