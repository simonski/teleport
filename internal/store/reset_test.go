package store_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"teleport/internal/app"
	"teleport/internal/model"
	"teleport/internal/store"
)

func TestResetRetainsRecoverableSQLiteBackup(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "original.sqlite")
	db, err := store.Open(path, true)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	before, err := app.Initialize(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Update(ctx, func(st *model.State) (bool, error) { st.NextTicketID = 123; return true, nil }); err != nil {
		t.Fatal(err)
	}
	fresh := &model.State{Schema: model.SchemaVersion, InstallationID: "replacement"}
	backup, err := db.Reset(ctx, fresh)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(backup)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("backup is not private", err)
	}
	recovery, err := store.Open(backup, false)
	if err != nil {
		t.Fatal(err)
	}
	defer recovery.Close()
	recovered, err := recovery.Read(ctx)
	if err != nil || recovered.InstallationID != before.InstallationID || recovered.NextTicketID != 123 {
		t.Fatal("backup omitted committed state", err)
	}
	after, err := db.Read(ctx)
	if err != nil || after.InstallationID != "replacement" || after.NextTicketID != 0 {
		t.Fatal("replacement failed", err)
	}
}

func TestResetRefusesOwnedJobsAndRetainsState(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "owned.sqlite")
	db, err := store.Open(path, true)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	before, err := app.Initialize(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Update(ctx, func(st *model.State) (bool, error) {
		st.ScriptRuns = map[string]model.ScriptRun{"J1": {State: "unknown"}}
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Reset(ctx, &model.State{Schema: model.SchemaVersion, InstallationID: "replacement"}); err == nil {
		t.Fatal("reset orphaned an unresolved owned job")
	}
	after, err := db.Read(ctx)
	if err != nil || after.InstallationID != before.InstallationID {
		t.Fatal("refused reset changed state", err)
	}
	backups, _ := filepath.Glob(path + ".backup-*")
	if len(backups) != 0 {
		t.Fatal("refused reset created a backup")
	}
}

func TestBackupFailureCannotResetSQLite(t *testing.T) {
	ctx := context.Background()
	directory := filepath.Join(t.TempDir(), "current")
	db, err := store.Open(filepath.Join(directory, "data.sqlite"), true)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	before, err := app.Initialize(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(directory, directory+".moved"); err != nil {
		t.Fatal(err)
	}
	defer os.Rename(directory+".moved", directory)
	backup, err := db.Reset(ctx, &model.State{Schema: model.SchemaVersion, InstallationID: "replacement"})
	if err == nil || backup != "" {
		t.Fatal("reset ignored unavailable backup destination", err)
	}
	after, err := db.Read(ctx)
	if err != nil || after.InstallationID != before.InstallationID {
		t.Fatal("failed backup changed installation", err)
	}
}
