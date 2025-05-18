// Package store provides storage backends for PromptPipe.
//
// This file implements a PostgreSQL-backed store for receipts.
package store

import (
	"database/sql"
	_ "embed"
	"fmt"

	"github.com/BTreeMap/PromptPipe/internal/models"
	_ "github.com/lib/pq"
)

//go:embed migrations.sql
var migrations string

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(connStr string) (*PostgresStore, error) {
	db, err := sql.Open("postgres", connStr)
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
