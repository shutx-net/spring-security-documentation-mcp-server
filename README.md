# Unofficial MCP Server for Spring Security Documentation

<p align="center">
  <a href="./LICENSE"><img src="https://img.shields.io/badge/License-Apache%202.0-blue.svg" alt="License: Apache-2.0"></a>
  <img src="https://img.shields.io/badge/status-experimental-orange" alt="Status: Experimental">
  <img src="https://img.shields.io/badge/MCP-Streamable%20HTTP-purple" alt="MCP: Streamable HTTP">
  <img src="https://img.shields.io/badge/Go-1.26.3-00ADD8?logo=go&logoColor=white" alt="Go 1.26.3">
</p>

> [!IMPORTANT]
> This is an unofficial community project.
> It is not affiliated with, endorsed by, sponsored by, or maintained by the Spring Security team, VMware, Broadcom, or the Spring project.

This project provides an experimental MCP (Model Context Protocol) server for searching and referencing Spring Security documentation through a remote HTTP MCP endpoint.

The purpose of this project is to help MCP-compatible clients find relevant Spring Security documentation while preserving attribution to the official Spring Security project and linking users back to the official documentation whenever possible.

This project is not intended to replace the official Spring Security documentation or any official support channel.

## Status

Experimental.

The current hosted server has been verified to respond to MCP `initialize` requests over HTTP.

Current server information:

```json
{
  "name": "spring-security-docs",
  "version": "0.1.0"
}
```

Current advertised capabilities include:

```json
{
  "logging": {},
  "tools": {
    "listChanged": true
  }
}
```

## Repository layout

```text
.
├── .claude/              # Claude-related local/project configuration
├── .devcontainer/        # Development container configuration
├── .github/workflows/    # GitHub Actions workflows
├── cdk/                  # Infrastructure as Code
├── mcp/                  # Go-based MCP server implementation
├── .gitattributes
├── .gitignore
└── CLAUDE.md
```

The MCP server implementation lives under `mcp/`.

The Go module is defined in:

```text
mcp/go.mod
```

## MCP server

The MCP server is implemented in Go.

The Go module currently uses:

- `github.com/modelcontextprotocol/go-sdk`
- `github.com/spf13/cobra`
- `github.com/aws/aws-sdk-go-v2`
- `github.com/aws/aws-sdk-go-v2/service/dynamodb`
- `github.com/PuerkitoBio/goquery`

This indicates that the implementation is centered around:

- MCP server functionality
- CLI command handling
- AWS integration
- DynamoDB access
- HTML parsing or documentation processing

## Hosted endpoint

The current hosted MCP endpoint is:

```text
https://ss-doc-mcp.shutx.net/mcp
```

The server uses Streamable HTTP / SSE-style responses.

A successful initialization response includes:

- `HTTP/2 200`
- `Content-Type: text/event-stream`
- `Mcp-Session-Id` response header
- JSON-RPC response body

## Using with Claude Code

Add this MCP server to Claude Code with the following command:

```bash
claude mcp add --transport http spring-security-docs https://ss-doc-mcp.shutx.net/mcp
```

## Initialize example

```bash
curl -i -sS -X POST 'https://ss-doc-mcp.shutx.net/mcp' \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  --data '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
      "protocolVersion": "2025-03-26",
      "capabilities": {},
      "clientInfo": {
        "name": "curl-test",
        "version": "0.1.0"
      }
    }
  }'
```

Example successful response:

```text
HTTP/2 200
content-type: text/event-stream
mcp-session-id: <session-id>

event: message
data: {"jsonrpc":"2.0","id":1,"result":{"capabilities":{"logging":{},"tools":{"listChanged":true}},"protocolVersion":"2025-03-26","serverInfo":{"name":"spring-security-docs","version":"0.1.0"}}}
```

## Listing tools

After initialization, use the returned `Mcp-Session-Id` header for subsequent requests.

```bash
SESSION_ID='<session-id>'

curl -i -sS -X POST 'https://ss-doc-mcp.shutx.net/mcp' \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H "Mcp-Session-Id: ${SESSION_ID}" \
  --data '{
    "jsonrpc": "2.0",
    "id": 2,
    "method": "tools/list",
    "params": {}
  }'
```

## Calling a tool

Use `tools/call` with a tool name returned by `tools/list`.

```bash
SESSION_ID='<session-id>'

curl -i -sS -X POST 'https://ss-doc-mcp.shutx.net/mcp' \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H "Mcp-Session-Id: ${SESSION_ID}" \
  --data '{
    "jsonrpc": "2.0",
    "id": 3,
    "method": "tools/call",
    "params": {
      "name": "<tool-name>",
      "arguments": {}
    }
  }'
```

## Use of Spring Security source code and documentation

This project may fetch, build, parse, or index content from the official Spring Security repository and documentation in order to provide documentation search and retrieval through MCP tools.

The intended use is limited to documentation-oriented search and reference.

The Spring Security repository is treated as an upstream source for documentation indexing, not as code owned or maintained by this project.

This project does not intend to redistribute the Spring Security source code as part of this repository. If documentation text is extracted into an index, it should be used to support search, retrieval, citation, and navigation back to the official documentation.

Where possible, indexed or returned content should preserve metadata such as:

- upstream repository URL
- documentation version or branch
- commit SHA
- official documentation URL
- build timestamp
- heading path or page title

Search results should link back to the official Spring Security documentation whenever possible.

For authoritative information, users should always refer to the official Spring Security documentation and the official Spring Security source repository.

## What this project does not do with Spring Security sources

This project does not intend to:

- claim ownership of Spring Security source code or documentation
- present itself as an official Spring Security distribution
- imply endorsement by the Spring Security team or the Spring project
- publish patched Spring Security source code
- modify the upstream Spring Security project
- remove or obscure upstream attribution
- replace the official Spring Security documentation
- provide official Spring Security support

## Project scope

This project is focused on Spring Security documentation.

The server is intended to help MCP-compatible clients search or retrieve Spring Security documentation through MCP tools.

The intended behavior is read-only from the perspective of MCP clients. Administrative operations, index rebuilds, deployments, and credential-related operations should not be exposed as public MCP tools.

## Local development

The repository contains development container configuration under `.devcontainer/`.

The MCP server source is located under `mcp/`.

```bash
cd mcp
go mod download
```

The exact local run command depends on the current command layout under `mcp/`.

If the module has a root `main.go`, use:

```bash
cd mcp
go run .
```

If the executable entry point is under `cmd/`, use the matching command package, for example:

```bash
cd mcp
go run ./cmd/...
```

Update this section once the executable entry point is finalized.

## Current limitations

The project is currently experimental.

At this stage:

- The hosted endpoint may change.
- Available MCP tools may change.
- Response formats may change.
- Local execution instructions are not finalized in this README.
- Public release artifacts are not documented.
- The exact indexing and deployment process is not documented in this README.

## Naming and attribution

This project uses the name “Spring Security” only to identify the upstream project and documentation that this unofficial tool is designed to search.

The project name, documentation, repository description, and hosted endpoint should not imply that this is an official Spring project.

If any naming, wording, attribution, or disclaimer is inappropriate, please open an issue. The intent is to respect the Spring Security project and to make the unofficial nature of this project clear.

## Security

This server is intended to expose read-only MCP functionality.

Do not expose administrative operations, index rebuild operations, deployment operations, credentials, or private data through public MCP tools.

Recommended operational controls include:

- request size limits
- query length limits
- response size limits
- timeouts
- rate limiting
- access logging
- error logging
- dependency updates
- secret scanning before public release

Before making this repository public, verify that no secrets or private operational data are committed.

Recommended checks:

```bash
git status
git log --stat --oneline
git ls-files
```

Consider running a secret scanner before public release.

## License

This project is licensed under the Apache License 2.0.

See [LICENSE](./LICENSE) for the full license text.

See [NOTICE](./NOTICE) for attribution, unofficial project notice, and clarification about the scope of this project's license.

## Disclaimer

This project is provided as-is, without warranty.

It is an unofficial experimental tool for accessing Spring Security documentation through MCP. It is not a substitute for the official Spring Security documentation, the official Spring Security source repository, or official support channels.
