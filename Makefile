.DEFAULT_GOAL := help
.PHONY: help build build-debug install sbom browser-tools deploy deploy-reset db-up db-down check check-go check-integration check-race race-check check-full test lint clean
DEPLOY_HOST ?= exedev@teleport.engineering
DEPLOY_DIR ?= /home/exedev/teleport
DEPLOY_BUILDER ?= auto
DEPLOY_ADDR ?= 0.0.0.0:8000
export DEPLOY_HOST DEPLOY_DIR DEPLOY_BUILDER DEPLOY_ADDR DEPLOY_PUBLIC_HOSTS
export CONFIRM
help:
	@echo ""
	@echo "build         Build bin/tk with a local build identity"
	@echo "build-debug   Build bin/tk with native debugging information"
	@echo "install       Build and install tk into GOBIN or GOPATH/bin"
	@echo "sbom          Generate sbom.cdx.json for Go and bundled browser dependencies"
	@echo "browser-tools Prepare the pinned persistent browser client"
	@echo "deploy        Build Linux tk, scp to /home/exedev/teleport and start it"
	@echo "deploy-reset  Replace the remote installation after CONFIRM=reset; retain a backup"
	@echo "db-up         Start the private local PostgreSQL installation"
	@echo "db-down       Stop local PostgreSQL without deleting data"
	@echo "check         Lightweight source and unit checks; no database or browser processes"
	@echo "check-go      Explicit Go tests with a disposable database, at low priority"
	@echo "check-integration  Explicit vet, Go/database, browser and process/deadline checks"
	@echo "check-race    Run the full Go race suite at low priority (optional locally)"
	@echo "race-check    Alias for check-race"
	@echo "check-full    Run lightweight, integration and race checks in order"
	@echo "test          Run Go tests"
	@echo "lint          Verify Go formatting and run vet"
	@echo "clean         Remove generated binary"
	@echo ""
build:
	python3 tools/build.py
build-debug:
	python3 tools/build.py --debug
install: build
	python3 tools/build.py --install-only
sbom:
	python3 -B tools/sbom.py
browser-tools:
	python3 -B tools/browser_setup.py
deploy:
	python3 -B tools/deploy.py
deploy-reset:
	python3 -B tools/deploy.py --reset
db-up:
	python3 -B tools/postgres.py up
db-down:
	python3 -B tools/postgres.py down
test check-go:
	python3 -B tools/go_check.py
lint:
	python3 -B tools/check.py --lint
check:
	python3 -B tools/check.py
check-integration:
	python3 -B tools/check.py --integration
check-race:
	python3 -B tools/go_check.py --race
race-check: check-race
check-full:
	$(MAKE) check
	$(MAKE) check-integration
	$(MAKE) check-race
clean:
	go clean
	rm -f bin/tk
