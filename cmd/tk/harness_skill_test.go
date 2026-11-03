package main

import (
	"os"
	"path/filepath"
	"testing"

	teleportskill "teleport/internal/skills/teleport"
)

func TestHarnessSkillInstallationPreservesExistingFile(t *testing.T) {
	home := t.TempDir()
	if err := teleportskill.EnsureInstalled(home); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".agents", "skills", "teleport", "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil || string(data) != teleportskill.Markdown {
		t.Fatal("skill content differs", err)
	}
	if err = teleportskill.EnsureInstalled(home); err != nil {
		t.Fatal("identical skill should be reusable", err)
	}
	if err = os.WriteFile(path, []byte("user-owned skill"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = teleportskill.EnsureInstalled(home); err == nil {
		t.Fatal("different user-owned skill was accepted")
	}
	data, err = os.ReadFile(path)
	if err != nil || string(data) != "user-owned skill" {
		t.Fatal("user-owned skill was modified", err)
	}
}

func TestHarnessSkillRejectsSymlinkedDestination(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".agents", "skills")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	if err := os.Symlink(other, filepath.Join(root, "teleport")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := teleportskill.EnsureInstalled(home); err == nil {
		t.Fatal("symlinked destination was accepted")
	}
	if _, err := os.Stat(filepath.Join(other, "SKILL.md")); !os.IsNotExist(err) {
		t.Fatal("wrote through a symlink")
	}
}
