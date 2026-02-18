include .env
export

.PHONY: build run clean generate

generate:
	PATH="$(HOME)/go/bin:$(PATH)" baml-cli generate

build:
	go build -o bin/vimy .

run:
	go run .

clean:
	rm -rf bin/
