# vpn-manager — Makefile

.PHONY: all up down build run sqlc migrate migrate-down test lint clean

# ============================================================================
# Переменные
# ============================================================================
APP_NAME    := vpn-manager
CMD_DIR     := ./cmd/$(APP_NAME)
BIN_OUT     := $(APP_NAME)
DB_DSN      ?= postgres://vpnmanager:vpnmanager@localhost:5432/vpnmanager?sslmode=disable
MIGRATE_DIR := ./migrations

DC          := docker compose

# ============================================================================
# Цели
# ============================================================================

all: build

# Запустить сервисы
up:
	$(DC) up -d

up-build:
	$(DC) up -d --build

# Остановить сервисы
down:
	$(DC) down

# Логи
logs:
	$(DC) logs -f

# Пересобрать и перезапустить
restart: down up

# ============================================================================
# Разработка локально
# ============================================================================

# ============================================================================
# Сборка
# ============================================================================

# Собрать бинарник (включает сборку фронтенда)
build: frontend-build
	go build -o $(BIN_OUT) $(CMD_DIR)

# Собрать только бэкенд (без фронтенда)
build-backend:
	go build -o $(BIN_OUT) $(CMD_DIR)

# Собрать фронтенд (Svelte)
.PHONY: frontend-build
frontend-build:
	cd frontend && npm ci --silent 2>/dev/null; npm run build
	rm -rf internal/web/ui
	cp -r frontend/dist internal/web/ui

# Запустить локально (БД должна быть поднята)
run:
	go run $(CMD_DIR)

# Запустить dev-окружение (БД + hot-reload)
dev:
	$(DC) up -d postgres
	go run $(CMD_DIR)

# ============================================================================
# SQLC
# ============================================================================

# Сгенерировать Go-код из SQL-запросов
sqlc:
	sqlc generate

sqlc-verify:
	sqlc vet

# ============================================================================
# Миграции
# ============================================================================

# Накатить миграции
migrate:
	migrate -path $(MIGRATE_DIR) -database "$(DB_DSN)" up

# Откатить миграции
migrate-down:
	migrate -path $(MIGRATE_DIR) -database "$(DB_DSN)" down 1

# Откатить все миграции
migrate-reset:
	migrate -path $(MIGRATE_DIR) -database "$(DB_DSN)" down -all

# Создать новую миграцию
migrate-new:
	@read -p "Migration name: " name; \
	migrate create -ext sql -dir $(MIGRATE_DIR) -seq $$name

# Применить миграции через docker
migrate-docker:
	$(DC) exec app migrate -path /migrations -database "$(DB_DSN)" up

# ============================================================================
# Тесты
# ============================================================================

test:
	go test -count=1 -v ./internal/...

test-short:
	go test -count=1 -short ./internal/...

test-cover:
	go test -count=1 -coverprofile=coverage.out ./internal/...
	go tool cover -func=coverage.out

test-cover-html:
	go test -count=1 -coverprofile=coverage.out ./internal/...
	go tool cover -html=coverage.out -o coverage.html

# ============================================================================
# Инструменты
# ============================================================================

lint:
	golangci-lint run ./...

tidy:
	go mod tidy

# Установить зависимости для разработки
tools:
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
	go install github.com/golang-migrate/migrate/v4/cmd/migrate@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# ============================================================================
# Вспомогательное
# ============================================================================

# Показать структуру проекта
tree:
	find . -not -path './.git/*' -not -path './vendor/*' -not -path './coverage*' \
		-not -name '*.out' -not -name '*.test' -not -name 'go.sum' \
		| sort | head -60

# Очистка
clean:
	rm -f $(BIN_OUT) coverage.out coverage.html
	rm -rf tmp/
