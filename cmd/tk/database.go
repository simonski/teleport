package main

import (
	"encoding/json"
	"errors"
	"os"
)

func databaseLocation(explicit, config string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	info, err := os.Lstat(config)
	if err != nil {
		return "", errors.New("configure PostgreSQL with TELEPORT_DB or -db-config; run make db-up for local setup")
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return "", errors.New("database configuration must be an owner-only regular file")
	}
	body, err := os.ReadFile(config)
	if err != nil {
		return "", errors.New("could not read database configuration")
	}
	var value struct {
		URL string `json:"url"`
	}
	if json.Unmarshal(body, &value) != nil || value.URL == "" {
		return "", errors.New("database configuration must contain a PostgreSQL url")
	}
	return value.URL, nil
}
