module github.com/eventflow/eventflow/services/ticketing

go 1.23

require (
	github.com/eventflow/eventflow/libs/go v0.0.0
	github.com/jackc/pgx/v5 v5.7.1
	github.com/redis/go-redis/v9 v9.7.0
)

replace github.com/eventflow/eventflow/libs/go => ../../libs/go
