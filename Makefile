.PHONY: build install test cover vet fmt spec-lint demo clean

BINARY  := cover100
GOBIN   ?= $(shell go env GOPATH)/bin
COVERAGE_FLOOR ?= 100

# build compiles the CLI into ./cover100.
build:
	go build -o $(BINARY) ./cmd/cover100

# install builds cover100 into GOBIN, so the command is named cover100 rather
# than cover100-cli (the installing binary is named after its package
# directory, which is ./cmd/cover100).
install:
	go build -o $(GOBIN)/$(BINARY) ./cmd/cover100

test:
	go test ./...

# cover enforces the same floor CI enforces, so a local run and a pull request
# cannot disagree about whether coverage is acceptable.
cover:
	go test -coverprofile=coverage.out ./...
	@total=$$(go tool cover -func=coverage.out | tail -1 | awk '{print $$NF}' | tr -d '%'); \
	awk -v t="$$total" -v floor="$(COVERAGE_FLOOR)" 'BEGIN { \
		printf "total coverage: %s%% (floor %s%%)\n", t, floor; \
		if (t + 0 < floor + 0) { print "FAIL: coverage is below the floor"; exit 1 } \
	}'
	@rm -f coverage.out

vet:
	go vet ./...

fmt:
	gofmt -l -w .

# spec-lint validates the SpecScore tree. `spec lint` exits 1 when violations
# exist, which is a gate signal rather than a failure to run.
spec-lint:
	specscore spec lint --severity warning

# demo regenerates the committed example report from the sample fixture. It
# needs Node and npm for the fixture's vitest run; see examples/README.md.
demo:
	./scripts/demo.sh

clean:
	rm -f $(BINARY) coverage.out
	rm -rf dist
