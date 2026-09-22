package agent

import (
	"strings"
	"testing"

	bamlpkg "github.com/boundaryml/baml/engine/language_client_go/pkg"
	baml "github.com/nstehr/vimy/vimy-core/baml_client"
	"github.com/nstehr/vimy/vimy-core/baml_client/types"
)

// promptText renders a built request's body to the text the model would see.
func promptText(t *testing.T, req bamlpkg.HTTPRequest) string {
	t.Helper()
	body, err := req.Body()
	if err != nil {
		t.Fatalf("body: %v", err)
	}
	text, err := body.Text()
	if err != nil {
		t.Fatalf("body text: %v", err)
	}
	return text
}

// burned_axes was computed every evaluation, assigned onto the situation, used
// by the rule engine to gate eight production rules — and never rendered into
// the prompt. The BURNED AXIS rule told the model to consult a list it had
// never been shown, while axis-burned silently switched production off.
//
// Rendering the prompt is the only way to catch that class of bug: the field
// existed, the assignment existed, and nothing errored.
func TestBurnedAxesReachThePrompt(t *testing.T) {
	sit := types.GameSituation{Tick: 5000, Phase: "Mid Game", Burned_axes: []string{"vehicle", "air"}}
	req, err := baml.Request.GenerateDoctrine("hold the line", sit, "germany", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	body := promptText(t, req)
	// The colon distinguishes the rendered LINE from the static rule text,
	// which also contains the words "BURNED AXES".
	if !strings.Contains(body, "BURNED AXES: ") {
		t.Fatal("the BURNED AXES line never rendered; the model cannot see the list it is told to obey")
	}
	if !strings.Contains(body, "BURNED AXES: vehicle, air") {
		t.Error("the axes did not render as a list")
	}
	for _, axis := range []string{"vehicle", "air"} {
		if !strings.Contains(body, axis) {
			t.Errorf("axis %q missing from the rendered prompt", axis)
		}
	}

	// And it must stay absent when nothing is burned, or every doctrine is told
	// about a constraint that does not exist.
	clean, err := baml.Request.GenerateDoctrine("hold the line",
		types.GameSituation{Tick: 5000, Phase: "Mid Game"}, "germany", nil)
	if err != nil {
		t.Fatalf("build clean request: %v", err)
	}
	if strings.Contains(promptText(t, clean), "BURNED AXES: ") {
		t.Error("BURNED AXES rendered with nothing burned")
	}
}

// Every situation field a prompt rule reasons about must actually reach the
// prompt.
//
// burned_axes was computed, assigned, used to gate eight production rules, and
// never rendered. Fixing that one did not fix its siblings: being_rushed and
// harvester_harassed were the same, so RUSH RESPONSE and HARVESTER HARASSMENT
// could never fire. Vimy was rushed in games 160 and 161 — 139 harvesters lost
// in one of them, refineries walked down to zero — and infantry_weight stayed
// at 0.35 against a rule demanding 0.5, because the model was told to check a
// flag it had never been shown. ground_squad_ready_ratio was invisible too,
// which made the comparison the prompt invites meaningless.
//
// The field existing, the assignment existing, and nothing erroring is exactly
// what this class of bug looks like. Only rendering catches it.
func TestSituationFlagsReachThePrompt(t *testing.T) {
	sit := types.GameSituation{
		Tick: 4000, Phase: "Early Game",
		Being_rushed:                 true,
		Harvester_harassed:           true,
		Ground_squad_ready_ratio:     0.4,
		Cash_burn_rate:               -250,
		Time_to_reach_enemy_estimate: 3500,
	}
	body := promptText(t, mustRequest(t, sit))
	for _, want := range []string{"BEING RUSHED: true", "HARVESTER HARASSMENT: true", "Ground squad readiness", "Cash burn rate", "3500"} {
		if !strings.Contains(body, want) {
			t.Errorf("%q never rendered; the rule that reads it cannot fire", want)
		}
	}

	// The two alarms are conditional and must stay silent when false, or every
	// doctrine is told it is under attack.
	calm := promptText(t, mustRequest(t, types.GameSituation{Tick: 4000, Phase: "Early Game"}))
	for _, unwanted := range []string{"BEING RUSHED: true", "HARVESTER HARASSMENT: true"} {
		if strings.Contains(calm, unwanted) {
			t.Errorf("%q rendered when the flag is false", unwanted)
		}
	}
}

func mustRequest(t *testing.T, sit types.GameSituation) bamlpkg.HTTPRequest {
	t.Helper()
	req, err := baml.Request.GenerateDoctrine("hold the line", sit, "germany", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	return req
}
