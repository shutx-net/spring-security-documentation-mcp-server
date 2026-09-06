package mcpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/store"
)

func newTestHTTPServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(NewHTTPHandler(store.NewMemoryStore()))
	t.Cleanup(srv.Close)
	return srv
}

// postMCP sends one JSON-RPC message to /mcp with the headers the Streamable
// HTTP transport requires; without them the handler rejects the request.
func postMCP(t *testing.T, srv *httptest.Server, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/mcp", strings.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestHTTPHandlerInitialize(t *testing.T) {
	srv := newTestHTTPServer(t)
	resp := postMCP(t, srv, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0.1"}}}`)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	var out struct {
		Result struct {
			ServerInfo struct {
				Name string `json:"name"`
			} `json:"serverInfo"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Result.ServerInfo.Name != "spring-security-docs" {
		t.Errorf("serverInfo.name = %q, want %q", out.Result.ServerInfo.Name, "spring-security-docs")
	}
}

func TestHTTPHandlerToolsListWithoutInitialize(t *testing.T) {
	srv := newTestHTTPServer(t)
	resp := postMCP(t, srv, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var out struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Result.Tools) != 4 {
		t.Fatalf("got %d tools, want 4", len(out.Result.Tools))
	}
}

func TestHTTPHandlerGetMCPNotAllowed(t *testing.T) {
	srv := newTestHTTPServer(t)
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/mcp", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
	}
	if allow := resp.Header.Get("Allow"); !strings.Contains(allow, http.MethodPost) {
		t.Errorf("Allow = %q, want it to contain %q", allow, http.MethodPost)
	}
}

func TestHTTPHandlerHealthz(t *testing.T) {
	srv := newTestHTTPServer(t)
	resp, err := srv.Client().Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var out map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["status"] != "ok" {
		t.Errorf("status = %q, want %q", out["status"], "ok")
	}
}
