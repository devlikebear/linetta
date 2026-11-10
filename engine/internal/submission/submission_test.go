package submission

import (
	"reflect"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/charcount"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
)

func mk(id, parent string, ordinal int, kind, label string, chars int) node.Node {
	n := node.Node{ID: id, ProjectID: "p", Ordinal: ordinal, Kind: kind, Label: label, WordCount: chars}
	if parent != "" {
		n.ParentID = &parent
	}
	return n
}

// A web novel of four episodes, one of them made ahead and empty:
// 1화 5,200, 2화 3,925 (two scenes), 3화 4,380, 4화 nothing yet.
func webNovel() []node.Node {
	return []node.Node{
		mk("arc", "", 0, node.KindContainer, "1권", 0),
		mk("ep1", "arc", 0, node.KindLeaf, "1화", 5200),
		mk("ep2", "arc", 1, node.KindContainer, "2화", 0),
		mk("ep3", "arc", 2, node.KindLeaf, "3화", 4380),
		mk("ep4", "arc", 3, node.KindContainer, "4화", 0),
		mk("s1", "ep2", 0, node.KindLeaf, "씬 1", 2100),
		mk("s2", "ep2", 1, node.KindLeaf, "씬 2", 1825),
	}
}

func contest() project.Project {
	return project.Project{
		OutlinePreset: project.OutlinePresetWebNovel, EpisodeCharMin: 5000, EpisodeCharTarget: 5300,
		SubmissionMinEpisodes: 7, SubmissionTotalMin: 30000, SubmissionNeedsSynopsis: true,
	}
}

func TestCheck_webNovelShortOfAContest(t *testing.T) {
	r := Check(webNovel(), contest())
	if !r.HasRule || !r.Structured || !r.EpisodeRule {
		t.Fatalf("flags = %+v", r)
	}
	if r.Episodes != 3 || r.MinEpisodes != 7 {
		t.Errorf("episodes = %d / %d, want 3 / 7 (the empty 4화 is not one yet)", r.Episodes, r.MinEpisodes)
	}
	if r.Meeting != 1 {
		t.Errorf("meeting = %d, want 1", r.Meeting)
	}
	wantShort := []EpisodeLength{
		{NodeID: "ep2", SceneID: "s1", Label: "2화", Chars: 3925, Delta: -1075},
		{NodeID: "ep3", SceneID: "ep3", Label: "3화", Chars: 4380, Delta: -620},
	}
	if !reflect.DeepEqual(r.Short, wantShort) {
		t.Errorf("short = %+v, want %+v", r.Short, wantShort)
	}
	if r.TotalChars != 13505 || r.TotalState != charcount.LengthShort || r.TotalDelta != -16495 {
		t.Errorf("total = %d %s %d, want 13505 short -16495", r.TotalChars, r.TotalState, r.TotalDelta)
	}
	if !r.NeedsSynopsis || r.HasSynopsis || r.Ready {
		t.Errorf("synopsis/ready = %v/%v/%v, want needed, missing, not ready", r.NeedsSynopsis, r.HasSynopsis, r.Ready)
	}
}

func TestCheck_readyWhenEveryRuleIsMet(t *testing.T) {
	nodes := []node.Node{
		mk("arc", "", 0, node.KindContainer, "1권", 0),
		mk("ep1", "arc", 0, node.KindLeaf, "1화", 5200),
		mk("ep2", "arc", 1, node.KindLeaf, "2화", 5000),
	}
	p := project.Project{OutlinePreset: project.OutlinePresetWebNovel, EpisodeCharMin: 5000, EpisodeCharTarget: 5300,
		EpisodeCharMax: 5500, SubmissionMinEpisodes: 2, SubmissionTotalMax: 12000, SubmissionNeedsSynopsis: true,
		Synopsis: "검을 잃은 검객의 이야기."}
	r := Check(nodes, p)
	if !r.Ready || r.Meeting != 2 || r.TotalState != charcount.LengthProgress {
		t.Fatalf("report = %+v, want ready with both episodes meeting", r)
	}
}

func TestCheck_longEpisodeAndLongWork(t *testing.T) {
	p := contest()
	p.EpisodeCharMax = 5000
	p.SubmissionMinEpisodes, p.SubmissionTotalMin, p.SubmissionTotalMax, p.SubmissionNeedsSynopsis = 0, 0, 10000, false
	r := Check(webNovel(), p)
	if len(r.Long) != 1 || r.Long[0].Label != "1화" || r.Long[0].Delta != 200 {
		t.Errorf("long = %+v, want 1화 +200", r.Long)
	}
	if r.TotalState != charcount.LengthLong || r.TotalDelta != 3505 || r.Ready {
		t.Errorf("total = %s %d ready %v, want long +3505, not ready", r.TotalState, r.TotalDelta, r.Ready)
	}
}

func TestCheck_noEpisodeBoundsLeavesEpisodeLinesOut(t *testing.T) {
	p := contest()
	p.EpisodeCharMin = 0
	r := Check(webNovel(), p)
	if r.EpisodeRule || r.Meeting != 0 || len(r.Short) != 0 {
		t.Fatalf("report judged episodes with no bounds: %+v", r)
	}
	if r.Episodes != 3 {
		t.Errorf("episodes = %d, want 3", r.Episodes)
	}
}

func TestCheck_shortStoryIsOnePiece(t *testing.T) {
	nodes := []node.Node{
		mk("part", "", 0, node.KindContainer, "본문", 0),
		mk("s1", "part", 0, node.KindLeaf, "씬 1", 9000),
		mk("s2", "part", 1, node.KindLeaf, "씬 2", 7000),
	}
	p := project.Project{OutlinePreset: project.OutlinePresetNovel, EpisodeCharMin: 5000, EpisodeCharTarget: 5300,
		SubmissionTotalMin: 15000, SubmissionTotalMax: 30000}
	r := Check(nodes, p)
	if r.Structured || r.EpisodeRule || len(r.Short) != 0 {
		t.Fatalf("a short story was judged by episode: %+v", r)
	}
	if r.TotalChars != 16000 || r.TotalState != charcount.LengthOK || !r.Ready {
		t.Fatalf("total = %d %s ready %v, want 16000 ok ready", r.TotalChars, r.TotalState, r.Ready)
	}
}

func TestCheck_workWithNoRule(t *testing.T) {
	r := Check(webNovel(), project.Project{OutlinePreset: project.OutlinePresetWebNovel, EpisodeCharTarget: 5000})
	if r.HasRule {
		t.Fatalf("has_rule = true for a work with none")
	}
	if !r.Ready {
		t.Fatalf("a work with no rule has nothing unmet: %+v", r)
	}
}
