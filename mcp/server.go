package mcp

import (
	"context"
	"errors"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func ServeStdio() error {
	s := gomcp.NewServer(&gomcp.Implementation{Name: "spring-security-docs", Version: "0.1.0"}, nil)

	type searchArgs struct {
		Query string `json:"query" jsonschema:"search query"`
		Ref   string `json:"ref,omitempty" jsonschema:"Spring Security version ref (e.g. 6.5.x, 7.0.x)"`
		Area  string `json:"area,omitempty" jsonschema:"documentation area (e.g. servlet, reactive, oauth2)"`
		Limit int    `json:"limit,omitempty" jsonschema:"maximum number of results"`
	}
	gomcp.AddTool(s, &gomcp.Tool{
		Name:        "search_spring_security_docs",
		Description: "Search Spring Security documentation",
	}, func(_ context.Context, _ *gomcp.CallToolRequest, _ searchArgs) (*gomcp.CallToolResult, any, error) {
		return nil, nil, errors.New("not yet implemented")
	})

	type getArgs struct {
		ID string `json:"id" jsonschema:"chunk ID"`
	}
	gomcp.AddTool(s, &gomcp.Tool{
		Name:        "get_spring_security_doc",
		Description: "Get a Spring Security documentation chunk by ID",
	}, func(_ context.Context, _ *gomcp.CallToolRequest, _ getArgs) (*gomcp.CallToolResult, any, error) {
		return nil, nil, errors.New("not yet implemented")
	})

	gomcp.AddTool(s, &gomcp.Tool{
		Name:        "list_spring_security_doc_sets",
		Description: "List available Spring Security documentation sets",
	}, func(_ context.Context, _ *gomcp.CallToolRequest, _ struct{}) (*gomcp.CallToolResult, any, error) {
		return nil, nil, errors.New("not yet implemented")
	})

	gomcp.AddTool(s, &gomcp.Tool{
		Name:        "get_spring_security_docs_status",
		Description: "Get the status of the Spring Security documentation index",
	}, func(_ context.Context, _ *gomcp.CallToolRequest, _ struct{}) (*gomcp.CallToolResult, any, error) {
		return nil, nil, errors.New("not yet implemented")
	})

	return s.Run(context.Background(), &gomcp.StdioTransport{})
}
