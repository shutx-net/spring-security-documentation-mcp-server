FROM golang:1.26 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -trimpath \
    -o /bin/spring-security-docs-mcp \
    ./cmd/spring-security-docs-mcp

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /bin/spring-security-docs-mcp /spring-security-docs-mcp
EXPOSE 8080
ENTRYPOINT ["/spring-security-docs-mcp"]
CMD ["serve-http", "--port", "8080"]
