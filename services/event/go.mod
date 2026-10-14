module github.com/eventflow/eventflow/services/event

go 1.25.0

require (
	github.com/eventflow/eventflow/libs/go v0.0.0
	github.com/google/uuid v1.6.0
	github.com/lib/pq v1.12.3
)

replace github.com/eventflow/eventflow/libs/go => ../../libs/go
