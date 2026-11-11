package ledger

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// briefTimeRunes bounds the story time section of the brief.
const briefTimeRunes = 600

// daysPerYear turns an age in years into days. The ledger keeps no calendar:
// a month is 30 days and a year 365.
const daysPerYear = 365

// tolerance is how far two counts of days may differ and still be one. People
// speak of elapsed time in round figures: "석 달" is anything from 80 to 100
// days. The larger of the two counts sets it.
func tolerance(days float64) float64 {
	switch d := math.Abs(days); {
	case d < 14:
		return 1
	case d < 60:
		return 7
	case d < 730:
		return 30
	}
	return 183
}

// flashbackAt reports whether the scene at story position i is a flashback.
func (t tree) flashbackAt(i int) bool {
	return i >= 0 && i < len(t.leaves) && t.leaves[i].Flashback
}

// ordered reports whether a fact takes part in comparisons in story order seen
// from scene cur. A flashback is set apart: its facts are compared only with
// each other, and nothing from one is compared with the scenes around it.
func (t tree) ordered(e Entry, cur int) bool {
	at := t.plantedAt(e)
	if t.flashbackAt(cur) {
		return at == cur
	}
	return at < 0 || at == cur || !t.flashbackAt(at)
}

// measured reports whether a fact is a count the comparison reads: an elapsed
// time that names its event, or an age in years. What a character said or a
// rumour carried may be wrong without the story being wrong.
func measured(e Entry) bool {
	if e.ValueNum == nil || e.Strength == StrengthSoft || e.Status != StatusOpen || e.SupersededBy != "" {
		return false
	}
	return (e.Attribute == AttributeElapsed && e.About != "") || e.Attribute == AttributeAge
}

// event is an elapsed fact's event, and the day it fell on when known.
func event(events map[string]Entry, id string) (Entry, bool) {
	ev, ok := events[id]
	if !ok || ev.Kind != KindTimeline || ev.Attribute == AttributeElapsed || ev.Status != StatusOpen {
		return Entry{}, false
	}
	return ev, true
}

// clockAt is a scene's story date from the facts recorded in it: its last 작중
// 날짜 first, then the last elapsed time from an event whose date is known, as
// that date plus the time since. skip is an event not to count from, the one
// whose elapsed times are being compared. A date carried over from an earlier
// scene would say no time passed, so only the scene's own facts count.
func clockAt(facts []Entry, events map[string]Entry, skip string) (float64, bool) {
	var date, elapsed *Entry
	for i := range facts {
		f := facts[i]
		if f.ValueNum == nil || f.Strength == StrengthSoft || f.Status != StatusOpen || f.SupersededBy != "" {
			continue
		}
		switch {
		case f.Attribute == AttributeDate:
			if date == nil || f.CreatedAt >= date.CreatedAt {
				date = &facts[i]
			}
		case f.Attribute == AttributeElapsed && f.About != "" && f.About != skip:
			ev, ok := event(events, f.About)
			if ok && ev.ValueNum != nil && (elapsed == nil || f.CreatedAt >= elapsed.CreatedAt) {
				elapsed = &facts[i]
			}
		}
	}
	switch {
	case date != nil:
		return *date.ValueNum, true
	case elapsed != nil:
		return *events[elapsed.About].ValueNum + *elapsed.ValueNum, true
	}
	return 0, false
}

// byScene groups facts by the story position they were planted at.
func (t tree) byScene(entries []Entry) map[int][]Entry {
	out := map[int][]Entry{}
	for _, e := range entries {
		if i, ok := t.index[e.PlantedNodeID]; ok {
			out[i] = append(out[i], e)
		}
	}
	return out
}

// timeView is a work's outline and open ledger, read once for comparing
// elapsed times.
type timeView struct {
	t       tree
	entries []Entry
	events  map[string]Entry
	scenes  map[int][]Entry
}

func (r *Repo) loadTime(ctx context.Context, projectID string) (timeView, error) {
	all, err := r.nodes.ListByProject(ctx, projectID)
	if err != nil {
		return timeView{}, err
	}
	entries, err := r.List(ctx, Filter{ProjectID: projectID, Status: StatusOpen})
	if err != nil {
		return timeView{}, err
	}
	v := timeView{t: newTree(all), entries: entries, events: make(map[string]Entry, len(entries))}
	for _, e := range entries {
		v.events[e.ID] = e
	}
	v.scenes = v.t.byScene(entries)
	return v, nil
}

// elapsedConflicts compares an elapsed time, or an age in years, with the last
// one before it in story order for the same event or person. Time since an
// event does not shrink. When both scenes' story dates are known, it grows by
// what passed between them. When they are not, it is no conflict: the jump is
// returned for the settlement pass to judge against the prose.
func (r *Repo) elapsedConflicts(ctx context.Context, e Entry) ([]Conflict, *Previous, error) {
	if !measured(e) {
		return nil, nil, nil
	}
	v, err := r.loadTime(ctx, e.ProjectID)
	if err != nil {
		return nil, nil, err
	}
	conflicts, previous := v.compare(e)
	return conflicts, previous, nil
}

func (v timeView) compare(e Entry) ([]Conflict, *Previous) {
	t := v.t
	cur, onStory := t.index[e.PlantedNodeID]
	if !measured(e) || !onStory {
		return nil, nil
	}
	age := e.Attribute == AttributeAge
	same := func(o Entry) bool {
		if age {
			return o.Attribute == AttributeAge && o.Subject == e.Subject
		}
		return o.Attribute == AttributeElapsed && o.About == e.About
	}
	var prior *Entry
	for i := range v.entries {
		o := v.entries[i]
		if o.ID == e.ID || !same(o) {
			continue
		}
		if o.SupersededBy == e.ID {
			return nil, nil
		}
		at := t.plantedAt(o)
		if !measured(o) || at < 0 || at > cur || (at == cur && o.CreatedAt > e.CreatedAt) || !t.ordered(o, cur) {
			continue
		}
		if prior == nil || t.storyOrder(o, *prior) >= 0 {
			prior = &v.entries[i]
		}
	}
	if prior == nil {
		return nil, nil
	}
	was, now := *prior.ValueNum, *e.ValueNum
	previous := &Previous{EntryID: prior.ID, ValueNum: was, Delta: now - was}
	at := t.plantedAt(*prior)
	there, here := t.leaves[at].Label, t.leaves[cur].Label
	conflict := func(reason string) ([]Conflict, *Previous) {
		return []Conflict{{EntryID: prior.ID, Body: prior.Body, Reason: reason}}, previous
	}

	unit, slack := 1.0, tolerance(math.Max(math.Abs(was), math.Abs(now)))
	if age {
		unit, slack = daysPerYear, 1
	}
	if now < was-slack {
		if age {
			return conflict(fmt.Sprintf("%s는 %s에서 이미 %s살이었는데 %s에서는 %s살입니다", e.Subject, there, number(was), here, number(now)))
		}
		return conflict(fmt.Sprintf("%s에서 이미 %s일이 지났는데 %s에서는 %s일입니다", there, number(was), here, number(now)))
	}
	if at == cur {
		return nil, previous
	}
	change := (now - was) * unit
	skip := e.About
	if age {
		skip = ""
	}
	from, known := clockAt(v.scenes[at], v.events, skip)
	to, knownToo := clockAt(v.scenes[cur], v.events, skip)
	if !known || !knownToo {
		if change > 0 {
			previous.JumpDays, previous.ScenesBetween = change, cur-at
		}
		return nil, previous
	}
	flow := to - from
	limit := tolerance(math.Max(math.Abs(change), math.Abs(flow)))
	if age {
		limit = daysPerYear
	}
	if math.Abs(change-flow) <= limit {
		return nil, previous
	}
	grew := "늘었습니다"
	if change < 0 {
		grew = "줄었습니다"
	}
	if age {
		return conflict(fmt.Sprintf("%s와 %s 사이에 %s일이 흘렀는데, %s의 나이는 %s살 %s",
			there, here, number(flow), e.Subject, number(math.Abs(now-was)), grew))
	}
	return conflict(fmt.Sprintf("%s와 %s 사이에 다른 기준으로는 %s일이 흘렀는데, %s 경과는 %s일 %s",
		there, here, number(flow), e.Subject, number(math.Abs(change)), grew))
}

// StoryTime is a work's elapsed times laid out for the writer: the story date
// each scene is known at, and for each event and each person with an age in
// years, every count in story order with what the comparison says of it.
type StoryTime struct {
	// Clocks maps a scene to its story date in days, where its own facts give one.
	Clocks map[string]float64 `json:"clocks"`
	Events []TimeRow          `json:"events"`
	Ages   []TimeRow          `json:"ages"`
}

// TimeRow is one event, or one person's age. Event is nil for an age.
type TimeRow struct {
	Event   *Entry     `json:"event,omitempty"`
	Subject string     `json:"subject"`
	Facts   []TimeFact `json:"facts"`
}

// TimeFact is one count and how it compares with the one before it.
type TimeFact struct {
	Entry     Entry      `json:"entry"`
	Conflicts []Conflict `json:"conflicts"`
	Previous  *Previous  `json:"previous,omitempty"`
}

// StoryTime reads a work's elapsed times. A count with no number, or one a
// character said, is listed and not compared.
func (r *Repo) StoryTime(ctx context.Context, projectID string) (StoryTime, error) {
	v, err := r.loadTime(ctx, projectID)
	if err != nil {
		return StoryTime{}, err
	}
	out := StoryTime{Clocks: map[string]float64{}, Events: []TimeRow{}, Ages: []TimeRow{}}
	for i, leaf := range v.t.leaves {
		if c, ok := clockAt(v.scenes[i], v.events, ""); ok {
			out.Clocks[leaf.ID] = c
		}
	}
	rows := map[string]*TimeRow{}
	var order []string
	for _, e := range v.entries {
		if e.SupersededBy != "" {
			continue
		}
		var key string
		var row TimeRow
		switch {
		case e.Attribute == AttributeElapsed && e.About != "":
			ev, ok := event(v.events, e.About)
			if !ok {
				continue
			}
			key, row = "e:"+ev.ID, TimeRow{Event: &ev, Subject: ev.Body}
			if ev.Subject != "" {
				row.Subject = ev.Subject
			}
		case e.Attribute == AttributeAge && e.ValueNum != nil:
			key, row = "a:"+e.Subject, TimeRow{Subject: e.Subject}
		default:
			continue
		}
		if rows[key] == nil {
			rows[key] = &row
			order = append(order, key)
		}
		conflicts, previous := v.compare(e)
		if conflicts == nil {
			conflicts = []Conflict{}
		}
		rows[key].Facts = append(rows[key].Facts, TimeFact{Entry: e, Conflicts: conflicts, Previous: previous})
	}
	for _, key := range order {
		row := *rows[key]
		slices.SortStableFunc(row.Facts, func(a, b TimeFact) int { return v.t.storyOrder(a.Entry, b.Entry) })
		if row.Event != nil {
			out.Events = append(out.Events, row)
		} else {
			out.Ages = append(out.Ages, row)
		}
	}
	return out, nil
}

// number writes a count of days or years as people read one: rounded, with
// thousands marked.
func number(v float64) string {
	s := strconv.FormatInt(int64(math.Round(math.Abs(v))), 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	if v < 0 && s != "0" {
		return "-" + s
	}
	return s
}

// timeLines renders the story time a scene is written at: the last story date
// known up to it, and for each event, the last elapsed time recorded before
// it. Events the scene talks about come first, then those measured most
// recently. Nothing planted after the scene is read. In a flashback the lines
// are the story's present, which the flashback steps back from.
func (t tree) timeLines(entries []Entry, nodeID string, cur int) ([]string, int) {
	var visible []Entry
	for _, e := range entries {
		if e.Status == StatusOpen && e.SupersededBy == "" && !t.after(e.PlantedNodeID, cur) {
			visible = append(visible, e)
		}
	}
	events := map[string]Entry{}
	for _, e := range visible {
		events[e.ID] = e
	}
	scenes := t.byScene(visible)
	present := func(at int) bool { return at == cur || !t.flashbackAt(at) }

	var lines []string
	clock, clockKnown := 0.0, false
	for i := min(cur, len(t.leaves)-1); i >= 0; i-- {
		if !present(i) {
			continue
		}
		if c, ok := clockAt(scenes[i], events, ""); ok {
			if i == cur {
				clock, clockKnown = c, true
				lines = append(lines, fmt.Sprintf("이 회차의 작중 날짜: %s일", number(c)))
			} else {
				lines = append(lines, fmt.Sprintf("마지막으로 아는 작중 날짜: %s일 (%s)", number(c), t.leaves[i].Label))
			}
			break
		}
	}

	latest := map[string]Entry{}
	for _, e := range visible {
		if e.Attribute != AttributeElapsed || !measured(e) || !present(t.plantedAt(e)) || t.plantedAt(e) < 0 {
			continue
		}
		if _, ok := event(events, e.About); !ok {
			continue
		}
		if prior, ok := latest[e.About]; !ok || t.storyOrder(e, prior) >= 0 {
			latest[e.About] = e
		}
	}
	if len(latest) == 0 {
		return lines, 0
	}
	text := t.sceneText(nodeID)
	if n, ok := t.byID[nodeID]; ok && n.ContentDoc != nil {
		text += "\n" + *n.ContentDoc
	}
	type row struct {
		line string
		hits int
		fact Entry
	}
	rows := make([]row, 0, len(latest))
	for id, fact := range latest {
		ev := events[id]
		name := ev.Body
		if ev.Subject != "" {
			name = ev.Subject
		}
		name = clip(name, briefSecretNameRunes)
		var b strings.Builder
		b.WriteString(name)
		if n, ok := t.byID[ev.PlantedNodeID]; ok {
			b.WriteString("(" + n.Label + ")")
		}
		fmt.Fprintf(&b, ": %s에서 %s(약 %s일)", t.leaves[t.plantedAt(fact)].Label, clip(fact.Body, briefStateValueRunes/2), number(*fact.ValueNum))
		if clockKnown && ev.ValueNum != nil {
			fmt.Fprintf(&b, ", 이 회차에서 약 %s일", number(clock-*ev.ValueNum))
		}
		rows = append(rows, row{b.String(), subjectHits(name, text), fact})
	}
	slices.SortFunc(rows, func(a, b row) int {
		return cmp.Or(b.hits-a.hits, t.storyOrder(b.fact, a.fact), cmp.Compare(a.line, b.line))
	})
	room, omitted := briefTimeRunes, 0
	for _, l := range lines {
		room -= utf8.RuneCountInString(l)
	}
	for _, r := range rows {
		cost := utf8.RuneCountInString(r.line)
		if cost > room {
			omitted++
			continue
		}
		room -= cost
		lines = append(lines, r.line)
	}
	return lines, omitted
}
