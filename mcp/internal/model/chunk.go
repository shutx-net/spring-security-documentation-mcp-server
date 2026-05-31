package model

import "time"

// Area represents a Spring Security documentation area.
type Area string

const (
	AreaServlet        Area = "servlet"
	AreaReactive       Area = "reactive"
	AreaOAuth2         Area = "oauth2"
	AreaSAML2          Area = "saml2"
	AreaMethodSecurity Area = "method-security"
	AreaTesting        Area = "testing"
	AreaArchitecture   Area = "architecture"
	AreaAuthorization  Area = "authorization"
	AreaAuthentication Area = "authentication"
	AreaOther          Area = "other"
)

// SourceType represents how the documentation was built.
type SourceType string

const (
	SourceTypeAntoraBuild SourceType = "antora-build"
	SourceTypeOfficialZIP SourceType = "official-zip"
)

// DocChunk is a single searchable unit of Spring Security documentation.
type DocChunk struct {
	ID string // sha256(ref + commitSha + canonicalUrl + headingPath)

	Project string // "spring-security"

	Ref        string     // e.g. "6.5.x", "7.0.x", "main"
	CommitSha  string
	BuiltAt    time.Time
	SourceType SourceType
	SourcePath string // path inside the generated HTML site
	SourceFile string // original .adoc file (optional)

	CanonicalURL string
	Title        string
	HeadingPath  []string // e.g. ["Authentication", "Username/Password", "Form Login"]

	Area Area

	ContentHtml string // raw HTML fragment of the chunk body
	ContentText string // plain text extracted for FTS and Embedding

	IndexedAt time.Time
}
