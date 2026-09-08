# Currie needs vimyc on PATH to compile a doctrine, and the .vy sources to
# replay against. Both are siblings of this directory in a normal checkout.
VIMYC ?= ../../vimyc
RULES ?= ../vimy-core/rules/vy
ADDR  ?= :8090
DIR   ?= ~/.vimy

.PHONY: run build test clean generate

# Regenerate the cache's query code after touching store/queries.sql or
# store/schema.sql. Committed, so an ordinary build needs no sqlc.
generate:
	cd store && PATH="$$HOME/go/bin:$$PATH" sqlc generate

# Reads OPENAI_API_KEY / ANTHROPIC_API_KEY from vimy-core's .env when there is
# one, so the model reads the report alongside you. Without a key the page is
# unchanged minus the prose.
run:
	@set -a; [ -f ../vimy-core/.env ] && . ../vimy-core/.env; set +a; \
	PATH="$(abspath $(VIMYC))/target/release:$$PATH" \
		go run . --dir $(DIR) --rules $(RULES) --addr $(ADDR)

build:
	go build -o currie .

test:
	go test ./...

clean:
	rm -f currie
