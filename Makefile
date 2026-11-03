.DEFAULT_GOAL := help
.PHONY: help build install sbom browser-tools deploy db-up db-down check check-race race-check check-full test lint clean
DEPLOY_HOST ?= exedev@teleport.engineering
DEPLOY_DIR ?= /home/exedev/teleport
DEPLOY_BUILDER ?= auto
DEPLOY_ADDR ?= 0.0.0.0:8000
export DEPLOY_HOST DEPLOY_DIR DEPLOY_BUILDER DEPLOY_ADDR DEPLOY_PUBLIC_HOSTS
help:
	@echo ""
	@echo "build         Build bin/tk with a local build identity"
	@echo "install       Build and install tk into GOBIN or GOPATH/bin"
	@echo "sbom          Generate sbom.cdx.json for Go and bundled browser dependencies"
	@echo "browser-tools Prepare the pinned persistent browser client"
	@echo "deploy        Build Linux tk, scp to /home/exedev/teleport and start it"
	@echo "db-up         Start the private local PostgreSQL installation"
	@echo "db-down       Stop local PostgreSQL without deleting data"
	@echo "check         Run everyday documentation, vet, unit and integration checks"
	@echo "check-race    Run the full Go race suite at low priority (optional locally)"
	@echo "race-check    Alias for check-race"
	@echo "check-full    Run everyday checks, then the race suite (CI/release gate)"
	@echo "test          Run Go tests"
	@echo "lint          Verify Go formatting and run vet"
	@echo "clean         Remove generated binary"
	@echo ""
build:
	python3 tools/build.py
install: build
	python3 tools/build.py --install-only
sbom:
	python3 -B tools/sbom.py
browser-tools:
	python3 -B tools/browser_setup.py
deploy:
	python3 -B tools/deploy.py
db-up:
	python3 -B tools/postgres.py up
db-down:
	python3 -B tools/postgres.py down
test:
	python3 -B tools/go_check.py
lint:
	@test -z "$$(gofmt -l cmd internal tools)"
	go vet ./...
	python3 -B tools/web_check.py
check: lint
	python3 -B tools/test_build.py
	python3 -B tools/test_sbom.py
	python3 -B tools/test_tool_history.py
	python3 -B tools/test_timing.py
	python3 -B tools/test_smoke_timeout.py
	python3 -B tools/test_smoke_profile.py
	python3 -B tools/test_browser_driver.py
	python3 -B tools/test_source_cache.py
	python3 -B tools/test_deploy.py
	python3 -B tools/test_go_check.py
	python3 -B tools/test_postgres.py
	python3 -B tools/spec_sources.py --verify
	python3 -B tools/client_scope.py
	python3 -B tools/go_check.py
check-race:
	python3 -B tools/go_check.py --race
race-check: check-race
check-full: check
	$(MAKE) check-race
clean:
	go clean
	rm -f bin/tk
