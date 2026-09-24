package ch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// fake stands in for ClickHouse. It records what the client asked for, because
// most of what this package does is get the request right.
type fake struct {
	*httptest.Server
	last   url.Values
	body   string
	status int
}

func newFake(body string) *fake {
	f := &fake{body: body, status: http.StatusOK}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.last = r.URL.Query()
		w.WriteHeader(f.status)
		_, _ = w.Write([]byte(f.body))
	}))
	return f
}

type row struct {
	Rule  string `json:"rule"`
	Fired int64  `json:"fired"`
}

func TestQueryDecodesEachRow(t *testing.T) {
	f := newFake("{\"rule\":\"build-power\",\"fired\":3}\n{\"rule\":\"deploy-mcv\",\"fired\":1}\n")
	defer f.Close()

	rows, err := Query[row](context.Background(), New(f.URL, "currie", "u", "p"),
		`SELECT rule, fired FROM stream_rule_evals`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Rule != "build-power" || rows[1].Fired != 1 {
		t.Fatalf("got %+v", rows)
	}
}

// The client must ask for unquoted 64-bit integers. ClickHouse quotes them by
// default, which decodes into an int64 field as a type error rather than a
// number -- and every count these queries return is a 64-bit integer.
func TestQuerySendsSettings(t *testing.T) {
	f := newFake("")
	defer f.Close()

	if _, err := Query[row](context.Background(), New(f.URL, "currie", "u", "p"), `SELECT 1`, nil); err != nil {
		t.Fatal(err)
	}
	if got := f.last.Get("output_format_json_quote_64bit_integers"); got != "0" {
		t.Errorf("64-bit quoting = %q, want 0", got)
	}
	// readonly=2 permits SELECT and per-query settings and refuses everything
	// that writes. It is the guard that has to already be in place before a
	// model is choosing the SQL.
	if got := f.last.Get("readonly"); got != "2" {
		t.Errorf("readonly = %q, want 2", got)
	}
	if got := f.last.Get("database"); got != "currie" {
		t.Errorf("database = %q", got)
	}
}

// Parameters are bound server-side. Nothing is interpolated into the statement.
func TestQueryBindsParams(t *testing.T) {
	f := newFake("")
	defer f.Close()

	_, err := Query[row](context.Background(), New(f.URL, "currie", "u", "p"),
		`SELECT rule FROM t WHERE session_id = {session:String} AND tick >= {swap:UInt32}`,
		map[string]any{"session": "abc-123", "swap": 4400})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.last.Get("param_session"); got != "abc-123" {
		t.Errorf("param_session = %q", got)
	}
	if got := f.last.Get("param_swap"); got != "4400" {
		t.Errorf("param_swap = %q", got)
	}
	if q := f.last.Get("query"); !contains(q, "{session:String}") {
		t.Errorf("the placeholder should reach the server verbatim: %q", q)
	}
}

func TestQueryAppendsFormat(t *testing.T) {
	f := newFake("")
	defer f.Close()

	if _, err := Query[row](context.Background(), New(f.URL, "", "", ""), "SELECT 1;", nil); err != nil {
		t.Fatal(err)
	}
	if q := f.last.Get("query"); !contains(q, "FORMAT JSONEachRow") {
		t.Errorf("query = %q", q)
	}
	// A caller that named a format keeps it.
	if _, err := Query[row](context.Background(), New(f.URL, "", "", ""), "SELECT 1 FORMAT TabSeparated", nil); err != nil {
		t.Fatal(err)
	}
	if q := f.last.Get("query"); contains(q, "JSONEachRow") {
		t.Errorf("an explicit format should be left alone: %q", q)
	}
}

func TestQueryReportsServerError(t *testing.T) {
	f := newFake("Code: 47. Unknown expression identifier")
	f.status = http.StatusBadRequest
	defer f.Close()

	_, err := Query[row](context.Background(), New(f.URL, "", "", ""), "SELECT nope", nil)
	if err == nil {
		t.Fatal("want an error")
	}
	if !contains(err.Error(), "Unknown expression identifier") {
		t.Errorf("the server's own message should survive: %v", err)
	}
}

// One distinguishes an empty answer from a failed one. A game played before
// -stream has no session, which is an answer, and treating it as a failure
// would take the rest of the report down with it.
func TestOneNoRows(t *testing.T) {
	f := newFake("")
	defer f.Close()

	_, err := One[row](context.Background(), New(f.URL, "", "", ""), "SELECT 1", nil)
	if !errors.Is(err, ErrNoRows) {
		t.Fatalf("err = %v, want ErrNoRows", err)
	}
}

// The page runs these while a reader waits, so a server that is up but
// grinding has to fail rather than hang.
func TestQueryTimesOut(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
	}))
	defer slow.Close()

	c := New(slow.URL, "", "", "")
	c.Timeout = 20 * time.Millisecond
	if _, err := Query[row](context.Background(), c, "SELECT 1", nil); err == nil {
		t.Fatal("want a timeout")
	}
}

func TestNilClient(t *testing.T) {
	if _, err := Query[row](context.Background(), nil, "SELECT 1", nil); err == nil {
		t.Fatal("a nil client should be an error, not a panic")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// Every query runs under caps, not only the ones a human wrote.
//
// readonly=2 stops a write; these stop a SELECT that is merely enormous, which
// is the other half of handing the SQL to something that does not know how big
// a table is. stream_rule_evals alone holds ten million rows.
func TestQueryIsBounded(t *testing.T) {
	f := newFake("")
	defer f.Close()

	if _, err := Query[row](context.Background(), New(f.URL, "currie", "u", "p"), `SELECT 1`, nil); err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{
		"max_result_rows":      "500",
		"result_overflow_mode": "break",
		"read_overflow_mode":   "break",
	} {
		if got := f.last.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	if f.last.Get("max_rows_to_read") == "" {
		t.Error("no scan cap: an unbounded read of the eval stream is a minute of CPU")
	}
	// Truncating beats failing: a hundred rows of an answer is worth more than
	// an error message, and the model cannot see how big a table was.
	if f.last.Get("result_overflow_mode") != "break" {
		t.Error("overflow must truncate rather than error")
	}
}
