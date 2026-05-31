package indexer

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/model"
)

// chunkID generates a stable chunk ID from its identifying fields.
func chunkID(ref, commitSha, canonicalURL string, headingPath []string) string {
	h := sha256.Sum256([]byte(ref + "\x00" + commitSha + "\x00" + canonicalURL + "\x00" + strings.Join(headingPath, "/")))
	return fmt.Sprintf("%x", h[:16])
}

// inferArea maps a URL path to a documentation area based on the first path component.
func inferArea(sourcePath string) model.Area {
	top := strings.ToLower(strings.SplitN(sourcePath, "/", 2)[0])
	switch top {
	case "servlet":
		return model.AreaServlet
	case "reactive":
		return model.AreaReactive
	case "oauth2":
		return model.AreaOAuth2
	case "saml2":
		return model.AreaSAML2
	case "method-security", "method_security":
		return model.AreaMethodSecurity
	case "testing":
		return model.AreaTesting
	case "architecture":
		return model.AreaArchitecture
	case "authorization":
		return model.AreaAuthorization
	case "authentication":
		return model.AreaAuthentication
	default:
		return model.AreaOther
	}
}
