// Command spring-security-docs-mcp-lambda serves the MCP server on AWS Lambda
// behind an API Gateway HTTP API (payload format 2.0).
//
// main() runs in the Lambda init phase, so the store is created once per
// execution environment and reused by every subsequent invoke.
package main

import (
	"context"
	"log"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"
	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/mcpserver"
	"github.com/shutx-net/spring-security-documentation-mcp-server/internal/store"
)

func main() {
	st, err := store.NewAWSStore(context.Background(), store.AWSConfigFromEnv())
	if err != nil {
		log.Fatalf("open AWS store: %v", err)
	}
	lambda.Start(httpadapter.NewV2(mcpserver.NewHTTPHandler(st)).ProxyWithContext)
}
