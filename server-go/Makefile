.PHONY: build run test vet

build:
	go build -o bin/mermaid-storage .

run: build
	./bin/mermaid-storage

test:
	go test ./... -count=1

vet:
	go vet ./...
