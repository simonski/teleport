package app_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"teleport/internal/app"
	"teleport/internal/model"
	"teleport/internal/store"
	"teleport/internal/teststore"
)

// Cache only serialized bootstrap data. Every test keeps its own SQL database,
// maps, installation identity, service and authenticated session. Production
// initialization/password verification remain covered by authentication tests.
var fixtureSeed struct {
	sync.Once
	data []byte
	err  error
}

func fixture(t *testing.T) (*app.Service, string) {
	t.Helper()
	svc, token, _ := fixtureWithStore(t)
	return svc, token
}

func fixtureWithStore(t *testing.T) (*app.Service, string, *store.Store) {
	t.Helper()
	ctx := context.Background()
	db := teststore.Open(t)
	fixtureSeed.Do(func() {
		var state *model.State
		state, fixtureSeed.err = app.Initialize(ctx, db)
		if fixtureSeed.err == nil {
			fixtureSeed.data, fixtureSeed.err = json.Marshal(state)
		}
	})
	if fixtureSeed.err != nil {
		t.Fatal(fixtureSeed.err)
	}
	var state model.State
	if err := json.Unmarshal(fixtureSeed.data, &state); err != nil {
		t.Fatal(err)
	}
	id := func() string {
		var bytes [32]byte
		if _, err := rand.Read(bytes[:]); err != nil {
			t.Fatal(err)
		}
		return hex.EncodeToString(bytes[:])
	}
	state.InstallationID = id()
	token := id()
	hash := sha256.Sum256([]byte(token))
	state.Sessions = map[string]model.Session{hex.EncodeToString(hash[:]): {AccountID: "admin", Expires: time.Now().Add(12 * time.Hour)}}
	if err := db.Update(ctx, func(current *model.State) (bool, error) {
		*current = state
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	return app.New(db), token, db
}

func TestFixtureSeedDoesNotShareIdentitySessionsOrWork(t *testing.T) {
	for index := 0; index < 4; index++ {
		t.Run(string(rune('A'+index)), func(t *testing.T) {
			t.Parallel()
			first, token, firstDB := fixtureWithStore(t)
			second, other, secondDB := fixtureWithStore(t)
			before, err := firstDB.Read(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			after, err := secondDB.Read(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if before.InstallationID == after.InstallationID || token == other {
				t.Fatal("fixture reused installation or session identity")
			}
			if _, err = second.Call(context.Background(), token, "getSession", []byte(`{}`)); err == nil {
				t.Fatal("another fixture's session was accepted")
			}
			call(t, first, token, "createProject", app.CreateProjectInput{Name: "Independent", RequestID: "fixture-project"})
			if page := call(t, second, other, "listProjects", struct{}{}).(app.Page[model.Project]); len(page.Items) != 0 {
				t.Fatal("fixture project escaped its database")
			}
		})
	}
}
