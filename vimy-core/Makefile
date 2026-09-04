include .env
export

.PHONY: build run clean generate rules

generate:
	PATH="$(HOME)/go/bin:$(PATH)" baml-cli generate
	cd store && PATH="$(HOME)/go/bin:$(PATH)" sqlc generate
	PATH="$(HOME)/go/bin:$(PATH)" templ generate

# Compiles the vimyc rule sets into the artifacts the binary embeds. Kept out of
# `generate` and the committed output checked in, so an ordinary build needs no
# Rust toolchain.
VIMYC ?= ../../vimyc

rules:
	cd $(VIMYC) && PATH="$(HOME)/.cargo/bin:$(PATH)" cargo run --quiet -- \
		rules/seed.vy --json > $(CURDIR)/rules/seed_rules.json.tmp
	mv $(CURDIR)/rules/seed_rules.json.tmp $(CURDIR)/rules/seed_rules.json

build:
	go build -o bin/vimy .

run:
	go run . $(ARGS)

clean:
	rm -rf bin/
