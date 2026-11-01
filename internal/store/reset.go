package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"teleport/internal/model"
)

// Reset is offline maintenance under the same lifetime ownership as the server.
// The verified backup survives any subsequent replacement/commit failure.
func (s *Store) Reset(ctx context.Context, replacement *model.State) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.postgres {
		return "", errors.New("force initialization supports SQLite only; PostgreSQL requires a separately verified backup/restore procedure")
	}
	if replacement == nil || replacement.Schema != model.SchemaVersion || replacement.InstallationID == "" {
		return "", errors.New("valid fresh installation required")
	}
	var original string
	err := s.db.QueryRowContext(ctx, "SELECT body FROM teleport_state WHERE id=1").Scan(&original)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if original != "" {
		var previous model.State
		if err = json.Unmarshal([]byte(original), &previous); err != nil {
			return "", errors.New("cannot verify retained job ownership; installation was not reset")
		}
		for _, job := range previous.ScriptRuns {
			if job.State != "completed" && job.State != "failed" && job.State != "stopped" {
				return "", errors.New("stop and reconcile retained script jobs before force initialization")
			}
		}
		for _, session := range previous.LocalSessions {
			if session.State != "exited" && session.State != "failed" {
				return "", errors.New("stop and reconcile retained harness sessions before force initialization")
			}
		}
	}
	backup, err := s.backupSQLite(ctx, original)
	if err != nil {
		return "", fmt.Errorf("backup failed; installation was not reset: %w", err)
	}
	encoded, err := json.Marshal(replacement)
	if err != nil {
		return backup, err
	}
	_, err = s.db.ExecContext(ctx, "INSERT INTO teleport_state (id,body) VALUES (1,?) ON CONFLICT(id) DO UPDATE SET body=excluded.body", string(encoded))
	return backup, err
}

func (s *Store) backupSQLite(ctx context.Context, original string) (path string, err error) {
	file, err := os.CreateTemp(filepath.Dir(s.sqlitePath), filepath.Base(s.sqlitePath)+".backup-"+time.Now().UTC().Format("20060102T150405Z")+"-*.sqlite")
	if err != nil {
		return "", err
	}
	path = file.Name()
	ownedPath := path
	defer func() {
		if err != nil {
			os.Remove(ownedPath)
		}
	}()
	if err = file.Close(); err != nil {
		return "", err
	}
	if _, err = s.db.ExecContext(ctx, "VACUUM main INTO ?", path); err != nil {
		return "", err
	}
	u := url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro"}
	copyDB, err := sql.Open("sqlite", u.String())
	if err != nil {
		return "", err
	}
	defer copyDB.Close()
	var check, body string
	if err = copyDB.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&check); err != nil {
		return "", err
	}
	if check != "ok" {
		return "", errors.New("SQLite backup integrity check failed")
	}
	err = copyDB.QueryRowContext(ctx, "SELECT body FROM teleport_state WHERE id=1").Scan(&body)
	if errors.Is(err, sql.ErrNoRows) && original == "" {
		err = nil
	}
	if err != nil {
		return "", err
	}
	if body != original {
		return "", errors.New("SQLite backup state verification failed")
	}
	file, err = os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return "", err
	}
	err = file.Sync()
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	err = directory.Sync()
	closeErr = directory.Close()
	if err == nil {
		err = closeErr
	}
	return path, err
}
