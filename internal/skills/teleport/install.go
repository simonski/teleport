package teleportskill

import (
	"errors"
	"os"
	"path/filepath"
)

// Install only an absent skill; an existing user-owned skill is never changed.
func EnsureInstalled(home string) error {
	if !filepath.IsAbs(home) {
		return errors.New("cannot locate an absolute home directory for the Teleport skill")
	}
	dir := filepath.Join(home, ".agents", "skills", "teleport")
	for _, path := range []string{filepath.Join(home, ".agents"), filepath.Join(home, ".agents", "skills"), dir} {
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			if err = os.Mkdir(path, 0700); err != nil && !os.IsExist(err) {
				return err
			}
			info, err = os.Lstat(path)
		}
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return errors.New("Teleport skill directory is not a direct directory")
		}
	}
	target := filepath.Join(dir, "SKILL.md")
	if info, err := os.Lstat(target); err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("existing Teleport skill is not a regular file")
		}
		data, err := os.ReadFile(target)
		if err != nil {
			return err
		}
		if string(data) != Markdown {
			return errors.New("existing Teleport skill differs; update it explicitly before starting")
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	f, err := os.CreateTemp(dir, ".teleport-skill-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.WriteString(Markdown)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Link(f.Name(), target); err != nil {
		if os.IsExist(err) {
			return EnsureInstalled(home)
		}
		return err
	}
	return nil
}
