module github.com/eventflow/eventflow/services/identity

go 1.26.0

require github.com/eventflow/eventflow/libs/go v0.0.0

require (
	github.com/google/uuid v1.6.0 // indirect
	github.com/lib/pq v1.12.3 // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
)

replace github.com/eventflow/eventflow/libs/go => ../../libs/go
