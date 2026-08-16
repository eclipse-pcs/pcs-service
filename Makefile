.PHONY: build test-unit test-shell test-all

build:
	mkdir -p bin
	go build -o bin/pcs-service ./cmd/pcs-service
	go build -o bin/pcs-split ./cmd/pcs-split
	go build -o bin/pcs-merge ./cmd/pcs-merge

test-unit:
	go test -count=1 ./...

test-shell:
	cd test && ./setup.sh && ./compare_all.sh

test-all: test-unit test-shell
