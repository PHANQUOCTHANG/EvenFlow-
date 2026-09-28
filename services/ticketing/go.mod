module github.com/eventflow/eventflow/services/ticketing

go 1.24

require (
	github.com/eventflow/eventflow/libs/go v0.0.0
	github.com/jackc/pgx/v5 v5.7.1
	github.com/redis/go-redis/v9 v9.22.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/crypto v0.27.0 // indirect
	golang.org/x/sync v0.8.0 // indirect
	golang.org/x/sys v0.30.0 // indirect
	golang.org/x/text v0.18.0 // indirect
)

replace github.com/eventflow/eventflow/libs/go => ../../libs/go
