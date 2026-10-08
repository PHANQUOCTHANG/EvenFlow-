module github.com/eventflow/eventflow/services/ticketing

go 1.25.0

require (
	github.com/eventflow/eventflow/libs/go v0.0.0
	github.com/jackc/pgx/v5 v5.9.2
	github.com/redis/go-redis/v9 v9.7.0
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	golang.org/x/text v0.29.0 // indirect
)

replace github.com/eventflow/eventflow/libs/go => ../../libs/go
