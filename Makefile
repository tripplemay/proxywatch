.PHONY: build test lint clean run web-build build-all

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

web-build:
	cd web && npm install && npm run build
	rm -rf internal/web/dist
	cp -r web/dist internal/web/dist
	# Restore the .gitkeep so go:embed always has the directory
	touch internal/web/dist/.gitkeep

build-all: web-build build
