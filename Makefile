.DEFAULT_GOAL := help
COMPOSE := docker compose -f deploy/compose/docker-compose.yml

.PHONY: help
help: ## Liet ke cac lenh
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | \
	 awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-18s\033[0m %s\n",$$1,$$2}'

## ---------- Moi truong ----------

.PHONY: bootstrap
bootstrap: ## Cai phu thuoc cho ca ba he sinh thai
	cd apps/web && npm install
	cd tests/e2e && npm install
	cd services/ai-worker && uv sync
	go work sync

.PHONY: up
up: ## Khoi dong toan bo ha tang + service
	$(COMPOSE) up -d --build
	@echo "web        http://localhost:3000"
	@echo "rabbitmq   http://localhost:15672  (eventflow/eventflow)"
	@echo "jaeger     http://localhost:16686"
	@echo "grafana    http://localhost:3001"

.PHONY: down
down: ## Dung va xoa volume
	$(COMPOSE) down -v

.PHONY: logs
logs: ## Theo doi log cac service ung dung
	$(COMPOSE) logs -f waitingroom ticketing ai-worker

## ---------- Co so du lieu ----------

.PHONY: migrate
migrate: ## Chay migration
	$(COMPOSE) run --rm migrate goose -dir /migrations up

.PHONY: seed
seed: ## Tao du lieu mau: 1 su kien ON_SALE, 20.000 ve
	./deploy/scripts/seed.sh

## ---------- Chat luong ----------

.PHONY: lint
lint: ## Lint toan bo
	golangci-lint run ./...
	cd apps/web && npm run lint
	cd services/ai-worker && uv run ruff check .

.PHONY: test
test: ## Unit test (nhanh, khong can ha tang)
	go test ./... -race -count=1
	cd services/ai-worker && uv run pytest -q
	cd apps/web && npm test

.PHONY: test-integration
test-integration: ## Integration test (testcontainers -- can Docker)
	go test -tags=integration ./... -count=1 -timeout=10m

.PHONY: test-oversell
test-oversell: ## EVF-39 -- cong chat luong. Fail thi KHONG duoc release.
	go test -tags=integration -run TestNoOversell \
		./services/ticketing/test/concurrency/... -count=200 -timeout=30m

## ---------- Chiu tai ----------

.PHONY: load
load: ## Mo phong gio mo ban (can k6)
	@mkdir -p tests/load/results
	k6 run -e BASE=http://localhost:8080 -e EVENT=$(EVENT) tests/load/opening-spike.js

.PHONY: smoke
smoke: ## Chay tron mot luong mua ve de kiem tra moi truong
	./deploy/scripts/smoke.sh

.PHONY: test-e2e
test-e2e: ## E2E Playwright (can make up truoc)
	cd tests/e2e && npm test
