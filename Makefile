# agones-check developer tasks. Run `make help` for the list.
GO ?= go
BIN := bin
KUBECONFIG ?= $(HOME)/.kube/config

.PHONY: help build plugin test cover lint bench perf oracle e2e examples clean

help: ## list targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-10s %s\n", $$1, $$2}'

build: ## build bin/agones-check and the kubectl plugin
	$(GO) build -o $(BIN)/agones-check ./cmd/agones-check
	cp $(BIN)/agones-check $(BIN)/kubectl-agones_check

plugin: build ## install the kubectl plugin into GOPATH/bin (use as `kubectl agones-check`)
	cp $(BIN)/kubectl-agones_check $$($(GO) env GOPATH)/bin/

test: ## unit tests
	$(GO) vet ./...
	$(GO) test ./...

cover: ## unit tests with coverage
	$(GO) test -coverprofile=coverage.out ./internal/... ./cmd/agones-check
	$(GO) tool cover -func=coverage.out | tail -1

bench: ## mutation benchmark (RQ1, RQ2) -> bench/results/mutation.md
	$(GO) run ./cmd/bench -out bench/results

perf: ## performance benchmark (RQ3) -> bench/results/perf.md
	$(GO) run ./cmd/bench -perf -out bench/results
	./scripts/cli-timing.sh

oracle: bench ## dry-run every mutant on the current cluster and compare -> bench/results/oracle.md
	$(GO) run ./cmd/oracle -mutants bench/mutants > bench/results/oracle.tsv
	$(GO) run ./cmd/bench -oracle bench/results/oracle.tsv -out bench/results

e2e: ## runtime checks on a kind cluster with Agones (scripts/e2e-runtime.sh)
	./scripts/e2e-runtime.sh

examples: build ## run the tool on Agones' examples, one file at a time
	@for f in $$(find testdata/agones-examples -name '*.yaml' | sort); do \
	  $(BIN)/agones-check -e prod,dev,beta-off --fail-on never $$f | tail -1 | sed "s|^|$$f: |"; done

clean:
	rm -rf $(BIN) coverage.out bench/mutants
