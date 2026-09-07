package settings

// ToolBudget is the measured cost of the tool set an agent is offered (#99).
//
// It exists because a bare switch is not a budget. The writer turning the
// memory or skills tools off is making a trade, and they can only make it if
// the pane says what the trade is worth: how many tools the agent gets right
// now, how many bytes of tool schema travel in every single request, and what
// each group costs of that.
//
// Every number here is MEASURED, never written down: the producer builds the
// tool set the engine would actually register and adds up the JSON it would
// send. That is the whole point of the indirection — a table of byte counts
// maintained by hand goes stale the first time a tool description is edited,
// which is exactly how apps/site's tool counts drifted. See
// mcphost.MeasureToolBudget.
//
// This package holds the type but cannot produce it: the tool layer is
// //go:build !mobile and imports this package, so the arrow only points one
// way. engineapp hands Store a producer with WithToolBudget; a build that
// wires none leaves Config.ToolBudget nil.
type ToolBudget struct {
	// Tools and Bytes describe what the built-in agent is offered with the
	// writer's switches as they stand — the agent rather than the external
	// server because the agent always registers the full set, so this is the
	// number the budget is actually about, and it is the ceiling for any
	// external client too.
	Tools int `json:"tools"`
	Bytes int `json:"bytes"`
	// Memory and Skills are what each group costs when it is on, whether or
	// not it currently is. A writer deciding whether to switch one off needs
	// the price of the thing they are giving up, which a report that only
	// described the current state could never tell them.
	Memory ToolBudgetGroup `json:"memory"`
	Skills ToolBudgetGroup `json:"skills"`
}

// ToolBudgetGroup is one switchable group's own share of the budget.
type ToolBudgetGroup struct {
	Tools int `json:"tools"`
	Bytes int `json:"bytes"`
}

// ToolBudgetFunc measures the budget for one combination of the two switches.
// The arguments are passed rather than read off the store so the producer
// stays a pure function of the tool set — it can be called for a hypothetical
// combination, and it cannot deadlock against the lock the store holds while
// building the settings.get view.
type ToolBudgetFunc func(memoryTools, skillTools bool) ToolBudget

// WithToolBudget installs the measurement settings.get reports, and returns s
// so it can be chained onto a constructor. A nil f (or no call at all) leaves
// Config.ToolBudget nil, which is what a mobile build and most tests get.
func (s *Store) WithToolBudget(f ToolBudgetFunc) *Store {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.toolBudget = f
	return s
}

// toolBudgetView measures the budget for the switches as they stand, or
// returns nil when nothing wired a producer.
//
// It reads the two switches through the accessors rather than under its own
// lock: redactedSettingsView takes s.mu itself, and taking it again here is
// the one shape of this that deadlocks.
func (s *Store) toolBudgetView(memoryTools, skillTools bool) *ToolBudget {
	s.mu.RLock()
	f := s.toolBudget
	s.mu.RUnlock()
	if f == nil {
		return nil
	}
	b := f(memoryTools, skillTools)
	return &b
}
