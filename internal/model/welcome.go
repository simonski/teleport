package model

// WelcomeSetup is private account progress, never execution authority.
type WelcomeSetup struct {
	ID          string `json:"id"`
	Version     int64  `json:"version"`
	Completed   bool   `json:"completed"`
	Step        int    `json:"step"`
	ProjectID   int64  `json:"projectId"`
	ProjectName string `json:"projectName"`
	ProjectKey  string `json:"projectKey"`
	Repository  string `json:"repository"`
	StorageRoot string `json:"storageRoot"`
	Harness     string `json:"harness"`
	Attention   string `json:"attention"`
}
