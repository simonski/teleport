package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type InstalledSkillInput struct {
	Name         string `json:"name"`
	Content      string `json:"content,omitempty"`
	ExpectedHash string `json:"expectedHash,omitempty"`
}
type InstalledSkillView struct {
	Name     string `json:"name"`
	Content  string `json:"content"`
	Hash     string `json:"hash"`
	Location string `json:"location"`
}

func (s *Service) installedSkillCall(ctx context.Context, token, op string, raw json.RawMessage) (any, error) {
	var in InstalledSkillInput
	if err := parse(raw, &in); err != nil {
		return nil, err
	}
	if !agentSkillName.MatchString(in.Name) || in.Name == "teleport" && op == "saveInstalledSkill" {
		return nil, failure(422, "invalid_skill", "Choose a valid skill name; the bundled teleport skill is managed by Teleport")
	}
	s.skillMu.Lock()
	defer s.skillMu.Unlock()
	st, err := s.db.Read(ctx)
	if err != nil {
		return nil, err
	}
	a, err := actor(st, token)
	if err != nil {
		return nil, err
	}
	if !a.Administrator || a.ServiceRunID != "" || a.ServiceSessionID != "" {
		return nil, failure(403, "administrator_required", "Skill curation requires an administrator")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, failure(503, "skills_unavailable", "Skill directory unavailable")
	}
	root, err := os.OpenRoot(home)
	if err != nil {
		return nil, failure(503, "skills_unavailable", "Skill directory unavailable")
	}
	defer root.Close()
	roots := []string{".agents/skills", ".codex/skills", ".claude/skills"}
	selected := ""
	for _, base := range roots {
		info, e := root.Lstat(filepath.Join(base, in.Name, "SKILL.md"))
		if e == nil && info.Mode().IsRegular() {
			selected = base
			break
		}
	}
	if op == "getInstalledSkill" && selected == "" {
		return nil, failure(404, "not_found", "Installed skill not found")
	}
	if selected == "" {
		selected = roots[0]
	}
	for _, component := range []string{strings.Split(selected, "/")[0], selected} {
		info, e := root.Lstat(component)
		if e == nil && !info.IsDir() {
			return nil, failure(409, "skill_unavailable", "Skill root is not a regular directory")
		}
		if e != nil && !os.IsNotExist(e) {
			return nil, failure(409, "skill_unavailable", "Skill root cannot be inspected")
		}
	}
	name := filepath.Join(selected, in.Name, "SKILL.md")
	if info, e := root.Lstat(filepath.Join(selected, in.Name)); e == nil && !info.IsDir() {
		return nil, failure(409, "skill_unavailable", "Skill directory is not a regular directory")
	}
	if info, e := root.Lstat(name); e == nil {
		if !info.Mode().IsRegular() {
			return nil, failure(409, "skill_unavailable", "Skill is not a regular file")
		}
		if info.Size() > 128<<10 {
			return nil, failure(409, "skill_too_large", "Skill exceeds the size limit")
		}
	}
	old, readErr := root.ReadFile(name)
	if readErr != nil && !os.IsNotExist(readErr) {
		return nil, failure(409, "skill_unavailable", "Skill cannot be read")
	}
	if len(old) > 128<<10 {
		return nil, failure(409, "skill_too_large", "Skill exceeds the size limit")
	}
	if op == "getInstalledSkill" {
		return InstalledSkillView{Name: in.Name, Content: string(old), Hash: digest(string(old)), Location: selected}, nil
	}
	if !utf8.ValidString(in.Content) || len(in.Content) > 128<<10 || strings.ContainsRune(in.Content, 0) || !strings.HasPrefix(in.Content, "---\n") || !strings.Contains(in.Content, "\n---\n") {
		return nil, failure(422, "invalid_skill", "Use bounded UTF-8 SKILL.md text with YAML frontmatter")
	}
	front := strings.SplitN(in.Content, "\n---\n", 2)[0]
	matchingName, hasDescription := false, false
	for _, line := range strings.Split(front, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if key == "name" && strings.Trim(value, `"'`) == in.Name {
			matchingName = true
		}
		if key == "description" && value != "" {
			hasDescription = true
		}
	}
	if !matchingName || !hasDescription {
		return nil, failure(422, "invalid_skill", "Frontmatter must contain the matching name and a description")
	}
	if readErr == nil && in.ExpectedHash != digest(string(old)) {
		if string(old) == in.Content {
			return InstalledSkillView{Name: in.Name, Content: in.Content, Hash: digest(in.Content), Location: selected}, nil
		}
		return nil, failure(409, "version_conflict", "Skill changed; reload before saving")
	}
	if os.IsNotExist(readErr) && in.ExpectedHash != "" {
		return nil, failure(409, "version_conflict", "Skill no longer exists")
	}
	if err = root.MkdirAll(filepath.Join(selected, in.Name), 0700); err != nil {
		return nil, failure(409, "skill_unavailable", "Skill directory cannot be created")
	}
	dir := filepath.Join(selected, in.Name)
	info, err := root.Lstat(dir)
	if err != nil || !info.IsDir() {
		return nil, failure(409, "skill_unavailable", "Skill directory is not a regular directory")
	}
	temp := filepath.Join(dir, ".skill-"+randomID())
	f, err := root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, failure(503, "skill_save_failed", "Skill could not be saved")
	}
	_, err = f.WriteString(in.Content)
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err == nil && readErr == nil {
		latest, e := root.ReadFile(name)
		if e != nil || digest(string(latest)) != digest(string(old)) {
			_ = root.Remove(temp)
			return nil, failure(409, "version_conflict", "Skill changed; reload before saving")
		}
	}
	if err == nil {
		err = root.Rename(temp, name)
	}
	if err != nil {
		_ = root.Remove(temp)
		return nil, failure(503, "skill_save_failed", "Skill could not be saved")
	}
	return InstalledSkillView{Name: in.Name, Content: in.Content, Hash: digest(in.Content), Location: selected}, nil
}
