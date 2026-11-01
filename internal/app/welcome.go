package app

import (
	"encoding/json"
	"os/exec"
	"strings"

	"teleport/internal/model"
)

type HarnessChoice struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Available bool   `json:"available"`
}
type WelcomeSetupResult struct {
	Setup             model.WelcomeSetup `json:"setup"`
	CanCreateProject  bool               `json:"canCreateProject"`
	CanBindRepository bool               `json:"canBindRepository"`
	Harnesses         []HarnessChoice    `json:"harnesses"`
}
type SaveWelcomeSetupInput struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	Step            int    `json:"step"`
	ProjectID       int64  `json:"projectId"`
	ProjectName     string `json:"projectName"`
	ProjectKey      string `json:"projectKey"`
	Repository      string `json:"repository"`
	StorageRoot     string `json:"storageRoot"`
	Harness         string `json:"harness"`
	Attention       string `json:"attention"`
}
type CompleteWelcomeSetupInput struct {
	ExpectedVersion int64 `json:"expectedVersion"`
	Confirm         bool  `json:"confirm"`
}

func welcomeSetup(st *model.State, a model.Account) model.WelcomeSetup {
	w, ok := st.WelcomeSetups[a.ID]
	if !ok {
		w = model.WelcomeSetup{Harness: "codex", Attention: "questions"}
	}
	// A private draft never keeps access to a project after rights loss.
	if w.ProjectID != 0 && role(st, a, w.ProjectID) == "" {
		w.ProjectID, w.Repository, w.StorageRoot = 0, "", ""
		w.ProjectName, w.ProjectKey = "", ""
	}
	return w
}

func welcomeOperations(st *model.State, a model.Account, operation string, raw json.RawMessage) (any, bool, error) {
	w := welcomeSetup(st, a)
	switch operation {
	case "getWelcomeSetup":
		if err := parse(raw, &struct{}{}); err != nil {
			return nil, false, err
		}
		_, err := exec.LookPath("codex")
		return WelcomeSetupResult{Setup: w, CanCreateProject: true, CanBindRepository: a.Administrator && len(st.Accounts) == 1, Harnesses: []HarnessChoice{{ID: "codex", Title: "Codex", Available: err == nil}}}, false, nil
	case "saveWelcomeSetup":
		var in SaveWelcomeSetupInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		if w.Completed || in.ExpectedVersion != w.Version {
			return nil, false, failure(409, "welcome_conflict", "Setup changed; reopen to load the current progress")
		}
		if in.Step < 0 || in.Step > 5 || len(in.ProjectName) > 200 || len(in.ProjectKey) > 80 || len(in.Repository) > 4096 || len(in.StorageRoot) > 4096 || !contains([]string{"", "codex"}, in.Harness) || !contains([]string{"questions", "all", "quiet"}, in.Attention) {
			return nil, false, failure(422, "invalid_setup", "Choose supported setup values within the field limits")
		}
		for _, v := range []string{in.ProjectName, in.ProjectKey, in.Repository, in.StorageRoot} {
			if strings.ContainsRune(v, 0) {
				return nil, false, failure(422, "invalid_setup", "Setup fields cannot contain NUL")
			}
		}
		if in.ProjectID != 0 {
			if err := require(st, a, in.ProjectID, "Viewer"); err != nil {
				return nil, false, err
			}
		}
		if w.ID == "" {
			w.ID = randomID()[:32]
		}
		w.Version++
		w.Step, w.ProjectID = in.Step, in.ProjectID
		w.ProjectName, w.ProjectKey = in.ProjectName, in.ProjectKey
		w.Repository, w.StorageRoot = in.Repository, in.StorageRoot
		w.Harness, w.Attention = in.Harness, in.Attention
	case "completeWelcomeSetup":
		var in CompleteWelcomeSetupInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		if !in.Confirm {
			return nil, false, failure(422, "confirmation_required", "Explicitly finish the displayed setup")
		}
		if w.Completed {
			return w, false, nil
		}
		if in.ExpectedVersion != w.Version || w.Step != 5 || w.ProjectID == 0 {
			return nil, false, failure(409, "welcome_conflict", "Select a project and review the current setup before completion")
		}
		if err := require(st, a, w.ProjectID, "Viewer"); err != nil {
			return nil, false, err
		}
		if w.Repository != "" || w.StorageRoot != "" {
			binding := st.Projects[w.ProjectID].Repository
			if binding == nil || binding.Repository != w.Repository || binding.StorageRoot != w.StorageRoot {
				return nil, false, failure(409, "binding_required", "Grant the displayed repository roots or return to skip repository setup")
			}
		}
		w.Completed = true
		w.Version++
	default:
		return nil, false, failure(404, "unsupported_operation", "Operation is not available in this build")
	}
	if st.WelcomeSetups == nil {
		st.WelcomeSetups = map[string]model.WelcomeSetup{}
	}
	st.WelcomeSetups[a.ID] = w
	return w, true, nil
}
