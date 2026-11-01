package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	teleportskill "teleport/internal/skills/teleport"
)

func TestSkillExportsOfflineWithoutHomeServerOrCredentials(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("TELEPORT_SERVER", "http://127.0.0.1:1")
	t.Setenv("TELEPORT_CONFIG", filepath.Join(t.TempDir(), "absent", "credentials.json"))
	for _, args := range [][]string{{"skill"}, {"skill", "--server", "http://127.0.0.1:1", "--config", "absent.json"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "SKILL.md")
			output, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			original := os.Stdout
			t.Cleanup(func() { os.Stdout = original })
			os.Stdout = output
			err = run(args)
			os.Stdout = original
			if closeErr := output.Close(); closeErr != nil {
				t.Fatal(closeErr)
			}
			if err != nil {
				t.Fatal(err)
			}
			content, err := os.ReadFile(path)
			if err != nil || string(content) != teleportskill.Markdown {
				t.Fatalf("export differs from bundled skill: %v", err)
			}
		})
	}
}

func TestSkillRejectsExtraArguments(t *testing.T) {
	if err := run([]string{"skill", "unexpected"}); err == nil {
		t.Fatal("unexpected positional argument accepted")
	}
}
