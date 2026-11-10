package storycontext

import (
	"context"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/charcount"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
)

func TestRenderEpisodeRule(t *testing.T) {
	for _, tc := range []struct {
		name string
		meta ProjectMeta
		lang string
		want string
	}{
		{"minimum", ProjectMeta{EpisodeCharMin: 5000, EpisodeCharTarget: 5300}, "ko",
			"회차 분량: 최소 5,000자(공백 포함), 5,300자 안팎을 목표로"},
		{"range", ProjectMeta{EpisodeCharMin: 4500, EpisodeCharTarget: 5000, EpisodeCharMax: 5500}, "ko",
			"회차 분량: 4,500~5,500자(공백 포함), 5,000자 안팎을 목표로"},
		{"maximum", ProjectMeta{EpisodeCharTarget: 5000, EpisodeCharMax: 5500, CharCountBasis: charcount.WithoutSpaces}, "ko",
			"회차 분량: 최대 5,500자(공백 제외), 5,000자 안팎을 목표로"},
		{"target only keeps its old line", ProjectMeta{EpisodeCharTarget: 3000}, "ko",
			"회차 목표 분량: 3000자(공백 포함)"},
		{"english", ProjectMeta{EpisodeCharMin: 5000, EpisodeCharTarget: 5300}, "en",
			"Episode length: at least 5,000 characters (including spaces), aiming for about 5,300"},
		{"japanese", ProjectMeta{EpisodeCharMin: 4500, EpisodeCharTarget: 5000, EpisodeCharMax: 5500}, "ja",
			"各話の分量: 4,500〜5,500字(空白含む)、5,000字前後を目標に"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderEpisodeRule(tc.meta, tc.lang); got != tc.want {
				t.Fatalf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestRenderEpisodeProgress(t *testing.T) {
	ep := func(chars, others int, state string, delta int) *EpisodeLengthBrief {
		return &EpisodeLengthBrief{Label: "2화", Chars: chars, OtherScenesChars: others, State: state, Delta: delta}
	}
	for _, tc := range []struct {
		name string
		meta ProjectMeta
		lang string
		want string
	}{
		{"short", ProjectMeta{EpisodeCharMin: 5000, Episode: ep(3925, 2100, charcount.LengthShort, -1075)}, "ko",
			"이 회차(2화): 지금 3,925자 (이 씬 제외 2,100자) · 최소까지 1,075자"},
		{"long", ProjectMeta{EpisodeCharMax: 5500, Episode: ep(5820, 3000, charcount.LengthLong, 320)}, "ko",
			"이 회차(2화): 지금 5,820자 (이 씬 제외 3,000자) · 최대보다 320자 많음"},
		{"within", ProjectMeta{EpisodeCharMin: 5000, Episode: ep(5100, 0, charcount.LengthOK, 0)}, "ko",
			"이 회차(2화): 지금 5,100자 (이 씬 제외 0자) · 기준 안"},
		{"under a maximum only", ProjectMeta{EpisodeCharMax: 5500, Episode: ep(4000, 0, charcount.LengthProgress, 0)}, "ko",
			"이 회차(2화): 지금 4,000자 (이 씬 제외 0자) · 최대까지 1,500자"},
		{"english", ProjectMeta{EpisodeCharMin: 5000, Episode: ep(3925, 2100, charcount.LengthShort, -1075)}, "en",
			"This episode (2화): 3,925 characters so far (2,100 without this scene) · 1,075 short of the minimum"},
		{"japanese", ProjectMeta{EpisodeCharMin: 5000, Episode: ep(3925, 2100, charcount.LengthShort, -1075)}, "ja",
			"この話(2화): 現在3,925字(この場面を除く2,100字) · 最低まであと1,075字"},
		{"no episode", ProjectMeta{EpisodeCharMin: 5000}, "ko", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderEpisodeProgress(tc.meta, tc.lang); got != tc.want {
				t.Fatalf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

// A work that sets no bound briefs exactly as it did before bounds existed.
func TestProjectMetaWithoutBoundsIsUnchanged(t *testing.T) {
	m := ProjectMeta{Genres: []string{"무협"}, LengthTarget: "series", DefaultPOV: "first", EpisodeCharTarget: 3000}
	want := "장르: 무협 · 분량: 시리즈 · 시점: 1인칭 · 회차 목표 분량: 3000자(공백 포함)"
	if got := renderProjectMeta(m, "ko"); got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestBriefCarriesTheEpisodeRuleAndProgress(t *testing.T) {
	f := newCtxFixture(t)
	ctx := context.Background()
	p := f.project(t, project.NewInput{Title: "t", LengthTarget: "series", DefaultPOV: "first",
		OutlinePreset: project.OutlinePresetWebNovel})
	first := *p.LastOpenedNodeID // 1화, a scene of its own under 1권

	ep2, err := f.nodes.CreateSibling(ctx, first, node.KindContainer, "2화", "", 1100)
	if err != nil {
		t.Fatal(err)
	}
	var scenes []node.Node
	for i, chars := range []int{2100, 1825} {
		s, err := f.nodes.CreateChild(ctx, ep2.ID, node.KindLeaf, "씬", "", int64(1200+i))
		if err != nil {
			t.Fatal(err)
		}
		doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` +
			strings.Repeat("가", chars) + `"}]}]}`
		if err := f.nodes.UpdateContent(ctx, s.ID, doc, int64(1300+i)); err != nil {
			t.Fatal(err)
		}
		scenes = append(scenes, s)
	}
	brief := func(id string) (Context, string) {
		c, err := f.builder().Build(ctx, id, "", "", Options{Language: "ko"})
		if err != nil {
			t.Fatal(err)
		}
		_, user := Render(c)
		return c, user
	}

	c, user := brief(scenes[0].ID)
	for _, want := range []string{
		"회차 분량: 최소 5,000자(공백 포함), 5,300자 안팎을 목표로",
		"이 회차(2화): 지금 3,925자 (이 씬 제외 1,825자) · 최소까지 1,075자",
	} {
		if !strings.Contains(user, want) {
			t.Errorf("brief is missing %q:\n%s", want, user)
		}
	}
	if c.Project.Episode == nil || c.Project.Episode.State != charcount.LengthShort {
		t.Errorf("episode = %+v, want short", c.Project.Episode)
	}

	if _, user := brief(first); !strings.Contains(user, "이 회차(1화): 지금 0자 (이 씬 제외 0자) · 최소까지 5,000자") {
		t.Errorf("an empty episode's brief is missing its progress:\n%s", user)
	}

	// Turning the section off drops both lines with the rest of the settings.
	off := false
	c, err = f.builder().Build(ctx, scenes[0].ID, "", "", Options{Language: "ko", Context: ContextSelection{ProjectMeta: &off}})
	if err != nil {
		t.Fatal(err)
	}
	if _, user := Render(c); strings.Contains(user, "이 회차") || strings.Contains(user, "회차 분량") {
		t.Errorf("project settings were off but the episode length stayed:\n%s", user)
	}
}

func TestBriefOfAWorkWithoutBoundsHasNoEpisodeLine(t *testing.T) {
	f := newCtxFixture(t)
	ctx := context.Background()
	p := f.project(t, project.NewInput{Title: "t", LengthTarget: "series", DefaultPOV: "first",
		OutlinePreset: project.OutlinePresetWebNovel})
	zero := 0
	if _, err := f.projects.Update(ctx, 1500, project.UpdateInput{ID: p.ID, EpisodeCharMin: &zero}); err != nil {
		t.Fatal(err)
	}
	c, err := f.builder().Build(ctx, *p.LastOpenedNodeID, "", "", Options{Language: "ko"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Project.Episode != nil {
		t.Fatalf("episode = %+v, want none", c.Project.Episode)
	}
	if _, user := Render(c); strings.Contains(user, "이 회차") || strings.Contains(user, "회차 분량") {
		t.Fatalf("unbounded work briefed an episode length:\n%s", user)
	}
}
