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
//
// The bool is "this is a real measurement". A producer that could not measure
// must return false, NOT a zero ToolBudget: a zero is a perfectly plausible
// budget on the wire — nothing about `{"tools":0,"bytes":0}` says "unknown" —
// and the pane that received one drew "the agent currently gets 0 tools,
// about 0.0 kB" over a tool set of nineteen. An absent budget is the only
// honest way to say the number is not known, and it is the case the pane
// already knows how to draw: it draws nothing.
type ToolBudgetFunc func(memoryTools, skillTools bool) (ToolBudget, bool)

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
// returns nil when nothing wired a producer — or when the producer that is
// wired could not measure.
//
// The two switches arrive as arguments rather than being read back off the
// store, and that is what keeps the answer self-consistent: the caller,
// redactedSettingsView, is describing one SNAPSHOT of the config, and a
// budget that went and read s.cfg again would be free to measure a different
// one — a concurrent Set landing in between, or Set's own call on the `next`
// it has not committed yet, which is how a pane could be told 16 tools beside
// two switches that both say on. s.mu is taken here only for the producer
// field, which any WithToolBudget can replace.
func (s *Store) toolBudgetView(memoryTools, skillTools bool) *ToolBudget {
	s.mu.RLock()
	f := s.toolBudget
	s.mu.RUnlock()
	if f == nil {
		return nil
	}
	b, ok := f(memoryTools, skillTools)
	if !ok {
		return nil
	}
	return &b
}
