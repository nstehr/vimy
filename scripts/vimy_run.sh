#!/usr/bin/env bash
# Play a game with telemetry on.
#
# LIVES IN THE REPO NOW. The original was at /tmp/vimy_run.sh and macOS purged
# it twice -- the second time on 2026-10-05, mid-way through banking a cohort,
# which would have cost the run if the directive had not been recoverable from
# a sidecar log. The directive below is part of the experiment, not a
# convenience: every game from 157 onward was played with it, and changing a
# word of it invalidates comparison against all of them.
#
# The directive is the one games 157-183 were played with, with ONE sentence
# changed: "Use aircraft for scouting and support only." became the air-as-a-
# striking-arm paragraph below. Everything else is byte-identical, so the
# comparison against those games is controlled.
#
# Why: Vimy has played Allied in all 14 recent games against a Soviet opponent
# in all 14. Its heaviest ground unit is the medium tank at 850; theirs is the
# mammoth at 2000, and produce-heavy-vehicle can only ever buy a medium tank
# for an Allied faction because HeavyTank is 3tnk, a Soviet actor. So the army
# value gap of 1.7x to 2.8x is not a production problem and cannot be closed on
# the ground. mh60 at 1500 needs only a helipad and medium tech and is already
# reachable; the Longbow at 2000 also wants a tech centre and probably is not.
#
# `go build`, not `go run`: the Go toolchain stamps vcs.revision and
# vcs.modified into a BUILT binary and not into `go run`, and rules/version.go
# reads them through debug.ReadBuildInfo. Without that, the session streams
# with an empty `revision` and Currie loses half the version dimension -- which
# is the one thing that makes "did that change move the number" answerable.
#
# vimyc must be on PATH: it compiles each doctrine, and the rules digest is a
# fingerprint of what it produces.
#
# The environment comes from vimy-core/.env, which is where the model key lives.
# vimy-core/Makefile gets it via `include .env` plus a bare `export`, and this
# script runs ./bin/vimy directly rather than through make -- so it has to do
# the same job itself. `set -a` is what makes the assignments exported rather
# than merely local, which is the difference between the strategist having a key
# and silently falling back.
set -euo pipefail

VIMY_DIR="${VIMY_DIR:-/Users/nstehr/code/vimy}"
VIMYC_BIN="${VIMYC_BIN:-/Users/nstehr/code/vimyc/target/release}"

DIRECTIVE="Combined arms, pressing forward. Medium tanks and artillery are the core; build enough infantry to screen them, and treat either one alone as a failure of the plan. Spend what you earn. Cash sitting at zero while production is idle means the refineries have outrun the factories - stop adding harvesters and refineries once it is the army limiting you rather than the income. Commit when you arrive. A squad that reaches the enemy base and turns back has paid the whole price of the attack and taken none of the ground, which is worse than never setting out. Press the attack once the army is formed and in position. Pull back when the base behind you is genuinely under threat, not because the enemy in front of you is strong - at home they always will be. Aircraft are a striking arm, not only eyes. We field the medium tank at 850 credits against their mammoth at 2000, so we do not win a ground armour trade with them and should stop trying to - build helipads and send helicopters at their armour and their buildings. Keep a scout alive as well, but air is not support any more."

cd "$VIMY_DIR/vimy-core"

ENV_FILE="${ENV_FILE:-$VIMY_DIR/vimy-core/.env}"
if [ -f "$ENV_FILE" ]; then
  set -a
  # shellcheck disable=SC1090
  . "$ENV_FILE"
  set +a
  echo "env: sourced $(grep -cE "^[A-Za-z_][A-Za-z0-9_]*=" "$ENV_FILE") variable(s) from ${ENV_FILE/#$HOME/~}"
else
  echo "WARNING: no $ENV_FILE -- the strategist has no model key and will not plan" >&2
fi

if [ -z "${OPENAI_API_KEY:-}" ] && [ -z "${ANTHROPIC_API_KEY:-}" ]; then
  echo "WARNING: neither OPENAI_API_KEY nor ANTHROPIC_API_KEY is set" >&2
fi

if [ ! -x "$VIMYC_BIN/vimyc" ]; then
  echo "no vimyc at $VIMYC_BIN/vimyc -- build it first (cargo build --release in the vimyc repo)" >&2
  exit 1
fi

# A dirty tree stamps vcs.modified=true, and every session so far has carried
# it. stream_sessions.modified is not a footnote: a before/after that spans one
# of these is not an answer, so say so loudly rather than discovering it in the
# cohort table afterwards.
if [ -n "$(git -C "$VIMY_DIR" status --porcelain)" ]; then
  echo "WARNING: the tree is dirty, so this run streams as modified=true and cannot" >&2
  echo "         serve as a before or an after. git status:" >&2
  git -C "$VIMY_DIR" status --short >&2
  echo >&2
fi

go build -o bin/vimy .
echo "revision: $(git -C "$VIMY_DIR" rev-parse --short=12 HEAD)"

# PATH is prepended in place rather than through `env PATH=...`, so everything
# sourced above stays in the environment the binary inherits.
export PATH="$VIMYC_BIN:$PATH"
exec ./bin/vimy -stream -doctrine "$DIRECTIVE" "$@"
