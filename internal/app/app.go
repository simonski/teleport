package app

import (
	"bytes"
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"teleport/internal/model"
	"teleport/internal/store"
)

type Error struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string                       { return e.Message }
func failure(status int, code, message string) error { return &Error{status, code, message} }
func randomID() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func digest(s string) string { sum := sha256.Sum256([]byte(s)); return hex.EncodeToString(sum[:]) }
func hashPassword(password string) string {
	salt := randomID()
	key, _ := pbkdf2.Key(sha256.New, password, []byte(salt), 600000, 32)
	return salt + ":" + hex.EncodeToString(key)
}
func passwordMatches(hash, password string) bool {
	parts := strings.Split(hash, ":")
	if len(parts) != 2 {
		return false
	}
	key, _ := pbkdf2.Key(sha256.New, password, []byte(parts[0]), 600000, 32)
	expected, err := hex.DecodeString(parts[1])
	return err == nil && subtle.ConstantTimeCompare(key, expected) == 1
}
func parse(raw json.RawMessage, dst any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return failure(400, "invalid_request", "Invalid request fields or JSON")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return failure(400, "invalid_request", "Expected one JSON object")
	}
	return nil
}

func Initialize(ctx context.Context, db *store.Store) (*model.State, error) {
	err := db.Update(ctx, func(st *model.State) (bool, error) {
		if st.Schema != 0 {
			return false, nil
		}
		*st = model.State{Schema: model.SchemaVersion, InstallationID: randomID(), Accounts: map[string]model.Account{}, Sessions: map[string]model.Session{}, Workflows: model.SeedWorkflows()}
		st.Accounts["admin"] = model.Account{ID: "admin", Username: "admin", PasswordHash: hashPassword("password"), Administrator: true}
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return db.Read(ctx)
}

type Service struct {
	db           *store.Store
	mu           sync.Mutex
	failures     int
	blockedUntil time.Time
}

func New(db *store.Store) *Service { return &Service{db: db} }

type LoginResult struct {
	Token   string        `json:"token"`
	Account model.Account `json:"account"`
}

func actor(st *model.State, token string) (model.Account, error) {
	session, ok := st.Sessions[digest(token)]
	if !ok || token == "" || time.Now().After(session.Expires) {
		return model.Account{}, failure(401, "unauthenticated", "Sign in to continue")
	}
	account, ok := st.Accounts[session.AccountID]
	if !ok || account.Suspended {
		return model.Account{}, failure(401, "unauthenticated", "Sign in to continue")
	}
	return account, nil
}
func publicAccount(a model.Account) model.Account { a.PasswordHash = ""; return a }

func (s *Service) Call(ctx context.Context, token, operation string, raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	if operation == "login" {
		return s.login(ctx, raw)
	}
	if operation == "getSession" || operation == "logout" || operation == "listWorkflows" {
		if err := parse(raw, &struct{}{}); err != nil {
			return nil, err
		}
	}
	var result any
	err := s.db.Update(ctx, func(st *model.State) (bool, error) {
		a, err := actor(st, token)
		if err != nil {
			return false, err
		}
		switch operation {
		case "getSession":
			result = map[string]any{"account": publicAccount(a), "installationId": st.InstallationID, "revision": st.Revision, "settings": st.Settings}
			return false, nil
		case "logout":
			delete(st.Sessions, digest(token))
			result = map[string]bool{"signedOut": true}
			return true, nil
		case "listWorkflows":
			result = map[string]any{"items": []model.Workflow{st.Workflows["human@1"], st.Workflows["agent@1"]}, "nextCursor": ""}
			return false, nil
		default:
			return false, failure(404, "unsupported_operation", "Operation is not available in this build")
		}
	})
	return result, err
}
func (s *Service) login(ctx context.Context, raw json.RawMessage) (any, error) {
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := parse(raw, &input); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Now().Before(s.blockedUntil) {
		return nil, failure(429, "rate_limited", "Too many sign-in attempts; retry shortly")
	}
	var result LoginResult
	err := s.db.Update(ctx, func(st *model.State) (bool, error) {
		a, ok := st.Accounts[input.Username]
		hash := a.PasswordHash
		if !ok {
			hash = st.Accounts["admin"].PasswordHash
		}
		if !passwordMatches(hash, input.Password) || !ok || a.Suspended {
			return false, failure(401, "invalid_credentials", "Invalid username or password")
		}
		token := randomID()
		st.Sessions[digest(token)] = model.Session{AccountID: a.ID, Expires: time.Now().UTC().Add(12 * time.Hour)}
		result = LoginResult{token, publicAccount(a)}
		return true, nil
	})
	if err != nil {
		var ae *Error
		if errors.As(err, &ae) && ae.Status == 401 {
			s.failures++
			if s.failures >= 10 {
				s.blockedUntil = time.Now().Add(30 * time.Second)
			}
		}
		return nil, err
	}
	s.failures = 0
	return result, nil
}

func ErrorResponse(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return &Error{500, "internal_error", "Operation failed; inspect server logs"}
}
func (s *Service) String() string { return fmt.Sprintf("teleport service") }
