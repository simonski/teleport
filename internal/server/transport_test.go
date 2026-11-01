package server_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"teleport/internal/app"
	"teleport/internal/server"
	"teleport/internal/teststore"
)

func TestRejectsInvalidBodiesOriginsAndUnauthenticatedCalls(t *testing.T) {
	db := teststore.Open(t)
	if _, err := app.Initialize(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	h := server.New(app.New(db), "test")
	for _, test := range []struct {
		name, body, origin string
		status             int
	}{{"unauthenticated", "{}", "", 401}, {"trailing", "{} {}", "", 400}, {"oversized", `{"padding":"` + strings.Repeat("x", 1<<20) + `"}`, "", 413}, {"cross-origin", "{}", "https://evil.test", 403}} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "http://localhost/api/tools/getSession", strings.NewReader(test.body))
			r.Header.Set("Content-Type", "application/json")
			if test.origin != "" {
				r.Header.Set("Origin", test.origin)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != test.status {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
	r := httptest.NewRequest("POST", "http://attacker.test:9999/api/tools/login", strings.NewReader(`{"username":"admin","password":"password"}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://attacker.test:9999")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 421 {
		t.Fatalf("untrusted Host reached login: status=%d", w.Code)
	}
}
