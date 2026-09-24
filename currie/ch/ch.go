// Package ch is Currie's read path into ClickHouse.
//
// The write path already existed -- `stream.Shipper` moves the sidecar's
// write-ahead log in -- and for a while nothing read back out, so every
// question answered from these tables was answered by a human running a
// `make` target. Meanwhile the report page computed its blame from the
// 1-in-15 export sample, which the archive's own README says cannot be
// counted. This package is what lets the page ask the unsampled table.
//
// Deliberately small: an HTTP client, server-side parameters, and a generic
// decode. The queries themselves belong to whoever asks them, not here.
package ch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrNoRows is what One returns when the query selected nothing. Separate from
// an error, because "no session for this game" is an answer -- a game played
// before -stream has none -- and treating it as a failure would take the rest
// of the page down with it.
var ErrNoRows = errors.New("ch: no rows")

// Client talks to ClickHouse over HTTP.
//
// The same endpoint, database and credentials the shipper uses, on purpose:
// one flag set configures both halves, and a read that works proves the write
// that follows will reach the same server.
type Client struct {
	Endpoint string
	Database string
	User     string
	Password string
	HTTP     *http.Client

	// Timeout bounds one query. The page runs these synchronously while a
	// reader waits, so a server that is up but grinding has to fail rather
	// than hang: a report without its stream sections still carries the blame
	// analysis, which is what the reader came for.
	Timeout time.Duration
}

// New builds a client with Currie's defaults.
func New(endpoint, database, user, password string) *Client {
	return &Client{
		Endpoint: strings.TrimRight(endpoint, "/"),
		Database: database,
		User:     user,
		Password: password,
		HTTP:     &http.Client{},
		Timeout:  5 * time.Second,
	}
}

// Ping checks the endpoint and that the streamed tables exist.
//
// Both, not just the endpoint: a reachable server with no `04_stream.sql`
// loaded answers every later query with an error that reads like a bug in the
// query. Failing here instead turns that into one warning at startup.
func (c *Client) Ping(ctx context.Context) error {
	_, err := Query[struct {
		N int64 `json:"n"`
	}](ctx, c, `SELECT count() AS n FROM stream_sessions`, nil)
	return err
}

// Query runs sql and decodes each result row into a T.
//
// Parameters are bound server-side: write `{game:UInt32}` in the SQL and pass
// `map[string]any{"game": id}`. Nothing is interpolated into the statement,
// which matters less today -- every caller is in this repo -- than it will
// when the model is choosing the arguments.
// The caps a query runs under.
//
// maxResultRows is what can come back: enough for a tick-banded summary or a
// few hundred units, far short of a table. maxRowsToRead is what it may scan
// getting there - stream_rule_evals alone holds ten million rows, and an
// unbounded scan of it is a minute of CPU for an answer nobody can read.
const (
	maxResultRows = 500
	maxRowsToRead = 50_000_000
)

func Query[T any](ctx context.Context, c *Client, sql string, params map[string]any) ([]T, error) {
	if c == nil {
		return nil, errors.New("ch: no client configured")
	}
	if c.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
	}

	q := url.Values{}
	q.Set("query", withFormat(sql))
	if c.Database != "" {
		q.Set("database", c.Database)
	}
	// 64-bit integers arrive as JSON strings by default, which decodes into an
	// int64 field as a type error rather than a number. Turning the quoting
	// off is safe here because every 64-bit value these queries return is a
	// count, and a count large enough to lose precision as a float would mean
	// something else had already gone wrong.
	q.Set("output_format_json_quote_64bit_integers", "0")
	// readonly=2 permits SELECT and per-query settings and refuses everything
	// that writes. The shipper has its own client and is unaffected. This is
	// the guard that has to already be here when a model starts choosing the
	// SQL, and it costs nothing to set now.
	q.Set("readonly", "2")
	if c.Timeout > 0 {
		q.Set("max_execution_time", fmt.Sprintf("%d", int(c.Timeout.Seconds())))
	}
	// Bounds, applied to every query and not only the model's. readonly=2 stops
	// a write; these stop a SELECT that is merely enormous, which is the other
	// half of handing the SQL to something that does not know how big a table
	// is. result_overflow_mode=break truncates rather than erroring: a hundred
	// rows of an answer beats a failure message.
	q.Set("max_result_rows", fmt.Sprintf("%d", maxResultRows))
	q.Set("result_overflow_mode", "break")
	q.Set("max_rows_to_read", fmt.Sprintf("%d", maxRowsToRead))
	q.Set("read_overflow_mode", "break")

	for k, v := range params {
		q.Set("param_"+k, fmt.Sprint(v))
	}

	// POST with the query in the URL rather than the body: these statements are
	// small, and keeping the body empty leaves it free for the bulk path if a
	// caller ever needs one.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint+"/?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	if c.User != "" {
		req.SetBasicAuth(c.User, c.Password)
	}
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return nil, fmt.Errorf("clickhouse %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	// JSONEachRow is one object per line, so it streams and a partial read
	// reports which row failed rather than failing the document.
	var out []T
	dec := json.NewDecoder(resp.Body)
	for {
		var row T
		if err := dec.Decode(&row); err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("ch: decoding row %d: %w", len(out)+1, err)
		}
		out = append(out, row)
	}
	return out, nil
}

// One runs a query expected to select a single row.
//
// ErrNoRows when it selected none, so a caller can tell an empty answer from a
// failed one without inspecting a slice length and guessing.
func One[T any](ctx context.Context, c *Client, sql string, params map[string]any) (T, error) {
	var zero T
	rows, err := Query[T](ctx, c, sql, params)
	if err != nil {
		return zero, err
	}
	if len(rows) == 0 {
		return zero, ErrNoRows
	}
	return rows[0], nil
}

// withFormat appends the row format unless the caller named one.
func withFormat(sql string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(sql), ";")
	if strings.Contains(strings.ToUpper(trimmed), " FORMAT ") {
		return trimmed
	}
	return trimmed + "\nFORMAT JSONEachRow"
}
