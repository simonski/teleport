package app

import (
	"os"
	"path/filepath"
	"strings"
)

const defaultProjectRoot = "~/code/teleport"

func validProjectRoot(root string) bool {
	return len(root) > 0 && len(root) <= 4096 && !strings.ContainsRune(root, 0) && (filepath.IsAbs(root) || strings.HasPrefix(root, "~/"))
}
func expandProjectRoot(root string) (string, error) {
	if strings.HasPrefix(root, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, strings.TrimPrefix(root, "~/")), nil
	}
	return root, nil
}
