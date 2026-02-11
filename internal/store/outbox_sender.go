// Package store provides the OutboxSender for processing outgoing messages.
package store

import (
	"context"
	"log/slog"
	"time"
)

// OutboxSendFunc is the callback that performs the actual message send.
// It receives the outbox message and should return an error if sending failed.
type OutboxSendFunc func(ctx context.Context, msg OutboxMessage) error

// OutboxSender periodically claims due outbox messages and attempts to send them.
type OutboxSender struct {
	repo           OutboxRepo
	sendFunc       OutboxSendFunc
	pollInterval   time.Duration
	staleThreshold time.Duration
	claimLimit     int
}

// NewOutboxSender creates a new OutboxSender.
func NewOutboxSender(repo OutboxRepo, sendFunc OutboxSendFunc, pollInterval time.Duration) *OutboxSender {
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}
	return &OutboxSender{
		repo:           repo,
		sendFunc:       sendFunc,
		pollInterval:   pollInterval,
		staleThreshold: 5 * time.Minute,
		claimLimit:     10,
	}
}

// RecoverStaleMessages requeues messages stuck in sending state (crash recovery).
// Should be called once at startup.
func (s *OutboxSender) RecoverStaleMessages() error {
	staleBefore := time.Now().Add(-s.staleThreshold)
	n, err := s.repo.RequeueStaleSendingMessages(staleBefore)
	if err != nil {
		return err
	}
	if n > 0 {
		slog.Info("OutboxSender.RecoverStaleMessages: requeued stale messages", "count", n)
	}
	return nil
}

// Run starts the polling loop. It blocks until the context is cancelled.
func (s *OutboxSender) Run(ctx context.Context) {
	slog.Info("OutboxSender.Run: starting outbox sender", "pollInterval", s.pollInterval)

	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("OutboxSender.Run: stopping")
			return
		case <-ticker.C:
			s.poll(ctx)
		}
	}
}

func (s *OutboxSender) poll(ctx context.Context) {
	now := time.Now()
	msgs, err := s.repo.ClaimDueOutboxMessages(now, s.claimLimit)
	if err != nil {
		slog.Error("OutboxSender.poll: claim failed", "error", err)
		return
	}

	for _, msg := range msgs {
		slog.Debug("OutboxSender.poll: sending message", "id", msg.ID, "participantID", msg.ParticipantID, "kind", msg.Kind)
		if err := s.sendFunc(ctx, msg); err != nil {
			slog.Error("OutboxSender.poll: send failed", "id", msg.ID, "error", err)
			// Exponential backoff: 10s, 20s, 40s, ...
			backoff := time.Duration(10*(1<<msg.Attempts)) * time.Second
			nextAttempt := now.Add(backoff)
			if err := s.repo.FailOutboxMessage(msg.ID, err.Error(), nextAttempt); err != nil {
				slog.Error("OutboxSender.poll: fail message error", "id", msg.ID, "error", err)
			}
		} else {
			if err := s.repo.MarkOutboxMessageSent(msg.ID); err != nil {
				slog.Error("OutboxSender.poll: mark sent error", "id", msg.ID, "error", err)
			}
			slog.Debug("OutboxSender.poll: message sent", "id", msg.ID, "participantID", msg.ParticipantID)
		}
	}
}
