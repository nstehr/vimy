# Core and Currie boundaries

The runtime and analysis tool remain separate Go modules. Currie imports the
core's domain types, archive reader, rule format, and compiler client.

## Runtime

- `agent/strategist.go` schedules model work and gathers observations.
  `situation.go` constructs model inputs; `replan.go` contains the replan policies.
- `agent/activation.go` prepares doctrines without touching the running engine,
  then activates them and closes the history window together. A generation token
  rejects responses from a reset game or a superseded directive.
- `rules/activation.go` installs rules, preferences, targeting policy, and firing
  windows between evaluations. When both engine locks are required, take
  `memMu` before `mu`. Never hold either lock while calling a model or compiler.
- `StrategicSignals` carries observations independently of doctrine compilation.
  `DoctrinePolicy` changes only when its rules become active. Tactical memory is
  still internal map-backed state; migrate further domains as their APIs settle.
  `IntelSnapshot` and `ThreatSnapshot` return owned data for external readers.
- `rules/actions_*.go` and `rules/env_*.go` group actions and predicates by gameplay
  responsibility, within one package. Predicate names remain the `.vy` interface.
- `RunAction` reports commands successfully sent and explicit memory effects.
  Actions depend on `CommandSender`; matching an exclusive rule still blocks its
  category even when the action produces no effect.

## Compiler and recordings

`vimyc` owns source selection, frozen source bundles, executable/source digests,
and cancellable subprocess execution. The default per-invocation timeout is ten
seconds. The seed program is separate from the doctrine source bundle.

New sampled exports include `rule_sets`, keyed by the active fingerprint. Each
compiled doctrine record contains its artifact, doctrine, source bundle and
source/compiler digests. Existing export readers can ignore this additive field.
Older recordings remain readable and cannot acquire missing historical provenance.

Currie continues to analyze against the selected rule sources. It validates
recorded artifacts when present and reports differences from recorded source or
compiler identities. Fingerprint mismatches retain the existing approximate
pairing fallback. Recording original inputs does not imply an old compiler is
available or that a current-rules analysis is a historical simulation.

## Currie

- HTTP handlers delegate replay, enrichment, readings, sweeps, and investigations
  to `analysisService`.
- `jobRunner` permits two running jobs and 64 outstanding jobs. Each job has a
  deadline. Closing the service cancels and joins jobs before closing its cache.
  A browser disconnect cancels its wait, not shared paid work.
- `jobSet` deduplicates concurrent requests. A subsequent start retries failed
  results and investigations that were not settled; polling keeps the old result
  visible until that start. Completed results are evicted as the cache fills.
- Replay identities include the game, export contents, archived doctrines and
  firings, frozen sources, compiler digest, and analysis version. The same identity
  scopes derived jobs and persistent readings. Bump `analysisVersion` when the
  interpretation or model input changes.
- `telemetryReader` supplies common field queries to the SVG view and model tools.
  Query failure, no recorded feed, and an observed field remain distinct. Sparse
  threat recordings cannot prove that an entirely empty feed was running.
