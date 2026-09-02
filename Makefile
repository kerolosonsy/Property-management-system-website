# Property Management System — make targets
#
# Every path is quoted: this repository lives in a directory whose name contains
# spaces, and an unquoted $(ROOT) breaks at the first one.
#
SHELL := /bin/bash

# Paths
ROOT := $(shell pwd)
API_DIR := $(ROOT)/api
WEB_DIR := $(ROOT)/web
CONTRACT := $(ROOT)/contracts/openapi.yaml
CERT_DIR := $(ROOT)/infra/certs
CERT_FILE := $(CERT_DIR)/localhost.crt
KEY_FILE := $(CERT_DIR)/localhost.key
ENV_FILE := $(ROOT)/.env
COMPOSE := docker compose -f "$(ROOT)/infra/docker-compose.yml"

# Load .env into the environment for targets that need it. `set -a` exports every
# assignment; the values in .env are shell-quoted so paths containing spaces survive.
# Targets that need credentials prefix their recipe with $(LOAD_ENV).
LOAD_ENV = set -a; if [ -f "$(ENV_FILE)" ]; then . "$(ENV_FILE)"; fi; set +a;

.PHONY: help env-check certs db-up db-down migrate generate build vet test check fmt dev run-api run-web logs psql reset-db seed-admin reset-admin-dev seed-demo clean
.PHONY: setup

help:
	@printf '%s\n' \
		"help             list every target" \
		"env-check        validate required environment variables without printing secrets" \
		"certs            generate a self-signed localhost TLS certificate" \
		"db-up            start PostgreSQL and wait for readiness" \
		"db-down          stop PostgreSQL" \
		"migrate          apply database migrations using the owner role" \
		"generate         regenerate Go and Angular clients from the OpenAPI contract" \
		"build            build production artifacts for the API and web app" \
		"vet              run go vet across the API" \
		"test             run all Go tests" \
		"check            run build, vet, tests, and the physical-CSS check" \
		"fmt              run gofmt and Prettier" \
		"dev              start PostgreSQL, the API, and the web development server" \
		"run-api          start the Go API with TLS" \
		"run-web          start the Angular development server with TLS" \
		"logs             follow PostgreSQL logs" \
		"psql             open psql as the database owner" \
		"reset-db         delete the local database volume and rebuild the schema" \
		"seed-admin       create the first administrator" \
		"reset-admin-dev  reset the local administrator password from .env" \
		"seed-demo        load demonstration property data" \
		"clean            remove generated build artifacts"

setup:
	bash "$(ROOT)/scripts/setup.sh"

env-check:
	@$(LOAD_ENV) \
	missing=0; \
	for v in PMS_DATABASE_APP_URL PMS_DATABASE_OWNER_URL PMS_TLS_CERT_PATH PMS_TLS_KEY_PATH; do \
		if [ -z "$${!v}" ]; then echo "  MISSING  $$v"; missing=1; else echo "  set      $$v"; fi; \
	done; \
	if [ -z "$$PMS_KEK" ]; then \
		echo "  MISSING  PMS_KEK (Constitution VII)"; missing=1; \
	else \
		decoded="$$(printf '%s' "$$PMS_KEK" | base64 -d 2>/dev/null | wc -c | tr -d ' ')"; \
		if [ "$$decoded" = "32" ]; then \
			echo "  set      PMS_KEK (32 bytes)"; \
		else \
			echo "  WRONG    PMS_KEK (decodes to $$decoded bytes; need 32)"; missing=1; \
		fi; \
	fi; \
	if [ -z "$$PMS_ATTACHMENT_STORE" ]; then \
		echo "  MISSING  PMS_ATTACHMENT_STORE"; missing=1; \
	else \
		if [ -d "$$PMS_ATTACHMENT_STORE" ] && [ -w "$$PMS_ATTACHMENT_STORE" ]; then \
			echo "  set      PMS_ATTACHMENT_STORE=$$PMS_ATTACHMENT_STORE (writable)"; \
		else \
			echo "  WRONG    PMS_ATTACHMENT_STORE=$$PMS_ATTACHMENT_STORE (missing, not a directory, or not writable)"; missing=1; \
		fi; \
	fi; \
	echo "  set      PMS_ATTACHMENT_MAX_BYTES=$${PMS_ATTACHMENT_MAX_BYTES:-52428800} (bytes)"; \
	echo "  set      PMS_EXTRACT_TEXT_MAX_BYTES=$${PMS_EXTRACT_TEXT_MAX_BYTES:-262144} (bytes)"; \
	if [ ! -f "$(ENV_FILE)" ]; then \
		echo; echo "No .env found. Copy .env.example to .env and fill it in."; exit 1; \
	fi; \
	if [ $$missing -ne 0 ]; then echo; echo "Fill the missing variables in $(ENV_FILE)."; exit 1; fi; \
	echo "All required variables are set."

certs:
	@mkdir -p "$(CERT_DIR)"
	@if [ ! -f "$(CERT_FILE)" ] || [ ! -f "$(KEY_FILE)" ]; then \
		openssl req -x509 -newkey rsa:2048 -nodes -sha256 -days 3650 \
			-keyout "$(KEY_FILE)" \
			-out "$(CERT_FILE)" \
			-subj "/CN=localhost" \
			-addext "subjectAltName=DNS:localhost,IP:127.0.0.1"; \
		echo "Self-signed certificate written to $(CERT_DIR)"; \
	else \
		echo "Certificate already exists at $(CERT_FILE)"; \
	fi

db-up:
	$(COMPOSE) up -d
	$(COMPOSE) exec -T postgres pg_isready

db-down:
	$(COMPOSE) down

migrate:
	@$(LOAD_ENV) \
	if [ -z "$$PMS_DATABASE_OWNER_URL" ]; then \
		echo "PMS_DATABASE_OWNER_URL must be set (owner role only). Run 'make env-check'."; exit 1; \
	fi; \
	cd "$(API_DIR)" && go run ./cmd/migrate up

generate:
	cd "$(API_DIR)" && go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen \
		--config oapi-codegen.yaml "$(CONTRACT)"
	cd "$(WEB_DIR)" && npm run gen:api

build:
	@mkdir -p "$(API_DIR)/bin"
	cd "$(API_DIR)" && go build -o "$(API_DIR)/bin/pms-api" ./cmd/server
	cd "$(WEB_DIR)" && npm run build -- --configuration production

vet:
	cd "$(API_DIR)" && go vet ./...

test:
	cd "$(API_DIR)" && go test ./...

check: build vet test
	@if grep -rnE '(margin|padding|border)-(left|right)|[^-a-z]\b(left|right):' \
		"$(WEB_DIR)/src/app" "$(WEB_DIR)/src/styles.scss"; then \
		echo "Physical CSS properties found."; exit 1; \
	else \
		echo "Physical CSS check: clean"; \
	fi

fmt:
	find "$(API_DIR)" -type f -name '*.go' -exec gofmt -w '{}' +
	cd "$(WEB_DIR)" && npx prettier --write "src/**/*.{ts,html,scss,css}"

dev: db-up
	@set -e; \
	$(MAKE) -C "$(ROOT)" run-api & pms_api_pid=$$!; \
	$(MAKE) -C "$(ROOT)" run-web & pms_web_pid=$$!; \
	trap 'kill "$$pms_api_pid" "$$pms_web_pid" 2>/dev/null || true' EXIT INT TERM; \
	wait

run-api:
	@$(LOAD_ENV) \
	if [ -z "$$PMS_DATABASE_APP_URL" ]; then \
		echo "PMS_DATABASE_APP_URL is not set. Run 'make env-check' to see what is missing."; exit 1; \
	fi; \
	cd "$(API_DIR)" && go run ./cmd/server

# TLS and the /api/v1 proxy are configured in web/angular.json (ssl, sslKey,
# sslCert, proxyConfig), so no flags are passed here — duplicating them on the
# command line would re-introduce the quoting problem this Makefile exists to avoid.
run-web:
	cd "$(WEB_DIR)" && npm start

logs:
	$(COMPOSE) logs --follow --tail=200 postgres

psql:
	$(COMPOSE) exec postgres psql --username pms_owner --dbname pms

reset-db:
	@read -r -p "Delete the local PostgreSQL volume? Type reset-db to continue: " pms_reset_answer; \
	if [ "$$pms_reset_answer" != "reset-db" ]; then echo "Cancelled."; exit 1; fi
	$(COMPOSE) down --volumes
	$(MAKE) -C "$(ROOT)" db-up
	$(MAKE) -C "$(ROOT)" migrate
seed-admin:
	@$(LOAD_ENV) \
	cd "$(API_DIR)" && go run ./cmd/admintool seed-admin --username "$${SEED_USERNAME:-admin}"

# Local development only. Resets the admin password to DEV_ADMIN_PASSWORD from .env and
# clears the change-on-next-sign-in requirement, so the account is usable immediately.
# The password lives in .env (gitignored), never in this file: Constitution VII forbids
# committing a secret, "not even a development value". It is passed by environment
# variable name rather than as a flag, because a flag value shows up in `ps` and shell
# history. This target must never be run against anything but a local database.
reset-admin-dev:
	@$(LOAD_ENV) \
	if [ -z "$${DEV_ADMIN_PASSWORD:-}" ]; then \
		echo "DEV_ADMIN_PASSWORD is not set in .env — add it there (it is gitignored)."; \
		exit 1; \
	fi; \
	cd "$(API_DIR)" && DEV_ADMIN_PASSWORD="$$DEV_ADMIN_PASSWORD" go run ./cmd/admintool reset-admin \
		--username "$${SEED_USERNAME:-admin}" \
		--password-env DEV_ADMIN_PASSWORD \
		--must-change=false
	@echo "admin password reset from DEV_ADMIN_PASSWORD; no change required at next sign-in"

seed-demo:
	@$(LOAD_ENV) \
	if [ -z "$$PMS_DATABASE_OWNER_URL" ]; then \
		echo "PMS_DATABASE_OWNER_URL must be set (owner role only). Run 'make env-check'."; exit 1; \
	fi; \
	cd "$(API_DIR)" && go run ./cmd/admintool seed-demo

clean:
	rm -rf "$(API_DIR)/bin"
	rm -rf "$(WEB_DIR)/dist" "$(WEB_DIR)/.angular"
