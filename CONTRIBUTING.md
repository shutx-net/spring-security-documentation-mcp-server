# Contributing

Thank you for your interest in this project.

This project is experimental and is not yet ready for broad external
contributions.

## Unofficial project notice

This is an unofficial community project.

It is not affiliated with, endorsed by, sponsored by, or maintained by
the Spring Security team, VMware, Broadcom, or the Spring project.

Please avoid wording, code, documentation, repository metadata, or assets
that could imply official endorsement by the Spring Security team or the
Spring project.

## Before opening a pull request

Please open an issue before starting substantial work.

This is especially important for changes involving:

- project naming
- attribution wording
- use of Spring Security source code or documentation
- indexing behavior
- hosted endpoint behavior
- MCP tool design
- public API or response format changes
- infrastructure changes
- security-sensitive behavior

## Contribution guidelines

Contributions should:

- preserve the unofficial nature of the project
- link users back to official Spring Security documentation where possible
- avoid removing or obscuring upstream attribution
- keep public MCP tools read-only unless a design discussion explicitly decides otherwise
- avoid committing generated indexes unless they are intentionally part of a reviewed release process
- avoid committing credentials, tokens, private endpoints, or local environment files
- include tests or verification notes when practical

## Use of Spring Security materials

This project may fetch, build, parse, or index content from the official
Spring Security repository and documentation for documentation-oriented
search and retrieval.

Contributions should not:

- claim ownership of Spring Security source code or documentation
- present this project as an official Spring Security distribution
- publish patched Spring Security source code
- remove or obscure upstream attribution
- replace official Spring Security documentation

For authoritative information, users should always be directed to the
official Spring Security documentation and source repository.

## Development notes

The MCP server implementation is located under:

```text
mcp/
```

The Go module is located at:

```text
mcp/go.mod
```

A typical setup starts with:

```bash
cd mcp
go mod download
```

The exact local run command depends on the current command layout under
`mcp/`. Update the README when the executable entry point is finalized.
