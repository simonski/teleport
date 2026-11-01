package server

import (
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"teleport/internal/app"
	teleportskill "teleport/internal/skills/teleport"
)

//go:embed web/*
var assets embed.FS

type Operation struct {
	Name    string `json:"name"`
	Action  string `json:"action"`
	Summary string `json:"summary"`
}

var Operations = []Operation{{"login", "ACT-1316", "Sign in with local credentials"}, {"getSession", "ACT-1309", "Read the current authenticated session"}, {"logout", "ACT-1310", "Revoke the current sign-in session"}, {"listWorkflows", "ACT-0701", "Read published workflow definitions"}}

func New(service *app.Service, version string, configuredHosts ...string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		write(w, 200, map[string]string{"status": "ok", "version": version})
	})
	mux.HandleFunc("GET /openapi.json", func(w http.ResponseWriter, r *http.Request) { write(w, 200, openAPI(version)) })
	mux.HandleFunc("GET /api/actions", func(w http.ResponseWriter, r *http.Request) { write(w, 200, map[string]any{"items": Operations}) })
	mux.HandleFunc("GET /skill.md", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		fmt.Fprint(w, teleportskill.Markdown)
	})
	mux.HandleFunc("POST /api/tools/{operation}", func(w http.ResponseWriter, r *http.Request) {
		mediaType, _, mediaErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if mediaErr != nil || mediaType != "application/json" {
			writeError(w, &app.Error{Status: 415, Code: "unsupported_media_type", Message: "Use application/json"})
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			scheme := "http"
			if r.TLS != nil {
				scheme = "https"
			}
			if err != nil || !strings.EqualFold(u.Host, r.Host) || u.Scheme != scheme || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
				writeError(w, &app.Error{Status: 403, Code: "forbidden_origin", Message: "Cross-origin requests are not permitted"})
				return
			}
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		defer r.Body.Close()
		var raw json.RawMessage
		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&raw); err != nil {
			status, code := 400, "invalid_request"
			if _, ok := err.(*http.MaxBytesError); ok {
				status, code = 413, "request_too_large"
			}
			writeError(w, &app.Error{Status: status, Code: code, Message: "Expected JSON request within 1 MiB"})
			return
		}
		if len(raw) == 0 || raw[0] != '{' {
			writeError(w, &app.Error{Status: 400, Code: "invalid_request", Message: "Expected a JSON object"})
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			status := 400
			if _, ok := err.(*http.MaxBytesError); ok {
				status = 413
			}
			writeError(w, &app.Error{Status: status, Code: "invalid_request", Message: "Expected one complete JSON object within 1 MiB"})
			return
		}
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == "" {
			if c, err := r.Cookie("teleport_session"); err == nil {
				token = c.Value
			}
		}
		op := r.PathValue("operation")
		result, err := service.Call(r.Context(), token, op, raw)
		if err != nil {
			e := app.ErrorResponse(err)
			if e.Status == 500 {
				slog.Error("tool call failed", "operation", op, "error", err)
			}
			if e.Status == 429 {
				w.Header().Set("Retry-After", "30")
			}
			writeError(w, e)
			return
		}
		if op == "login" {
			login := result.(app.LoginResult)
			http.SetCookie(w, &http.Cookie{Name: "teleport_session", Value: login.Token, Path: "/", HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode, MaxAge: 43200})
		}
		if op == "logout" {
			http.SetCookie(w, &http.Cookie{Name: "teleport_session", Value: "", Path: "/", HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode, MaxAge: -1})
		}
		write(w, 200, result)
	})
	web, _ := fs.Sub(assets, "web")
	index := func(w http.ResponseWriter, r *http.Request) {
		var entropy [24]byte
		if _, err := rand.Read(entropy[:]); err != nil {
			http.Error(w, "Page initialization failed", 500)
			return
		}
		nonce := base64.RawURLEncoding.EncodeToString(entropy[:])
		content, err := assets.ReadFile("web/index.html")
		if err != nil {
			http.Error(w, "Page unavailable", 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", contentPolicy(nonce))
		metadata := `<meta name="teleport-style-nonce" content="` + nonce + `"><meta name="teleport-build-version" content="` + html.EscapeString(version) + `"></head>`
		fmt.Fprint(w, strings.Replace(string(content), "</head>", metadata, 1))
	}
	mux.HandleFunc("GET /{$}", index)
	mux.HandleFunc("GET /index.html", index)
	mux.Handle("GET /", http.FileServer(http.FS(web)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", contentPolicy(""))
		w.Header().Set("Cache-Control", "no-store")
		if !trustedHost(r.Host, configuredHosts) {
			writeError(w, &app.Error{Status: 421, Code: "untrusted_host", Message: "Request Host is not configured for this server"})
			return
		}
		mux.ServeHTTP(w, r)
	})
}
func contentPolicy(nonce string) string {
	style := "style-src 'self'"
	if nonce != "" {
		style += " 'nonce-" + nonce + "'"
	}
	return "default-src 'self'; script-src 'self'; " + style + "; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'"
}
func trustedHost(host string, configured []string) bool {
	if hostname, _, err := net.SplitHostPort(host); err == nil {
		host = hostname
	}
	host = strings.TrimSuffix(strings.ToLower(strings.Trim(host, "[]")), ".")
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return true
	}
	for _, allowed := range configured {
		if allowed != "" && host == strings.ToLower(strings.TrimSuffix(allowed, ".")) {
			return true
		}
	}
	return false
}
func write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, e *app.Error) { write(w, e.Status, map[string]any{"error": e}) }
func HTTP(addr string, handler http.Handler) *http.Server {
	return &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
}
func openAPI(version string) map[string]any {
	paths := map[string]any{}
	for _, op := range Operations {
		paths["/api/tools/"+op.Name] = map[string]any{"post": map[string]any{"operationId": op.Name, "summary": op.Summary, "x-action-id": op.Action, "security": []any{map[string]any{"session": []string{}}, map[string]any{"bearer": []string{}}}, "requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object"}}}}, "responses": map[string]any{"200": map[string]string{"description": "Typed operation result; see API_CONTRACT.md"}, "400": map[string]string{"description": "Invalid request"}, "401": map[string]string{"description": "Authentication required"}, "403": map[string]string{"description": "Forbidden"}, "409": map[string]string{"description": "Version or state conflict"}, "413": map[string]string{"description": "Complete body exceeds 1 MiB"}}}}
	}
	for _, op := range Operations {
		definition, ok := contracts[op.Name]
		if !ok {
			panic("missing typed API contract: " + op.Name)
		}
		post := paths["/api/tools/"+op.Name].(map[string]any)["post"].(map[string]any)
		post["requestBody"] = map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": schema(definition.Input)}}}
		responses := post["responses"].(map[string]any)
		responses["200"] = map[string]any{"description": "Operation result", "content": map[string]any{"application/json": map[string]any{"schema": schema(definition.Output)}}}
		for _, code := range []string{"400", "401", "403", "404", "409", "413", "415", "422", "429", "500"} {
			responses[code] = map[string]any{"description": "Operation error", "content": map[string]any{"application/json": map[string]any{"schema": schema(struct {
				Error app.Error `json:"error"`
			}{})}}}
		}
		if op.Name == "login" {
			post["security"] = []any{}
		}
	}
	for _, path := range []string{"/healthz", "/openapi.json", "/api/actions", "/skill.md", "/", "/app.js", "/workspace.js", "/controls.js", "/workflows.js", "/agents.js", "/delivery.js", "/style.css", "/workflows.css", "/agents.css", "/delivery.css", "/shell.js", "/shell.css", "/navigation.js", "/navigation.css", "/tickets.js"} {
		paths[path] = map[string]any{"get": map[string]any{"responses": map[string]any{"200": map[string]string{"description": "Public application asset or discovery"}}}}
	}
	return map[string]any{"openapi": "3.1.0", "info": map[string]string{"title": "teleport", "version": version}, "paths": paths, "components": map[string]any{"securitySchemes": map[string]any{"session": map[string]string{"type": "apiKey", "in": "cookie", "name": "teleport_session"}, "bearer": map[string]string{"type": "http", "scheme": "bearer"}}}}
}
