# Currie needs vimyc on PATH to compile a doctrine, and the .vy sources to
# replay against. Both are siblings of this directory in a normal checkout.
VIMYC ?= ../../vimyc
RULES ?= ../vimy-core/rules/vy
ADDR  ?= :8090
DIR   ?= ~/.vimy
CH    ?= http://localhost:8123

.PHONY: run build test test-ch clean generate

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

# The queries in clickhouse.go and live.go are strings, so the compiler checks
# none of them: a typo is a section that silently disappears from the page.
# This runs them against a real server. Needs `make -C clickhouse up` and a
# game that has been streamed; skips cleanly without either.
#
# Run it after touching any SQL. Three of the four bugs found writing this
# read path were invisible to `go test` and to review: an alias that shadowed
# its own column, a monotonic function ClickHouse moved inside an aggregate,
# and a WITH alias the analyzer would not resolve.
test-ch:
	CURRIE_CH=$(CH) go test ./ -run TestLiveQueries -v

clean:
	rm -f currie
