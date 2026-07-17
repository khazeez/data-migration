BINARY=sheet-loader
GOBUILD=go build
GOCLEAN=go clean
GOTEST=go test
GOMOD=go mod

.PHONY: all build clean test run dry-run list

all: build

build:
	$(GOBUILD) -o $(BINARY) ./cmd/

clean:
	$(GOCLEAN)
	rm -f $(BINARY)

test:
	$(GOTEST) -v ./internal/...

run: build
	./$(BINARY) -job master-data -config configs/app.yml

dry-run: build
	./$(BINARY) -job master-data -config configs/app.yml -dry-run

list: build
	./$(BINARY) -list -config configs/app.yml

deps:
	$(GOMOD) tidy

fmt:
	go fmt ./...
