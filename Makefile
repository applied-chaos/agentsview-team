SHELL := /bin/bash
GOLANGCI_LINT := github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
GOPKGS        := ./cmd/... ./internal/...
COMPOSE       := docker compose -f compose.yaml
IMAGE         ?= agentsview-team:local
# One Compose project per run (CI sets it from the run id), so concurrent runs share nothing.
export COMPOSE_PROJECT_NAME ?= avteam-$(shell printf %s '$(CURDIR)' | cksum | cut -d' ' -f1)
export PLAYWRIGHT_VERSION := $(shell sed -n 's/.*"@playwright\/test": "\(.*\)".*/\1/p' web/package.json)

.PHONY: psa check fmt fmt-check lint unit e2e ui web-install image down

check: fmt-check lint unit e2e ui

fmt:
	gofmt -w cmd internal e2e
	cd web && npx prettier --write .

fmt-check: web-install
	@test -z "$$(gofmt -l cmd internal e2e)" || { gofmt -l cmd internal e2e; echo "gofmt: files above need formatting"; exit 1; }
	cd web && npm run -s format:check

lint: web-install
	go vet $(GOPKGS) && go vet -tags testclock $(GOPKGS) && go vet -tags e2e ./e2e/...
	go run $(GOLANGCI_LINT) run $(GOPKGS)
	go run $(GOLANGCI_LINT) run --build-tags testclock,e2e $(GOPKGS) ./e2e/...
	cd web && npm run -s lint && npm run -s typecheck
	shellcheck internal/hosts/install/*.sh
	$(MAKE) psa

PSA_VERSION := 1.24.0
PSA_DIR     := $(CURDIR)/.cache/psscriptanalyzer-$(PSA_VERSION)

psa:
	@test -f $(PSA_DIR)/PSScriptAnalyzer.psd1 || { mkdir -p $(PSA_DIR) && curl -fsSL https://www.powershellgallery.com/api/v2/package/PSScriptAnalyzer/$(PSA_VERSION) -o $(PSA_DIR).nupkg && unzip -q -o $(PSA_DIR).nupkg -d $(PSA_DIR); }
	@# docker cp, not bind mounts: a CI job container's paths do not exist on the Docker host.
	@id=$$(docker create mcr.microsoft.com/powershell:7.5-debian-12 pwsh -NoProfile -File /lint.ps1) && \
		docker cp "$(CURDIR)/internal/hosts/install" $$id:/scripts && docker cp "$(PSA_DIR)" $$id:/psa && \
		docker cp "$(CURDIR)/lint-pwsh.ps1" $$id:/lint.ps1 && \
		{ docker start -a $$id; rc=$$?; docker rm $$id >/dev/null; exit $$rc; }

unit: web-install
	go test $(GOPKGS) && go test -tags testclock ./cmd/platform
	cd web && npm test -s

e2e:
	$(COMPOSE) --profile test up --build --abort-on-container-exit --exit-code-from tests tests; s=$$?; $(COMPOSE) --profile test down -v --remove-orphans --rmi local; exit $$s

# Screenshots come out with `compose cp`, not a bind mount: a CI job container's paths do not exist on the Docker host.
ui:
	$(COMPOSE) --profile ui up --build --wait -d platform-prod
	$(COMPOSE) exec -T postgres psql -U postgres -d platform_prod -q -c "insert into users (email, name, is_admin) values ('jess@halcyon.example', 'Jess Okafor', true) on conflict (email) do nothing"
	$(COMPOSE) --profile ui up --exit-code-from ui --abort-on-container-exit ui; s=$$?; \
		mkdir -p web/e2e/screens && $(COMPOSE) cp ui:/web/e2e/screens/. web/e2e/screens; \
		[ $$s -eq 0 ] || $(COMPOSE) cp ui:/web/test-results web/test-results; \
		$(COMPOSE) --profile ui down -v --remove-orphans --rmi local; exit $$s

web-install:
	cd web && npm ci --no-audit --no-fund

image:
	docker build -t $(IMAGE) .

down:
	$(COMPOSE) --profile test --profile ui down -v --remove-orphans --rmi local
