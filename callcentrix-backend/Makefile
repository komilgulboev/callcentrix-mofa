BINARY=callcentrix

.PHONY: build run tidy

tidy:
	go mod tidy

build:
	go build -o $(BINARY) ./cmd/server

run:
	@export $$(cat .env | grep -v '^#' | xargs) && go run ./cmd/server

run-binary:
	@export $$(cat .env | grep -v '^#' | xargs) && ./$(BINARY)

linux:
	GOOS=linux GOARCH=amd64 go build -o $(BINARY)-linux ./cmd/server
