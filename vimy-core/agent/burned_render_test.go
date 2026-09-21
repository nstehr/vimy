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
