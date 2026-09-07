include .env
export

.PHONY: build run clean generate rules check-rules

generate:
	PATH="$(HOME)/go/bin:$(PATH)" baml-cli generate
	cd store && PATH="$(HOME)/go/bin:$(PATH)" sqlc generate
	PATH="$(HOME)/go/bin:$(PATH)" templ generate

# Compiles `rules/vy/seed.vy` into the artifact the binary embeds, and
# regenerates the token tables the dashboard highlights with. The rule sources
# live here; vimyc is only the compiler. Kept out of `generate` and the output
# committed, so an ordinary build needs no Rust toolchain.
VIMYC ?= ../../vimyc

rules:
	cd $(VIMYC) && PATH="$(HOME)/.cargo/bin:$(PATH)" cargo run --quiet -- \
		$(CURDIR)/rules/vy/seed.vy --json > $(CURDIR)/rules/seed_rules.json.tmp
	mv $(CURDIR)/rules/seed_rules.json.tmp $(CURDIR)/rules/seed_rules.json
	cd $(VIMYC) && PATH="$(HOME)/.cargo/bin:$(PATH)" cargo build --release --quiet
	./server/views/gen_tokens.sh $(VIMYC)/target/release/vimyc \
		$(CURDIR)/server/views/vy_tokens.go
	gofmt -w server/views/vy_tokens.go

# Type-checks the doctrine rule set the way a game compiles it: every `.vy` in
# `rules/vy/` bar the seed, as one unit. Warnings — shared priorities, shadowed
# rules — print and do not fail, the same as at runtime.
check-rules:
	@$(VIMYC)/target/release/vimyc \
		$(filter-out %/seed.vy,$(wildcard $(CURDIR)/rules/vy/*.vy))
	@echo "rule set ok"

build:
	go build -o bin/vimy .

run:
	go run . $(ARGS)

clean:
	rm -rf bin/
