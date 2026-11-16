BIN_DIR ?= bin

.PHONY: build test check e2e coverage

build:
	go build -mod=readonly -trimpath -o "$(BIN_DIR)/" ./cmd/...

test:
	go test -mod=readonly -race ./...

check:
	go mod tidy -diff
	go vet -mod=readonly ./...

e2e:
	bash tests/e2e/run.sh

coverage:
	go test -mod=readonly -race -coverpkg=./... -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
