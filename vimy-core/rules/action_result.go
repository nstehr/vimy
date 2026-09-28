package rules

import "fmt"

// CommandSender is the only transport capability an action needs.
type CommandSender interface {
	Send(messageType string, payload any) error
}

// ActionResult distinguishes a matching condition from an effective action.
type ActionResult struct {
	CommandsSent int
	StateChanged bool
	NoOpReason   string
}

func (r ActionResult) Acted() bool { return r.CommandsSent > 0 || r.StateChanged }

type actionSender struct {
	destination CommandSender
	result      *ActionResult
}

func (s actionSender) Send(kind string, payload any) error {
	if s.destination == nil {
		return fmt.Errorf("action has no command sender")
	}
	if err := s.destination.Send(kind, payload); err != nil {
		return err
	}
	s.result.CommandsSent++
	return nil
}

// RunAction scopes effects to this invocation, including orders sent before an
// error. It does not infer effects from unrelated connection traffic or memory.
func RunAction(action ActionFunc, env RuleEnv, sender CommandSender) (ActionResult, error) {
	result := ActionResult{}
	env.result = &result
	err := action(env, actionSender{destination: sender, result: &result})
	if !result.Acted() && err == nil && result.NoOpReason == "" {
		result.NoOpReason = "no command or state change"
	}
	return result, err
}

func markEffect(env RuleEnv) {
	if env.result != nil {
		env.result.StateChanged = true
	}
}
