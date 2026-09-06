package main

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"
	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/mcpserver"
	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/store"
)

// TestProxyToolsList exercises the only wiring this command owns: an API
// Gateway HTTP API payload format 2.0 event reaching the MCP handler.
//
// The Accept value matters. httpadapter splits non-singleton headers on commas,
// so "application/json, text/event-stream" arrives as two Accept values, and
// the Streamable HTTP handler rejects a POST with 400 unless both media types
// are present.
func TestProxyToolsList(t *testing.T) {
	adapter := httpadapter.NewV2(mcpserver.NewHTTPHandler(store.NewMemoryStore()))

	resp, err := adapter.ProxyWithContext(context.Background(), events.APIGatewayV2HTTPRequest{
		Version:  "2.0",
		RouteKey: "$default",
		RawPath:  "/mcp",
		Headers: map[string]string{
			"content-type": "application/json",
			"accept":       "application/json, text/event-stream",
		},
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			DomainName: "ss-doc-mcp.shutx.net",
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
				Method:   http.MethodPost,
				Path:     "/mcp",
				Protocol: "HTTP/1.1",
			},
		},
		Body: `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`,
	})
	if err != nil {
		t.Fatalf("ProxyWithContext: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusOK, resp.Body)
	}
	if resp.IsBase64Encoded {
		t.Fatalf("IsBase64Encoded = true, want false for a JSON response")
	}

	var out struct {
		Error  *json.RawMessage `json:"error"`
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &out); err != nil {
		t.Fatalf("decode %q: %v", resp.Body, err)
	}
	if out.Error != nil {
		t.Fatalf("JSON-RPC error: %s", *out.Error)
	}
	if len(out.Result.Tools) == 0 {
		t.Fatalf("got 0 tools, want the MCP handler to have served the request")
	}
}
