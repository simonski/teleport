package server_test

import (
	"context"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"teleport/internal/app"
	"teleport/internal/server"
	"teleport/internal/teststore"
)

func TestTerminalStyleNonceIsPerDocumentWithoutInlineScriptPermission(t *testing.T) {
	db := teststore.Open(t)
	if _, err := app.Initialize(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	h := server.New(app.New(db), "test")
	previous := ""
	for _, path := range []string{"/", "/index.html"} {
		response := httptest.NewRecorder()
		h.ServeHTTP(response, httptest.NewRequest("GET", "http://localhost"+path, nil))
		matches := regexp.MustCompile(`name="teleport-style-nonce" content="([A-Za-z0-9_-]+)"`).FindStringSubmatch(response.Body.String())
		if response.Code != 200 || len(matches) != 2 || len(matches[1]) < 24 || matches[1] == previous {
			t.Fatal("missing or reused page nonce")
		}
		policy := response.Header().Get("Content-Security-Policy")
		if !strings.Contains(policy, "style-src 'self' 'nonce-"+matches[1]+"'") || !strings.Contains(policy, "script-src 'self';") || strings.Contains(policy, "unsafe-inline") {
			t.Fatal("nonce weakened script policy", policy)
		}
		previous = matches[1]
	}
}
