module github.com/nstehr/vimy/currie

go 1.25.4

// Currie is a separate module because it does not ship in the bot binary, but
// it reuses the archive reader, the doctrine type and the rule-set fingerprint
// rather than reimplementing any of them.
replace github.com/nstehr/vimy/vimy-core => ../vimy-core

require (
	github.com/boundaryml/baml v0.219.0
	github.com/nstehr/vimy/vimy-core v0.0.0-00010101000000-000000000000
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/expr-lang/expr v1.17.8 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mfridman/interpolate v0.0.2 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/pressly/goose/v3 v3.27.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/sethvargo/go-retry v0.3.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	modernc.org/libc v1.70.0 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
	modernc.org/sqlite v1.48.2 // indirect
)
