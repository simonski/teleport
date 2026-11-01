.DEFAULT_GOAL := help
.PHONY: help build install sbom check test lint clean
help:
	@echo "build    Build bin/tk with a local build identity"
	@echo "install  Build and install tk into GOBIN or GOPATH/bin"
	@echo "sbom     Generate sbom.cdx.json for Go and bundled browser dependencies"
	@echo "check    Run documentation, vet, unit, integration and race checks"
	@echo "test     Run Go tests"
	@echo "lint     Verify Go formatting and run vet"
	@echo "clean    Remove generated binary"
build:
	python3 tools/build.py
install: build
	python3 tools/build.py --install-only
sbom:
	python3 -B tools/sbom.py
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
	python3 -B tools/spec_sources.py --verify
	python3 -B tools/client_scope.py
	go test -race ./...
clean:
	go clean
	rm -f bin/tk
