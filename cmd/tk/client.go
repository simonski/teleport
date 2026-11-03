package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"teleport/internal/app"
)

func readCredentials(endpoint, path string) (credentials, error) {
	creds := credentials{Server: endpoint}
	body, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return creds, nil
	}
	if err != nil {
		return creds, errors.New("cannot read client credentials; check -config")
	}
	if err := json.Unmarshal(body, &creds); err != nil {
		return creds, errors.New("invalid client credential JSON; inspect the file selected by -config")
	}
	if creds.Server != endpoint {
		return creds, errors.New("credential server differs; sign in for this server")
	}
	return creds, nil
}

func requestOperation(endpoint string, creds credentials, operation string, data []byte) ([]byte, error) {
	if len(data) > 1<<20 {
		return nil, errors.New("complete JSON request exceeds 1 MiB")
	}
	if !json.Valid(data) {
		return nil, errors.New("-data must be valid JSON")
	}
	req, err := http.NewRequest("POST", endpoint+"/api/tools/"+operation, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if creds.Token != "" {
		req.Header.Set("Authorization", "Bearer "+creds.Token)
	}
	client := http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("server unavailable; run tk serve (no database fallback): %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(body) > 4<<20 {
		return nil, errors.New("response exceeds 4 MiB; reduce the page limit and retry")
	}
	if resp.StatusCode >= 400 {
		var result struct {
			Error app.Error `json:"error"`
		}
		json.Unmarshal(body, &result)
		return nil, fmt.Errorf("%s: %s", resp.Status, result.Error.Message)
	}
	return body, nil
}
