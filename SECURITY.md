# Security Policy

## Supported versions

This project is experimental and does not currently define supported
release versions.

Security fixes are handled on a best-effort basis.

## Reporting a vulnerability

Please report security issues privately when possible.

Do not open a public issue for vulnerabilities that expose:

- credentials
- tokens
- private infrastructure details
- deployment secrets
- unauthorized access paths
- denial-of-service vectors that are not already public

If a private reporting channel is not yet configured for this repository,
please contact the repository owner directly.

## Scope

Security reports may include issues related to:

- exposed secrets
- unsafe MCP tool behavior
- unintended write or administrative operations exposed through MCP
- request handling bugs
- excessive resource usage
- dependency vulnerabilities
- infrastructure or deployment misconfiguration
- unsafe indexing or document processing behavior

## Intended security model

The hosted MCP server is intended to expose read-only functionality.

Public MCP clients should not be able to:

- rebuild indexes
- modify upstream content
- deploy infrastructure
- read credentials
- access private files
- trigger administrative operations
- write to DynamoDB or other storage except as explicitly required by safe server-side operation

## Operational recommendations

Before making this repository public, verify that no secrets or private
operational details are committed.

Recommended checks:

```bash
git status
git log --stat --oneline
git ls-files
```

Consider running a secret scanner before public release.

Recommended hosted-service controls include:

- request size limits
- query length limits
- result limit caps
- response size limits
- timeouts
- rate limiting
- access logging
- error logging
- dependency updates
- least-privilege IAM permissions
- separation of public MCP tools from administrative jobs
