BIN := ../bin/mermaid-server

.PHONY: build run test vet

build:
	go build -o $(BIN) .

# Run from the repo root so relative DATA_DIR/STATIC_DIR resolve like systemd.
run: build
	cd .. && ./bin/mermaid-storage

test:
	go test ./... -count=1

vet:
	go vet ./...
