// Package store owns the server's durable installation state. Clients never import it.
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
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
	"teleport/internal/model"
)

var ErrUninitialized = errors.New("storage is not initialized; run tk init")

type Store struct {
	db         *sql.DB
	connection *sql.Conn
	postgres   bool
	sqlitePath string
	mu         sync.Mutex
	release    func()
}

func Open(location string, initialize bool) (*Store, error) {
	s := &Store{postgres: strings.HasPrefix(location, "postgres://") || strings.HasPrefix(location, "postgresql://")}
	driver, dsn := "pgx", location
	if !s.postgres {
		abs, err := filepath.Abs(location)
		if err != nil {
			return nil, err
		}
		s.sqlitePath = abs
		if initialize {
			if err = os.MkdirAll(filepath.Dir(abs), 0700); err != nil {
				return nil, err
			}
		} else if _, err = os.Stat(abs); err != nil {
			return nil, ErrUninitialized
		}
		release, err := lockFile(abs + ".lock")
		if err != nil {
			return nil, err
		}
		s.release = release
		if initialize {
			f, err := os.OpenFile(abs, os.O_CREATE|os.O_RDWR, 0600)
			if err != nil {
				s.Close()
				return nil, err
			}
			f.Close()
		}
		u := url.URL{Scheme: "file", Path: abs}
		q := u.Query()
		q.Add("_pragma", "foreign_keys(1)")
		q.Add("_pragma", "busy_timeout(5000)")
		q.Add("_pragma", "journal_mode(WAL)")
		u.RawQuery = q.Encode()
		driver, dsn = "sqlite", u.String()
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		s.Close()
		return nil, err
	}
	s.db = db
	if s.postgres {
		db.SetMaxOpenConns(4)
		ctx, cancel := context.WithTimeout(context.Background(), 10e9)
		defer cancel()
		conn, err := db.Conn(ctx)
		if err != nil {
			s.Close()
			return nil, fmt.Errorf("connect storage: %w", err)
		}
		var locked bool
		err = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(1949675634)").Scan(&locked)
		if err != nil || !locked {
			conn.Close()
			s.Close()
			return nil, errors.New("installation is already owned or its lock is unavailable")
		}
		s.release = func() {
			defer conn.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			conn.ExecContext(ctx, "SELECT pg_advisory_unlock(1949675634)")
		}
		s.connection = conn
	} else {
		db.SetMaxOpenConns(1)
	}
	if initialize {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if s.connection != nil {
			_, err = s.connection.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS teleport_state (id INTEGER PRIMARY KEY, body TEXT NOT NULL)")
		} else {
			_, err = db.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS teleport_state (id INTEGER PRIMARY KEY, body TEXT NOT NULL)")
		}
		if err != nil {
			s.Close()
			return nil, err
		}
	}
	return s, nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.postgres && s.release != nil {
		s.release()
		s.release = nil
	}
	var err error
	if s.db != nil {
		err = s.db.Close()
	}
	if s.release != nil {
		s.release()
		s.release = nil
	}
	return err
}
func (s *Store) Bind(query string) string {
	if !s.postgres {
		return query
	}
	n := 0
	return replaceParams(query, &n)
}
func replaceParams(query string, n *int) string {
	var b strings.Builder
	for _, r := range query {
		if r == '?' {
			*n++
			fmt.Fprintf(&b, "$%d", *n)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (s *Store) Read(ctx context.Context) (*model.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var body string
	query := s.db.QueryRowContext
	if s.connection != nil {
		query = s.connection.QueryRowContext
	}
	if err := query(ctx, "SELECT body FROM teleport_state WHERE id=1").Scan(&body); err != nil {
		var pgError *pgconn.PgError
		if errors.Is(err, sql.ErrNoRows) || errors.As(err, &pgError) && pgError.Code == "42P01" || strings.Contains(err.Error(), "no such table: teleport_state") {
			return nil, ErrUninitialized
		}
		return nil, fmt.Errorf("read storage: %w", err)
	}
	return decode(body)
}
func decode(body string) (*model.State, error) {
	var st model.State
	if err := json.Unmarshal([]byte(body), &st); err != nil {
		return nil, fmt.Errorf("read installation: %w", err)
	}
	if st.Schema != model.SchemaVersion {
		return nil, fmt.Errorf("unsupported storage schema %d", st.Schema)
	}
	return &st, nil
}

// Update commits authorization, domain state and history together. A false result is a no-op.
func (s *Store) Update(ctx context.Context, fn func(*model.State) (bool, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	begin := s.db.BeginTx
	if s.connection != nil {
		begin = s.connection.BeginTx
	}
	tx, err := begin(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	query := "SELECT body FROM teleport_state WHERE id=1"
	if s.postgres {
		query += " FOR UPDATE"
	}
	var body string
	err = tx.QueryRowContext(ctx, query).Scan(&body)
	st := &model.State{}
	if err == nil {
		st, err = decode(body)
	} else if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return err
	}
	changed, err := fn(st)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	st.WithdrawSuspendedPrivateExecutions()
	st.Revision++
	encoded, err := json.Marshal(st)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, s.Bind("INSERT INTO teleport_state (id,body) VALUES (1,?) ON CONFLICT(id) DO UPDATE SET body=excluded.body"), string(encoded))
	if err != nil {
		return err
	}
	return tx.Commit()
}
