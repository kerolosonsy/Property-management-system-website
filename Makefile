# Property Management System — make targets
#
# Every path is quoted: this repository lives in a directory whose name contains
# spaces, and an unquoted $(ROOT) breaks at the first one.
#
# Targets:
#   certs      generate a self-signed localhost certificate into infra/certs/
#   db-up      start PostgreSQL via docker compose
#   db-down    stop PostgreSQL
#   migrate    apply goose migrations using the owner role
#   generate   regenerate Go server types and the Angular client from contracts/openapi.yaml
#   run-api    start the Go API (TLS only, no plaintext listener exists)
#   run-web    start the Angular development server (TLS, self-signed)
#   seed-admin create the initial administrator account
#   seed-demo  insert the six design properties for manual verification
#   test       run the two pieces of unit-tested logic
#   env-check  show which required variables are set, without printing values

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

.PHONY: help certs db-up db-down migrate generate run-api run-web seed-admin seed-demo reset-admin-dev test clean env-check

help:
	@echo "Targets: certs, db-up, db-down, migrate, generate, run-api, run-web, seed-admin, seed-demo, reset-admin-dev, test, env-check, clean"

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

test:
	cd "$(API_DIR)" && go test ./internal/identity/... ./internal/auth/...

clean:
	rm -rf "$(API_DIR)/bin"
	rm -rf "$(WEB_DIR)/dist" "$(WEB_DIR)/.angular"
