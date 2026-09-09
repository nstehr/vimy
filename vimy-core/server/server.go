package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/nstehr/vimy/vimy-core/agent"
	"github.com/nstehr/vimy/vimy-core/rules"
	"github.com/nstehr/vimy/vimy-core/server/views"
	"github.com/nstehr/vimy/vimy-core/store"
)

// Server serves the doctrine dashboard over HTTP.
type Server struct {
	strategist *agent.Strategist
	store      *store.Store
	mux        *http.ServeMux
}

// New creates a dashboard server backed by the given strategist and store.
func New(strategist *agent.Strategist, store *store.Store) *Server {
	s := &Server{strategist: strategist, store: store}
	s.mux = http.NewServeMux()
	s.routes()
	return s
}

// Start listens on addr and serves HTTP until error.
func (s *Server) Start(addr string) error {
	return http.ListenAndServe(addr, s.mux)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /{$}", s.handleDashboard)
	s.mux.HandleFunc("GET /api/directive", s.handleGetDirective)
	s.mux.HandleFunc("PUT /api/directive", s.handleSetDirective)
	s.mux.HandleFunc("GET /api/doctrine/current", s.handleCurrentDoctrine)
	s.mux.HandleFunc("GET /api/doctrine/history", s.handleDoctrineHistory)
	s.mux.HandleFunc("GET /api/rules", s.handleRules)
	// A route rather than inline: templ won't interpolate inside a <script>, and
	// this way it caches.
	s.mux.HandleFunc("GET /static/vy-prism.js", s.handleVyGrammar)
	s.mux.HandleFunc("GET /api/battlefield", s.handleBattlefield)
	s.mux.HandleFunc("GET /api/record", s.handleRecord)
	s.mux.HandleFunc("GET /api/record/panel", s.handleRecordPanel)
	s.mux.HandleFunc("GET /api/memory/panel", s.handleMemoryPanel)
	s.mux.HandleFunc("GET /api/memory/librarian", s.handleLibrarianPanel)
	s.mux.HandleFunc("GET /api/rules/firings", s.handleRuleFiringPanel)
}

func (s *Server) wins() int {
	if s.store == nil {
		return 0
	}
	return s.store.Wins()
}

func (s *Server) losses() int {
	if s.store == nil {
		return 0
	}
	return s.store.Losses()
}

func (s *Server) currentDirective() string {
	if s.strategist == nil {
		return ""
	}
	return s.strategist.GetDirective()
}

func (s *Server) handleGetDirective(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"directive": s.currentDirective(),
	})
}

func (s *Server) handleSetDirective(w http.ResponseWriter, r *http.Request) {
	if s.strategist == nil {
		http.Error(w, "no strategist configured", http.StatusBadRequest)
		return
	}

	var body struct {
		Directive string `json:"directive"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.strategist.SetDirective(body.Directive)
	slog.Info("directive updated via dashboard", "directive", body.Directive)

	views.DirectiveForm(body.Directive).Render(r.Context(), w)
}

func (s *Server) handleCurrentDoctrine(w http.ResponseWriter, r *http.Request) {
	if s.strategist == nil {
		views.DoctrineCard(nil).Render(r.Context(), w)
		return
	}
	rec := s.strategist.GetCurrentDoctrine()
	views.DoctrineCard(rec).Render(r.Context(), w)
}

func (s *Server) handleRules(w http.ResponseWriter, r *http.Request) {
	var summaries []rules.RuleSummary
	if s.strategist != nil {
		summaries = s.strategist.GetRules()
	}
	views.RulesPanel(summaries).Render(r.Context(), w)
}

func (s *Server) handleBattlefield(w http.ResponseWriter, r *http.Request) {
	var status *agent.BattlefieldStatus
	if s.strategist != nil {
		status = s.strategist.GetBattlefieldStatus()
	}
	views.BattlefieldPanel(status).Render(r.Context(), w)
}

func (s *Server) handleRecordPanel(w http.ResponseWriter, r *http.Request) {
	views.RecordPanel(s.wins(), s.losses()).Render(r.Context(), w)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	lessons, games := s.memorySnapshot(r.Context())
	var librarian *agent.LibrarianSnapshot
	var ruleTrace agent.RuleTraceSnapshot
	if s.strategist != nil {
		librarian = s.strategist.GetLibrarianSnapshot()
		ruleTrace = s.strategist.GetRuleTraceSnapshot()
	}
	views.Dashboard(s.currentDirective(), s.wins(), s.losses(), lessons, games, librarian, ruleTrace).
		Render(r.Context(), w)
}

func (s *Server) handleMemoryPanel(w http.ResponseWriter, r *http.Request) {
	lessons, games := s.memorySnapshot(r.Context())
	views.MemoryPanel(lessons, games).Render(r.Context(), w)
}

func (s *Server) handleLibrarianPanel(w http.ResponseWriter, r *http.Request) {
	var snap *agent.LibrarianSnapshot
	if s.strategist != nil {
		snap = s.strategist.GetLibrarianSnapshot()
	}
	views.LibrarianPanel(snap).Render(r.Context(), w)
}

func (s *Server) handleRuleFiringPanel(w http.ResponseWriter, r *http.Request) {
	var snap agent.RuleTraceSnapshot
	if s.strategist != nil {
		snap = s.strategist.GetRuleTraceSnapshot()
	}
	views.RuleFiringPanel(snap).Render(r.Context(), w)
}

func (s *Server) memorySnapshot(ctx context.Context) ([]store.GlobalLesson, []store.GameSummary) {
	if s.store == nil {
		return nil, nil
	}
	lessons, err := s.store.TopLessons(ctx, 8)
	if err != nil {
		slog.Warn("TopLessons failed", "error", err)
		lessons = nil
	}
	games, err := s.store.RecentGames(ctx, 10)
	if err != nil {
		slog.Warn("RecentGames failed", "error", err)
		games = nil
	}
	return lessons, games
}

func (s *Server) handleRecord(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.store == nil {
		json.NewEncoder(w).Encode(map[string]any{"wins": 0, "losses": 0, "games": []store.GameRecord{}})
		return
	}

	json.NewEncoder(w).Encode(map[string]any{
		"wins":   s.store.Wins(),
		"losses": s.store.Losses(),
		"games":  s.store.Games(),
	})
}

type historyPoint struct {
	Tick                  int      `json:"tick"`
	EconomyPriority       float64  `json:"economy_priority"`
	TechPriority          float64  `json:"tech_priority"`
	InfantryWeight        float64  `json:"infantry_weight"`
	VehicleWeight         float64  `json:"vehicle_weight"`
	AirWeight             float64  `json:"air_weight"`
	NavalWeight           float64  `json:"naval_weight"`
	Aggression            float64  `json:"aggression"`
	GroundDefensePriority float64  `json:"ground_defense_priority"`
	AirDefensePriority    float64  `json:"air_defense_priority"`
	ScoutPriority         float64  `json:"scout_priority"`
	SuperweaponPriority   float64  `json:"superweapon_priority"`
	Name                  string   `json:"name"`
	PreferredInfantry     []string `json:"preferred_infantry,omitempty"`
	PreferredVehicle      []string `json:"preferred_vehicle,omitempty"`
	PreferredAircraft     []string `json:"preferred_aircraft,omitempty"`
	PreferredNaval        []string `json:"preferred_naval,omitempty"`
}

func (s *Server) handleDoctrineHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.strategist == nil {
		json.NewEncoder(w).Encode([]historyPoint{})
		return
	}

	history := s.strategist.GetHistory()
	points := make([]historyPoint, len(history))
	for i, rec := range history {
		d := rec.Doctrine
		points[i] = historyPoint{
			Tick:                  rec.Tick,
			EconomyPriority:       d.EconomyPriority,
			TechPriority:          d.TechPriority,
			InfantryWeight:        d.InfantryWeight,
			VehicleWeight:         d.VehicleWeight,
			AirWeight:             d.AirWeight,
			NavalWeight:           d.NavalWeight,
			Aggression:            d.Aggression,
			GroundDefensePriority: d.GroundDefensePriority,
			AirDefensePriority:    d.AirDefensePriority,
			ScoutPriority:         d.ScoutPriority,
			SuperweaponPriority:   d.SuperweaponPriority,
			Name:                  d.Name,
			PreferredInfantry:     d.PreferredInfantry,
			PreferredVehicle:      d.PreferredVehicle,
			PreferredAircraft:     d.PreferredAircraft,
			PreferredNaval:        d.PreferredNaval,
		}
	}
	json.NewEncoder(w).Encode(points)
}

// handleVyGrammar serves the Prism definition for `.vy`, generated from the
// keyword list rather than hand-written.
func (s *Server) handleVyGrammar(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "no-cache")
	if _, err := io.WriteString(w, views.VyPrismGrammar()); err != nil {
		slog.Debug("serving the vy grammar", "error", err)
	}
}
