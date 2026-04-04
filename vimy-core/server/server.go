package server

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/a-h/templ"
	"github.com/nstehr/vimy/vimy-core/agent"
	"github.com/nstehr/vimy/vimy-core/rules"
	"github.com/nstehr/vimy/vimy-core/server/views"
)

// Server serves the doctrine dashboard over HTTP.
type Server struct {
	strategist *agent.Strategist
	mux        *http.ServeMux
}

// New creates a dashboard server backed by the given strategist.
func New(strategist *agent.Strategist) *Server {
	s := &Server{strategist: strategist}
	s.mux = http.NewServeMux()
	s.routes()
	return s
}

// Start listens on addr and serves HTTP until error.
func (s *Server) Start(addr string) error {
	return http.ListenAndServe(addr, s.mux)
}

func (s *Server) routes() {
	s.mux.Handle("GET /", templ.Handler(views.Dashboard(s.currentDirective())))
	s.mux.HandleFunc("GET /api/directive", s.handleGetDirective)
	s.mux.HandleFunc("PUT /api/directive", s.handleSetDirective)
	s.mux.HandleFunc("GET /api/doctrine/current", s.handleCurrentDoctrine)
	s.mux.HandleFunc("GET /api/doctrine/history", s.handleDoctrineHistory)
	s.mux.HandleFunc("GET /api/rules", s.handleRules)
	s.mux.HandleFunc("GET /api/battlefield", s.handleBattlefield)
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

	// Return the updated directive form as HTML fragment for htmx swap.
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

type historyPoint struct {
	Tick                      int      `json:"tick"`
	EconomyPriority           float64  `json:"economy_priority"`
	TechPriority              float64  `json:"tech_priority"`
	InfantryWeight            float64  `json:"infantry_weight"`
	VehicleWeight             float64  `json:"vehicle_weight"`
	AirWeight                 float64  `json:"air_weight"`
	NavalWeight               float64  `json:"naval_weight"`
	Aggression                float64  `json:"aggression"`
	GroundDefensePriority     float64  `json:"ground_defense_priority"`
	AirDefensePriority        float64  `json:"air_defense_priority"`
	ScoutPriority             float64  `json:"scout_priority"`
	SuperweaponPriority       float64  `json:"superweapon_priority"`
	Name                      string   `json:"name"`
	PreferredInfantry         []string `json:"preferred_infantry,omitempty"`
	PreferredVehicle          []string `json:"preferred_vehicle,omitempty"`
	PreferredAircraft         []string `json:"preferred_aircraft,omitempty"`
	PreferredNaval            []string `json:"preferred_naval,omitempty"`
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
			EconomyPriority:      d.EconomyPriority,
			TechPriority:         d.TechPriority,
			InfantryWeight:       d.InfantryWeight,
			VehicleWeight:        d.VehicleWeight,
			AirWeight:            d.AirWeight,
			NavalWeight:          d.NavalWeight,
			Aggression:           d.Aggression,
			GroundDefensePriority: d.GroundDefensePriority,
			AirDefensePriority:   d.AirDefensePriority,
			ScoutPriority:        d.ScoutPriority,
			SuperweaponPriority:  d.SuperweaponPriority,
			Name:                 d.Name,
			PreferredInfantry:    d.PreferredInfantry,
			PreferredVehicle:     d.PreferredVehicle,
			PreferredAircraft:    d.PreferredAircraft,
			PreferredNaval:       d.PreferredNaval,
		}
	}
	json.NewEncoder(w).Encode(points)
}
