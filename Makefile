APP_NAME := spectra-watch

.PHONY: build run fmt tidy clean test-term

build:
	GO111MODULE=on go build -o bin/$(APP_NAME) ./cmd/watcher

test-term:
	GO111MODULE=on go build -o bin/termtest ./cmd/termtest
	./bin/termtest

run: build
	./bin/$(APP_NAME)

fmt:
	gofmt -w cmd internal

tidy:
	go mod tidy

clean:
	rm -rf bin
