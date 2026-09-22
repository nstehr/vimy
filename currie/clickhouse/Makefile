# ClickHouse for Currie. Everything here is derived from ~/.vimy -- nothing in
# this directory is a source of truth, so `make reset` is always safe.

COMPOSE := docker compose
CH      := $(COMPOSE) exec -T clickhouse clickhouse-client --user currie --password currie

.PHONY: local up down load query currie reset stream stream-reset ship ship-loop tuning verify

# No server, no ingest, no state. Reads the .json.gz archive in place.
local:
	$(COMPOSE) run --rm local --queries-file /sql/explore.sql

# Interactive clickhouse-local: `make shell-local`
shell-local:
	$(COMPOSE) run --rm local

# The entrypoint boots a temporary server to run the initdb scripts, then stops
# it and execs the real one, so a single successful SELECT does not mean the
# server is up -- it may be the one about to go away. Wait for three in a row.
up:
	$(COMPOSE) up -d clickhouse
	@ok=0; for i in $$(seq 1 120); do \
		if $(CH) --query "SELECT 1" >/dev/null 2>&1; then ok=$$((ok+1)); else ok=0; fi; \
		[ $$ok -ge 3 ] && break; sleep 1; \
	done; \
	[ $$ok -ge 3 ] || { echo "clickhouse did not come up"; exit 1; }
	@echo "clickhouse up -- play UI at http://localhost:8123/play (currie/currie)"

# Re-runnable. Truncates the event tables and reloads from the archive.
load: up
	$(CH) --queries-file /sql/02_load.sql
	@$(CH) --query "SELECT 'rule_evals', count() FROM currie.rule_evals \
	  UNION ALL SELECT 'game_states', count() FROM currie.game_states \
	  UNION ALL SELECT 'doctrines', count() FROM currie.doctrines \
	  UNION ALL SELECT 'games', count() FROM currie.games"

# --- the streamed half -------------------------------------------------------
# Unsampled, shipped from the sidecar's write-ahead log while the game runs.
# See ../stream/wal.go for the contract and ../../currie/README.md for why.
#
# `currie` itself ships in the background now, so neither target below is part
# of playing a game. `ship` is the one-off catch-up for segments written while
# nothing was shipping; `ship-loop` is a headless shipper on a box with no
# server. Both are safe to run alongside the server -- the deduplication token
# is the segment path, so a segment shipped twice lands once.

stream: up
	$(CH) --queries-file /sql/04_stream.sql
	@echo "stream tables ready"

ship: stream
	cd .. && go run . -ship -ship-every 0

ship-loop: stream
	cd .. && go run . -ship -ship-every 5s

# Did that change move the number. The query the archive could not answer.
tuning: up
	$(CH) --queries-file /sql/05_tuning.sql

# Does the stream agree with SQLite's counters for one game? Exact agreement
# expected: the stream is unsampled.
GAME_ID ?= latest
verify: up
	@id="$(GAME_ID)"; \
	if [ "$$id" = "latest" ]; then \
	  id=$$(sqlite3 $(HOME)/.vimy/vimy.db "select max(id) from games"); fi; \
	echo "verifying game $$id"; \
	$(CH) --param_game=$$id --queries-file /sql/06_verify.sql

# The stream tables are derived from the WAL, which is still on disk, so
# dropping and reloading them costs nothing. Needed after a schema change:
# CREATE TABLE IF NOT EXISTS will not add a column to a table that exists.
stream-reset: up
	@for t in stream_sessions stream_events stream_segments; do \
		$(CH) --query "DROP TABLE IF EXISTS currie.$$t SYNC"; done
	$(CH) --queries-file /sql/04_stream.sql
	@echo "stream tables recreated -- re-run make ship to reload from the WAL"

currie: up
	$(CH) --queries-file /sql/03_currie.sql

# make query Q="SELECT ..."
query: up
	@$(CH) --query "$(Q)"

shell: up
	$(COMPOSE) exec clickhouse clickhouse-client --user currie --password currie

down:
	$(COMPOSE) down

# Drops the data volume. The archive is untouched; `make load` rebuilds.
reset:
	$(COMPOSE) down -v
