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

	"teleport/internal/localruntime"
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
	st, _, err := InitializeWithOptions(ctx, db, InitializeOptions{})
	return st, err
}

type InitializeOptions struct {
	AdminPassword string
}

func freshInstallation(adminPassword string) model.State {
	if adminPassword == "" {
		adminPassword = "password"
	}
	st := model.State{Schema: model.SchemaVersion, InstallationID: randomID(), Accounts: map[string]model.Account{}, Sessions: map[string]model.Session{}, Workflows: model.SeedWorkflows()}
	st.Accounts["admin"] = model.Account{ID: "admin", Username: "admin", PasswordHash: hashPassword(adminPassword), Administrator: true}
	return st
}

func InitializeWithOptions(ctx context.Context, db *store.Store, options InitializeOptions) (*model.State, string, error) {
	if options.AdminPassword != "" && (len(options.AdminPassword) < 12 || len(options.AdminPassword) > 1024) {
		return nil, "", failure(422, "invalid_password", "Use an initial password between 12 and 1,024 bytes")
	}
	err := db.Update(ctx, func(st *model.State) (bool, error) {
		changed := st.Schema == 0
		if changed {
			*st = freshInstallation(options.AdminPassword)
		} else if options.AdminPassword != "" && !passwordMatches(st.Accounts["admin"].PasswordHash, options.AdminPassword) {
			return false, failure(409, "initial_password_mismatch", "Existing admin credentials differ; initialization never resets a password")
		}
		return changed, nil
	})
	if err != nil {
		return nil, "", err
	}
	st, err := db.Read(ctx)
	return st, "", err
}

type Service struct {
	skillMu        sync.Mutex
	scriptMu       sync.Mutex
	localEndpoint  string
	localMu        sync.Mutex
	nativeMu       sync.Mutex
	localClient    *localruntime.Client
	fixtureHarness bool
	controllerID   string
	gitMu          sync.Mutex
	db             *store.Store
	mu             sync.Mutex
	failures       int
	blockedUntil   time.Time
}

func New(db *store.Store) *Service { return &Service{db: db, controllerID: randomID()} }

type LoginResult struct {
	Token   string        `json:"token"`
	Account model.Account `json:"account"`
}

func actor(st *model.State, token string) (model.Account, error) {
	if scoped, ok := scriptActor(st, token); ok {
		return scoped, nil
	}
	if scoped, ok := localActor(st, token); ok {
		return scoped, nil
	}
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
	if scriptOperation(operation) {
		return s.scriptCall(ctx, token, operation, raw)
	}
	if token != "" {
		snapshot, err := s.db.Read(ctx)
		if err != nil {
			return nil, err
		}
		if scoped, ok := scriptActor(snapshot, token); ok {
			if operation == "login" {
				return nil, failure(403, "script_scope", "Script credentials cannot sign in as a human")
			}
			if err := checkScriptDispatch(ctx, snapshot, &scoped, operation, raw); err != nil {
				return nil, err
			}
		}
	}
	if operation == "login" {
		if token != "" {
			snapshot, err := s.db.Read(ctx)
			if err != nil {
				return nil, err
			}
			if _, ok := localActor(snapshot, token); ok {
				return nil, failure(403, "agent_scope", "Managed context credentials cannot sign in as a human")
			}
		}
		return s.login(ctx, raw)
	}
	if operation == "listProjectSummaries" {
		return s.projectDirectory(ctx, token, raw)
	}
	if operation == "getHome" {
		return s.home(ctx, token, raw)
	}
	if operation == "getUsageSummary" {
		return s.usageSummary(ctx, token, raw)
	}
	if operation == "listUsageBreakdown" {
		return s.usageBreakdown(ctx, token, raw)
	}
	if operation == "getUsageAttribution" {
		return s.usageAttribution(ctx, token, raw)
	}
	if operation == "listUsagePrices" || operation == "publishUsagePrice" || operation == "listUsagePriceHistory" || operation == "listUsageValuations" {
		return s.usagePriceCall(ctx, token, operation, raw)
	}
	if operation == "getBoardExecution" {
		return s.boardExecution(ctx, token, raw)
	}
	if operation == "listHostHarnesses" || operation == "stopHostHarness" {
		return s.hostHarnessCall(ctx, token, operation, raw)
	}
	if operation == "getInstalledSkill" || operation == "saveInstalledSkill" {
		return s.installedSkillCall(ctx, token, operation, raw)
	}
	if contains([]string{"startSpellcheck", "getSpellcheckRun", "getTicketSpellchecks"}, operation) {
		return s.spellcheckCall(ctx, token, operation, raw)
	}
	if localOperation(operation) {
		return s.localCall(ctx, token, operation, raw)
	}
	if operation == "configureLocalRepository" || operation == "prepareTaskWorktree" || operation == "recordTaskDelivery" || operation == "approveTicket" || operation == "reconcileGitOperation" {
		return s.gitCall(ctx, token, operation, raw)
	}
	if operation == "getSession" || operation == "logout" {
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
		collections(st)
		if err := checkScriptDispatch(ctx, st, &a, operation, raw); err != nil {
			return false, err
		}
		if a.ServiceSessionID != "" && !contains([]string{"getTicket", "listTickets", "listProjects", "listWorkflows", "getWorkflow", "getTicketAgreements", "listTicketHistory", "submitTicketProposal", "getProposalRunContext", "reportProposalRunUsage"}, operation) {
			return false, failure(403, "agent_scope", "This managed credential permits scoped context reads only")
		}
		if a.ServiceSessionReadOnly && (operation == "submitTicketProposal" || operation == "reportProposalRunUsage") {
			return false, failure(403, "agent_scope", "This harness credential permits context reads only")
		}
		switch operation {
		case "getSession":
			body := map[string]any{"account": publicAccount(a), "installationId": st.InstallationID, "revision": st.Revision, "settings": st.Settings, "preferences": personalPreferences(st, a), "welcomeSetup": welcomeSetup(st, a)}
			if location := currentLocation(st, a); location != nil {
				body["lastLocation"] = location
			}
			body["contentRevision"] = st.Revision - st.LocationWrites
			result = body
			return false, nil
		case "saveCurrentLocation":
			var changed bool
			result, changed, err = saveCurrentLocation(st, a, raw)
			return changed, err
		case "logout":
			delete(st.Sessions, digest(token))
			result = map[string]bool{"signedOut": true}
			return true, nil
		case "listAgentDefinitions", "listInstalledAgentSkills", "saveAgentDefinition", "listProjectAgents", "getAgentInventory":
			var changed bool
			result, changed, err = agentFactoryCall(st, a, operation, raw)
			return changed, err
		case "listAgentUsages", "saveAgentUsage", "removeAgentUsage":
			var changed bool
			result, changed, err = agentUsageCall(st, a, operation, raw)
			return changed, err
		case "listTicketProposals", "submitTicketProposal", "commentTicketProposal", "acceptTicketProposal", "getProposalRunContext", "reportProposalRunUsage":
			var changed bool
			result, changed, err = ticketProposalOperations(st, a, operation, raw)
			return changed, err
		default:
			var changed bool
			result, changed, err = projectOperations(st, a, operation, raw)
			if changed && err == nil {
				reconcileReadyAttention(st, a)
			}
			return changed, err
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
