.PHONY: build run run-test run-all stop lint test vet fmt tidy docker-build docker-run clean

APP_NAME := nil-loader
TEST_SVC := testservice

## Build

build:
	go build -o bin/$(APP_NAME) ./cmd/server/
	go build -o bin/$(TEST_SVC) ./cmd/testservice/

## Run

run: build
	./bin/$(APP_NAME)

run-test: build
	./bin/$(TEST_SVC)

run-all: build
	./bin/$(TEST_SVC) & echo $$! > .testservice.pid
	sleep 1
	./bin/$(APP_NAME)
	@if [ -f .testservice.pid ]; then kill $$(cat .testservice.pid) 2>/dev/null; rm -f .testservice.pid; fi

stop:
	@lsof -ti :8080 | xargs kill -9 2>/dev/null || true
	@lsof -ti :50051 | xargs kill -9 2>/dev/null || true
	@rm -f .testservice.pid
	@echo "Stopped nil-loader (8080) and testservice (50051)"

## Quality

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./...

vet:
	go vet ./...

fmt:
	goimports -w .
	go fmt ./...

test:
	go test -v -race -count=1 ./...

test-cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

check: fmt vet lint test

## Docker

docker-build:
	docker build -t $(APP_NAME) .

docker-run:
	docker run --rm -p 8080:8080 -p 50051:50051 $(APP_NAME)

docker-run-all:
	docker run --rm -p 8080:8080 -p 50051:50051 $(APP_NAME) sh -c "testservice & nil-loader"

## Clean

clean:
	rm -rf bin/ coverage.out .testservice.pid
