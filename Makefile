.DEFAULT_GOAL := help
.PHONY: help build install sbom browser-tools deploy check test lint clean
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
	@echo "check         Run documentation, vet, unit, integration and race checks"
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
test:
	go test ./...
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
	python3 -B tools/spec_sources.py --verify
	python3 -B tools/client_scope.py
	go test -race ./...
clean:
	go clean
	rm -f bin/tk
