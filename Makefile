.PHONY: build run check fmt vet test lint doctor calibrate clean

BIN    := bin/wb
MODULE := github.com/solomonadzape95/workbench

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X $(MODULE)/internal/version.Version=$(VERSION) \
	-X $(MODULE)/internal/version.Commit=$(COMMIT) \
	-X $(MODULE)/internal/version.Date=$(DATE)

build:
	@mkdir -p bin
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/workbench

run:
	go run ./cmd/workbench

doctor:
	go run ./cmd/workbench doctor

calibrate:
	WORKBENCH_WRITE_CALIBRATION=1 go test ./internal/detect -run TestCalibrationReport -v

fmt:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

vet:
	go vet ./...

test:
	go test -race ./...

check: fmt vet test

lint:
	golangci-lint run

clean:
	rm -rf bin
