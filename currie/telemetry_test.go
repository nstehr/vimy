package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nstehr/vimy/currie/ch"
)

func TestFieldConsumersDistinguishMissingThreatFromQueryFailure(t *testing.T) {
	for _, mode := range []string{"missing", "failure", "observed"} {
		t.Run(mode, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				query := r.URL.Query().Get("query")
				switch {
				case strings.Contains(query, "FROM stream_units"):
					fmt.Fprintln(w, `{"tick":100,"unit_id":1,"side":"ours","type":"e1","x":4,"y":4}`)
				case strings.Contains(query, "count() AS n"):
					if mode == "failure" {
						http.Error(w, "broken query", 500)
						return
					}
					if mode == "missing" {
						fmt.Fprintln(w, `{"n":0}`)
					} else {
						fmt.Fprintln(w, `{"n":1}`)
					}
				default:
					fmt.Fprintln(w, `{"col":1,"row":2,"value":3}`)
				}
			}))
			defer srv.Close()
			client := ch.New(srv.URL, "currie", "", "")
			iv := &investigator{ch: client}
			prose := iv.fieldAt(t.Context(), "session", 100)
			v := &fieldView{TerrainSpan: 64}
			v.loadThreat(t.Context(), client, "session", 100)
			switch mode {
			case "missing":
				if !strings.Contains(prose, "NOT RECORDED") || !strings.Contains(v.ThreatNote, "not recorded") {
					t.Fatalf("%s / %s", prose, v.ThreatNote)
				}
			case "failure":
				if !strings.Contains(prose, "query failed") || !strings.Contains(v.ThreatNote, "query failed") {
					t.Fatalf("%s / %s", prose, v.ThreatNote)
				}
			case "observed":
				if !strings.Contains(prose, "1 zones") || len(v.Threat) != 1 || v.ThreatMax != 3 {
					t.Fatalf("%s / %+v", prose, v)
				}
			}
		})
	}
}

func TestLiveFieldUsesResolvedTickForThreat(t *testing.T) {
	var threatTick string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		switch {
		case strings.Contains(query, "min(tick)"):
			fmt.Fprintln(w, `{"lo":20,"hi":200,"n":10}`)
		case strings.Contains(query, "terrain_cols"):
			fmt.Fprintln(w, `{"terrain_cols":1,"terrain_rows":1,"terrain_cell_w":64,"terrain_cell_h":64,"terrain":"."}`)
		case strings.Contains(query, "count() AS n"):
			fmt.Fprintln(w, `{"n":1}`)
		case strings.Contains(query, "FROM stream_threat"):
			threatTick = r.URL.Query().Get("param_tick")
			fmt.Fprintln(w, `{"col":0,"row":0,"value":2}`)
		default:
			fmt.Fprintln(w, `{"tick":200,"unit_id":1,"side":"ours"}`)
		}
	}))
	defer srv.Close()
	v := loadField(t.Context(), ch.New(srv.URL, "currie", "", ""), "live-resolved-tick", 0, false)
	if v.Tick != 200 || threatTick != "200" || len(v.Threat) != 1 {
		t.Fatalf("tick=%d query tick=%q zones=%d note=%s", v.Tick, threatTick, len(v.Threat), v.Note)
	}
}
