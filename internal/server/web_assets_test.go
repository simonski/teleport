package server

import (
	"bytes"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestPackedAssetsMatchSourcesAndSupportHTTPRanges(t *testing.T) {
	handler := New(nil, "fixture")
	index := httptest.NewRecorder()
	handler.ServeHTTP(index, httptest.NewRequest(http.MethodGet, "http://localhost/", nil))
	if index.Code != http.StatusOK || !strings.Contains(index.Body.String(), `name="teleport-build-version" content="fixture"`) || !strings.Contains(index.Header().Get("Content-Security-Policy"), "'nonce-") {
		t.Fatal("packed index lost its dynamic build metadata or nonce")
	}
	err := fs.WalkDir(os.DirFS("web"), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || path == "index.html" {
			return walkErr
		}
		expected, err := os.ReadFile("web/" + path)
		if err != nil {
			return err
		}
		request := httptest.NewRequest(http.MethodGet, "http://localhost/"+path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), expected) {
			t.Errorf("asset %s differs or returned %d", path, response.Code)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://localhost/vendor/three.core.js", nil)
	request.Header.Set("Range", "bytes=0-31")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusPartialContent || response.Body.Len() != 32 {
		t.Fatalf("range response: %d, %d bytes", response.Code, response.Body.Len())
	}
	if !strings.Contains(response.Header().Get("Content-Type"), "javascript") || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("packed JavaScript lost its content type or security header")
	}
	file, err := newWebFS(packedWeb).Open("vendor/three.core.js")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, ok := file.(io.Seeker); !ok {
		t.Fatal("embedded asset does not support seeking")
	}
}
