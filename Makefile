APP_NAME := spectra-watch

.PHONY: build run fmt tidy clean

build:
	GO111MODULE=on go build -o bin/$(APP_NAME) ./cmd/watcher

run: build
	./bin/$(APP_NAME)

fmt:
	gofmt -w cmd internal

tidy:
	go mod tidy

clean:
	rm -rf bin
