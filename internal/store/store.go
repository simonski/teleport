// Package store owns the server's durable installation state. Clients never import it.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"teleport/internal/model"
)

var ErrUninitialized = errors.New("storage is not initialized; run tk init")

type Store struct {
	db         *sql.DB
	connection *sql.Conn
	mu         sync.Mutex
	release    func()
}

func Open(location string, initialize bool) (*Store, error) {
	u, err := url.Parse(location)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Path == "" || u.Path == "/" {
		return nil, errors.New("configure a PostgreSQL database URL with TELEPORT_DB or -db-config; run make db-up for local setup")
	}
	s := &Store{}
	db, err := sql.Open("pgx", location)
	if err != nil {
		s.Close()
		return nil, errors.New("invalid PostgreSQL connection configuration")
	}
	s.db = db
	{
		db.SetMaxOpenConns(4)
		ctx, cancel := context.WithTimeout(context.Background(), 10e9)
		defer cancel()
		conn, err := db.Conn(ctx)
		if err != nil {
			s.Close()
			return nil, errors.New("could not connect to PostgreSQL; verify database configuration and service availability")
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
	}
	if initialize {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err = s.connection.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS teleport_state (id INTEGER PRIMARY KEY, body TEXT NOT NULL)")
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
	if s.release != nil {
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
		if errors.Is(err, sql.ErrNoRows) || errors.As(err, &pgError) && pgError.Code == "42P01" {
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
	query += " FOR UPDATE"
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
	_, err = tx.ExecContext(ctx, "INSERT INTO teleport_state (id,body) VALUES (1,$1) ON CONFLICT(id) DO UPDATE SET body=excluded.body", string(encoded))
	if err != nil {
		return err
	}
	return tx.Commit()
}
