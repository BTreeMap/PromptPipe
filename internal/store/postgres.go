// Package store provides storage backends for PromptPipe.
//
// This file implements a PostgreSQL-backed store for receipts.
package store

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"

	"github.com/BTreeMap/PromptPipe/internal/models"
	_ "github.com/lib/pq"
)

//go:embed migrations.sql
var migrations string

type PostgresStore struct {
	db *sql.DB
}

// Opts holds configuration for the Postgres store, allowing override of connection string.
type Opts struct {
	DSN string // overrides DATABASE_URL
}

// Option defines a configuration option for the Postgres store.
type Option func(*Opts)

// WithPostgresDSN overrides the DSN used by the Postgres store.
func WithPostgresDSN(dsn string) Option {
	return func(o *Opts) {
		o.DSN = dsn
	}
}

// NewPostgresStore creates a new Postgres store based on provided options.
func NewPostgresStore(opts ...Option) (*PostgresStore, error) {
	// Apply options
	var cfg Opts
	for _, opt := range opts {
		opt(&cfg)
	}
	// Determine DSN from options
	dsn := cfg.DSN
	if dsn == "" {
		return nil, fmt.Errorf("Postgres DSN not provided")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	// Run migrations to ensure receipts table exists
	if _, err := db.Exec(migrations); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}
	return &PostgresStore{db: db}, nil
}

func (s *PostgresStore) AddReceipt(r models.Receipt) error {
	_, err := s.db.Exec(`INSERT INTO receipts (recipient, status, time) VALUES ($1, $2, $3)`, r.To, r.Status, r.Time)
	return err
}

func (s *PostgresStore) GetReceipts() ([]models.Receipt, error) {
	rows, err := s.db.Query(`SELECT recipient, status, time FROM receipts`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var receipts []models.Receipt
	for rows.Next() {
		var r models.Receipt
		if err := rows.Scan(&r.To, &r.Status, &r.Time); err != nil {
			return nil, err
		}
		receipts = append(receipts, r)
	}
	return receipts, nil
}

// AddResponse stores an incoming response in Postgres.
func (s *PostgresStore) AddResponse(r models.Response) error {
	_, err := s.db.Exec(`INSERT INTO responses (sender, body, time) VALUES ($1, $2, $3)`, r.From, r.Body, r.Time)
	return err
}

// GetResponses retrieves all stored responses from Postgres.
func (s *PostgresStore) GetResponses() ([]models.Response, error) {
	rows, err := s.db.Query(`SELECT sender, body, time FROM responses`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var responses []models.Response
	for rows.Next() {
		var r models.Response
		if err := rows.Scan(&r.From, &r.Body, &r.Time); err != nil {
			return nil, err
		}
		responses = append(responses, r)
	}
	return responses, nil
}
