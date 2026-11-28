module github.com/PuraFome/meuRPG/backend

go 1.27.4

tool (
	connectrpc.com/connect/cmd/protoc-gen-connect-go
	google.golang.org/protobuf/cmd/protoc-gen-go
)

require (
	connectrpc.com/connect v1.21.1
	github.com/cockroachdb/cockroach-go/v2 v2.4.3
	github.com/coreos/go-oidc/v3 v3.21.0
	github.com/expr-lang/expr v1.17.8
	github.com/go-jose/go-jose/v4 v4.1.9
	github.com/google/uuid v1.6.0
	github.com/jackc/pgx/v5 v5.13.0
	github.com/pressly/goose/v3 v3.30.1
	golang.org/x/image v0.48.0
	golang.org/x/oauth2 v0.38.0
	golang.org/x/text v0.44.0
	google.golang.org/protobuf v1.36.12
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.3 // indirect
	github.com/mfridman/interpolate v0.2.0 // indirect
	github.com/sethvargo/go-retry v0.5.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/sync v0.25.0 // indirect
)
