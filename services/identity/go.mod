module github.com/eventflow/eventflow/services/identity

go 1.26.0

require (
	github.com/google/uuid v1.6.0
	github.com/lib/pq v1.12.3
	github.com/redis/go-redis/v9 v9.7.0
	golang.org/x/crypto v0.57.0
)

require (
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/joho/godotenv v1.5.1 // indirect
	golang.org/x/sys v0.48.0 // indirect
	gopkg.in/alexcesaro/quotedprintable.v3 v3.0.0-20150716171945-2caba252f4dc // indirect
	gopkg.in/gomail.v2 v2.0.0-20160411212932-81ebce5c23df // indirect
)

replace github.com/eventflow/eventflow/libs/go => ../../libs/go
