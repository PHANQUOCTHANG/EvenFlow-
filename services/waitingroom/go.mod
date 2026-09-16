module github.com/eventflow/eventflow/services/waitingroom

go 1.23

require (
	github.com/eventflow/eventflow/libs/go v0.0.0
	github.com/redis/go-redis/v9 v9.7.0
)

replace github.com/eventflow/eventflow/libs/go => ../../libs/go
