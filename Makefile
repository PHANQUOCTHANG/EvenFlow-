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
	$(COMPOSE) logs -f waitingroom waitingroom-controller ticketing ticketing-worker ai-worker

## ---------- Co so du lieu ----------

.PHONY: migrate
migrate: ## Chay migration
	$(COMPOSE) run --rm migrate goose -dir /migrations up

.PHONY: seed
seed: ## Tao du lieu mau: 1 su kien ON_SALE, 20.000 ve
	bash deploy/scripts/seed.sh

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

# Chi la tien ich. Duong chay CHINH THUC la `bash .github/scripts/gates.sh`:
# may dev khong co `make`, nen script khong duoc phu thuoc muc tieu nay (C4).
.PHONY: gates
gates: ## Cong chat luong cuc bo cho apps/web (lint, typecheck, coverage, build + C4.2/C4.3/C4.4)
	bash .github/scripts/gates.sh

.PHONY: test-integration
test-integration: ## Integration test (testcontainers -- can Docker)
	go test -tags=integration ./... -count=1 -timeout=10m

# TESTFLAGS: co them cho `go test`, mac dinh rong (chay tay khong doi hanh vi). CI
# truyen `TESTFLAGS=-v` de .github/scripts/run-and-report-tests.sh dem duoc so lan
# PASS/SKIP -- `go test` coi SKIP la thanh cong, nen khong co -v thi mot gate skip
# toan bo van xanh ma khong ai biet.
TESTFLAGS ?=

.PHONY: test-oversell
test-oversell: ## EVF-39 -- cong chat luong: 10.000 goroutine mua 100 ve, 200 lan. Fail thi KHONG duoc release.
	go test $(TESTFLAGS) -tags=integration -run TestNoOversell_UnderExtremeConcurrency \
		./services/ticketing/test/concurrency/... -count=200 -timeout=30m

# Tach rieng vi moi lan chay ngu >= 15 giay (5 vong cho hold het han): 200 lan la hon mot gio.
.PHONY: test-oversell-expiry
test-oversell-expiry: ## EVF-39 -- sweeper tra kho song song voi viec mua (20 lan)
	go test $(TESTFLAGS) -tags=integration -run TestNoOversell_WithConcurrentExpiry \
		./services/ticketing/test/concurrency/... -count=20 -timeout=20m

## ---------- Chiu tai ----------

# Bo k6 (EV-194): tests/load/main.js + tests/load/README.md. Moi bien duoi day duoc chuyen sang k6 bang -e neu co dat,
# vd: make load PROFILE=spike JOIN_TARGET=5000. K6 doi duoc thanh image Docker (xem README).
K6 ?= k6
PROFILE ?= smoke
LOAD_VARS := PROFILE BASE EVENT TICKET_TYPE JWT_SECRET ALLOW_REMOTE REMOTE_CONFIRM RUN_ID HOLD_TTL USERS QUOTA SLO_SCALE SIGNALS   JOIN_TARGET JOIN_RAMP JOIN_MAX_VUS JITTER_MS WAIT_VUS WAIT_DURATION CHECKOUT_RATE CHECKOUT_DURATION   WEBHOOK_RATE REPLAY_RATIO
LOAD_ENV = $(foreach v,$(LOAD_VARS),$(if $($(v)),-e $(v)=$($(v))))

.PHONY: load
load: ## Bo k6 qua gateway: PROFILE=smoke|spike|wait|checkout|full (can k6, ANTIBOT_MODE=shadow)
	@mkdir -p tests/load/results
	$(K6) run $(LOAD_ENV) tests/load/main.js

.PHONY: load-seed
load-seed: ## Tao su kien tai + USERS danh tinh tai (idempotent)
	bash tests/load/seed-load.sh seed

.PHONY: load-verify
load-verify: ## Kiem oversell trong Postgres: issued + open holds <= quota
	bash tests/load/seed-load.sh verify

.PHONY: load-smoke
load-smoke: load-seed ## Seed + PROFILE=smoke (< 2 phut tren compose)
	@$(MAKE) --no-print-directory load PROFILE=smoke

.PHONY: test-load-lib
test-load-lib: ## Unit test helper k6 (node --test, khong can Docker/k6)
	node --test tests/load/lib/*.test.mjs

.PHONY: smoke
smoke: ## Chay tron mot luong mua ve (xep hang -> admit -> giu ghe -> tra kho)
	bash deploy/scripts/smoke.sh

.PHONY: demo
demo: ## Demo end-to-end qua gateway: dang ky -> organizer tao su kien -> mua ve -> ve + email (can make up/migrate/seed)
	bash deploy/scripts/demo.sh

.PHONY: test-e2e
test-e2e: ## E2E Playwright (can make up truoc)
	cd tests/e2e && npm test
