.PHONY: build test lint clean run

BINARY := proxywatch
GOFLAGS :=

build:
	go build $(GOFLAGS) -o bin/$(BINARY) ./cmd/proxywatch

test:
	go test -race ./...

lint:
	go vet ./...
	gofmt -l . | tee /dev/stderr | (! read)

run: build
	./bin/$(BINARY)

clean:
	rm -rf bin/ dist/ web/dist/
