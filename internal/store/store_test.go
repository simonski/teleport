package store_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	"teleport/internal/app"
	"teleport/internal/model"
	"teleport/internal/store"
	"teleport/internal/teststore"
)

func TestOwnershipRollbackAndReopen(t *testing.T) {
	location := teststore.Location(t)
	db, err := store.Open(location, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	initial, err := app.Initialize(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := store.Open(location, false); err == nil {
		second.Close()
		t.Fatal("a second server acquired the same installation")
	}
	rejected := errors.New("reject entire transaction")
	if err := db.Update(ctx, func(st *model.State) (bool, error) {
		st.InstallationID = "discarded"
		st.NextTicketID = 500
		return true, rejected
	}); !errors.Is(err, rejected) {
		t.Fatal(err)
	}
	unchanged, err := db.Read(ctx)
	if err != nil || unchanged.InstallationID != initial.InstallationID || unchanged.NextTicketID != initial.NextTicketID || unchanged.Revision != initial.Revision {
		t.Fatal("rollback changed persistent state", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = db.Read(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled reads were misreported as uninitialized", err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := store.Open(location, false)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	retained, err := reopened.Read(ctx)
	if err != nil || retained.InstallationID != initial.InstallationID {
		t.Fatal("reopen lost identity", err)
	}
	if err = reopened.Update(ctx, func(st *model.State) (bool, error) { st.Schema = 999; return true, nil }); err != nil {
		t.Fatal(err)
	}
	if _, err = reopened.Read(ctx); err == nil {
		t.Fatal("unsupported schema was accepted")
	}
}

func TestPostgresLostOwnershipFailsClosed(t *testing.T) {
	if os.Getenv("TELEPORT_TEST_POSTGRES") == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	location := teststore.Location(t)
	db, err := store.Open(location, true)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if _, err = app.Initialize(ctx, db); err != nil {
		t.Fatal(err)
	}
	admin, err := sql.Open("pgx", location)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	var terminated bool
	err = admin.QueryRowContext(ctx, "SELECT pg_terminate_backend(pid) FROM pg_locks WHERE locktype='advisory' AND objid=1949675634 AND database=(SELECT oid FROM pg_database WHERE datname=current_database())").Scan(&terminated)
	if err != nil || !terminated {
		t.Fatal("could not reproduce ownership connection loss", err)
	}
	if _, err = db.Read(ctx); err == nil {
		t.Fatal("read silently reconnected after losing ownership")
	}
	if err = db.Update(ctx, func(st *model.State) (bool, error) { st.Revision++; return true, nil }); err == nil {
		t.Fatal("write proceeded after losing ownership")
	}
	replacement, err := store.Open(location, false)
	if err != nil {
		t.Fatal("released installation could not be recovered", err)
	}
	defer replacement.Close()
	if _, err = replacement.Read(ctx); err != nil {
		t.Fatal(err)
	}
}
