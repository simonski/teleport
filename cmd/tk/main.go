package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"teleport/internal/app"
	"teleport/internal/server"
	"teleport/internal/store"
)

var version = "0.1.0-dev"

type credentials struct {
	Server string `json:"server"`
	Token  string `json:"token"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "tk:", err)
		os.Exit(1)
	}
}
func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func run(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" {
		fmt.Println("teleport (tk)\n\n  tk init [--db PATH]              Initialize storage; start nothing\n  tk serve [--db PATH] [--addr localhost:9999]\n  tk login [--username admin] --password-stdin\n  tk OPERATION --data JSON         Call the authenticated server API\n  tk tools | spec | skill          Read server discovery\n  tk version\n\nEnvironment: TELEPORT_DB, TELEPORT_SERVER, TELEPORT_CONFIG")
		return nil
	}
	command := args[0]
	if command == "version" {
		fmt.Println(version)
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	dbPath := flags.String("db", env("TELEPORT_DB", filepath.Join(home, ".teleport", "database.db")), "SQLite path or PostgreSQL URL (init/serve only)")
	addr := flags.String("addr", "localhost:9999", "listen address")
	endpoint := flags.String("server", env("TELEPORT_SERVER", "http://localhost:9999"), "server URL")
	config := flags.String("config", env("TELEPORT_CONFIG", filepath.Join(home, ".teleport", "client.json")), "client credential file")
	data := flags.String("data", "{}", "JSON request")
	username := flags.String("username", "admin", "local username")
	passwordStdin := flags.Bool("password-stdin", false, "read password from standard input")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected arguments; use --data JSON")
	}
	ctx := context.Background()
	if command == "init" || command == "serve" {
		db, err := store.Open(*dbPath, command == "init")
		if err != nil {
			return err
		}
		defer db.Close()
		if command == "init" {
			if _, err = app.Initialize(ctx, db); err != nil {
				return err
			}
			fmt.Printf("Storage: %s\nInitialization complete. Run: tk serve\n", redactLocation(*dbPath))
			return nil
		}
		if _, err = db.Read(ctx); err != nil {
			return err
		}
		httpServer := server.HTTP(*addr, server.New(app.New(db), version))
		stopCtx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
		defer cancel()
		go func() {
			<-stopCtx.Done()
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			httpServer.Shutdown(shutdown)
		}()
		slog.Info("teleport listening", "address", *addr, "version", version, "agents", "off")
		err = httpServer.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
	*endpoint = strings.TrimRight(*endpoint, "/")
	if command == "spec" || command == "tools" || command == "skill" {
		path := map[string]string{"spec": "/openapi.json", "tools": "/api/actions", "skill": "/skill.md"}[command]
		resp, err := http.Get(*endpoint + path)
		if err != nil {
			return fmt.Errorf("server unavailable; run tk serve: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("server returned %s", resp.Status)
		}
		_, err = io.Copy(os.Stdout, io.LimitReader(resp.Body, 4<<20))
		return err
	}
	creds := credentials{Server: *endpoint}
	if b, err := os.ReadFile(*config); err == nil {
		json.Unmarshal(b, &creds)
		if creds.Server != *endpoint {
			return errors.New("credential server differs; sign in for this server")
		}
	}
	if command == "login" {
		if !*passwordStdin {
			return errors.New("use --password-stdin to read a password without exposing it in process arguments")
		}
		b, err := io.ReadAll(io.LimitReader(os.Stdin, 1025))
		if err != nil {
			return err
		}
		if len(b) > 1024 {
			return errors.New("password is too long")
		}
		encoded, _ := json.Marshal(map[string]string{"username": *username, "password": strings.TrimRight(string(b), "\r\n")})
		*data = string(encoded)
	}
	if !json.Valid([]byte(*data)) {
		return errors.New("--data must be valid JSON")
	}
	req, err := http.NewRequest("POST", *endpoint+"/api/tools/"+command, bytes.NewBufferString(*data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if creds.Token != "" {
		req.Header.Set("Authorization", "Bearer "+creds.Token)
	}
	client := http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("server unavailable; run tk serve (no database fallback): %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		var result struct {
			Error app.Error `json:"error"`
		}
		json.Unmarshal(body, &result)
		return fmt.Errorf("%s: %s", resp.Status, result.Error.Message)
	}
	if command == "login" {
		var login app.LoginResult
		if err = json.Unmarshal(body, &login); err != nil {
			return err
		}
		creds = credentials{*endpoint, login.Token}
		if err = saveCredentials(*config, creds); err != nil {
			return err
		}
		fmt.Println("Signed in as", login.Account.Username)
		return nil
	}
	if command == "logout" {
		if err = os.Remove(*config); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	var pretty bytes.Buffer
	if json.Indent(&pretty, body, "", "  ") == nil {
		fmt.Println(pretty.String())
	} else {
		fmt.Print(string(body))
	}
	return nil
}
func redactLocation(location string) string {
	if strings.HasPrefix(location, "postgres") {
		return "PostgreSQL (configured instance)"
	}
	return location
}
func saveCredentials(path string, c credentials) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".client-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		err = json.NewEncoder(f).Encode(c)
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}
