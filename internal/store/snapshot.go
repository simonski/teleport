package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"

	"teleport/internal/model"
)

// Snapshot is offline maintenance under the installation's exclusive ownership.
// Raw state retains IDs, versions, password verifiers and session authority.
func (s *Store) Snapshot(ctx context.Context) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var body string
	if err := s.connection.QueryRowContext(ctx, "SELECT body FROM teleport_state WHERE id=1").Scan(&body); err != nil {
		return nil, err
	}
	if _, err := decode(body); err != nil {
		return nil, err
	}
	return []byte(body), nil
}

// RestoreSnapshot accepts an empty target or an exact retry, never a replacement.
func (s *Store) RestoreSnapshot(ctx context.Context, body []byte, expected string) error {
	hash := sha256.Sum256(body)
	if len(expected) != 64 || hex.EncodeToString(hash[:]) != expected {
		return errors.New("state checksum mismatch")
	}
	var state model.State
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil || decoder.Decode(new(any)) != io.EOF {
		return errors.New("invalid or incompatible installation snapshot")
	}
	if state.Schema != model.SchemaVersion || len(state.InstallationID) < 16 || len(state.Accounts) == 0 || len(state.Workflows) == 0 {
		return errors.New("snapshot lacks a compatible installation identity or bootstrap state")
	}
	for _, job := range state.ScriptRuns {
		if job.State != "completed" && job.State != "failed" && job.State != "stopped" {
			return errors.New("stop and reconcile retained script jobs before migration")
		}
	}
	for _, session := range state.LocalSessions {
		if session.State != "exited" && session.State != "failed" {
			return errors.New("stop and reconcile retained harness sessions before migration")
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.connection.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var current string
	err = tx.QueryRowContext(ctx, "SELECT body FROM teleport_state WHERE id=1 FOR UPDATE").Scan(&current)
	if err == nil {
		if current != string(body) {
			return errors.New("target already contains a different installation state; restore refused")
		}
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO teleport_state (id,body) VALUES (1,$1)", string(body)); err != nil {
		return err
	}
	return tx.Commit()
}
