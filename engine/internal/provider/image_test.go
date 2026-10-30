package provider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/tars/pkg/llm"
)

type fakeImageClient struct {
	fakeClient
	opts llm.ProviderOptions
}

func (f fakeImageClient) GenerateImage(context.Context, llm.ImageRequest) (llm.ImageResult, error) {
	return llm.ImageResult{}, nil
}

// newImageSource is newSource with a factory that records what it was asked
// for and hands back a client able to draw.
func newImageSource(t *testing.T) (*Source, *settings.Store, string, *llm.ProviderOptions) {
	t.Helper()
	src, st, codexHome := newSource(t)
	seen := &llm.ProviderOptions{}
	src.WithFactory(func(opts llm.ProviderOptions) (llm.Client, error) {
		*seen = opts
		return fakeImageClient{opts: opts}, nil
	})
	return src, st, codexHome, seen
}

func setImage(t *testing.T, st *settings.Store, id, model string, consented bool) {
	t.Helper()
	ip := settings.ImageProviderPatch{Model: strPtr(model), ConsentedAt: int64Ptr(0)}
	if consented {
		ip.ConsentedAt = int64Ptr(1700000000000)
	}
	if _, err := st.Set(context.Background(), settings.Patch{
		ImageProvider:  strPtr(id),
		ImageProviders: map[string]settings.ImageProviderPatch{id: ip},
	}); err != nil {
		t.Fatalf("set image %s: %v", id, err)
	}
}

func loginCodex(t *testing.T, codexHome string) {
	t.Helper()
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexHome, "auth.json"), []byte(`{"tokens":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestImageGenerator_offUntilAProviderIsChosen(t *testing.T) {
	src, _, _, _ := newImageSource(t)
	_, _, err := src.ImageGenerator(context.Background(), "")
	if reasonOf(t, err) != rpc.ReasonImageGenerationOff {
		t.Fatalf("err = %v", err)
	}
}

func TestImageGenerator_needsACredentialAndItsOwnConsent(t *testing.T) {
	src, st, _, _ := newImageSource(t)
	setImage(t, st, settings.ProviderGeminiNative, "gemini-3.1-flash-image", true)
	if _, _, err := src.ImageGenerator(context.Background(), ""); reasonOf(t, err) != rpc.ReasonProviderNotConfigured {
		t.Fatalf("without a key: %v", err)
	}

	// The agent's consent is not the image consent.
	configure(t, st, settings.ProviderGeminiNative, "AIza-test", true)
	setImage(t, st, settings.ProviderGeminiNative, "gemini-3.1-flash-image", false)
	if _, _, err := src.ImageGenerator(context.Background(), ""); reasonOf(t, err) != rpc.ReasonImageConsentRequired {
		t.Fatalf("without image consent: %v", err)
	}
}

func TestImageGenerator_geminiUsesTheChosenImageModelAndTheAgentKey(t *testing.T) {
	src, st, _, seen := newImageSource(t)
	configure(t, st, settings.ProviderGeminiNative, "AIza-test", false)
	setImage(t, st, settings.ProviderGeminiNative, "gemini-3.1-flash-image", true)

	gen, r, err := src.ImageGenerator(context.Background(), "")
	if err != nil {
		t.Fatalf("ImageGenerator: %v", err)
	}
	if gen == nil || r.Model != "gemini-3.1-flash-image" {
		t.Fatalf("gen=%v resolved=%v", gen, r)
	}
	if seen.Provider != settings.ProviderGeminiNative || seen.Model != "gemini-3.1-flash-image" || seen.APIKey != "AIza-test" {
		t.Fatalf("factory got %+v", *seen)
	}
}

func TestImageGenerator_geminiHasNoDefaultModel(t *testing.T) {
	src, st, _, _ := newImageSource(t)
	configure(t, st, settings.ProviderGeminiNative, "AIza-test", false)
	setImage(t, st, settings.ProviderGeminiNative, "", true)
	if _, _, err := src.ImageGenerator(context.Background(), ""); reasonOf(t, err) != rpc.ReasonImageModelRequired {
		t.Fatalf("err = %v", err)
	}
}

func TestImageGenerator_codexDefaultsToTheAccountsFirstModel(t *testing.T) {
	src, st, codexHome, seen := newImageSource(t)
	loginCodex(t, codexHome)
	src.WithFetcher(&fakeFetcher{models: []string{"gpt-5.6-sol", "gpt-5.6-terra"}})
	setImage(t, st, settings.ProviderOpenAICodex, "", true)

	_, r, err := src.ImageGenerator(context.Background(), "")
	if err != nil {
		t.Fatalf("ImageGenerator: %v", err)
	}
	if r.Model != "gpt-5.6-sol" || seen.Model != "gpt-5.6-sol" || seen.AuthConfig.CodexHome != codexHome {
		t.Fatalf("resolved %v, factory got %+v", r, *seen)
	}

	src.WithFetcher(&fakeFetcher{})
	if _, _, err := src.ImageGenerator(context.Background(), ""); reasonOf(t, err) != rpc.ReasonImageModelRequired {
		t.Fatalf("empty model list: %v", err)
	}
}

func TestImageGenerator_rejectsAProviderThatCannotDraw(t *testing.T) {
	src, st, _, _ := newImageSource(t)
	configure(t, st, settings.ProviderAnthropic, "sk-ant", true)
	if _, _, err := src.ImageGenerator(context.Background(), settings.ProviderAnthropic); reasonOf(t, err) != rpc.ReasonProviderNotConfigured {
		t.Fatalf("err = %v", err)
	}

	src.WithFactory(func(llm.ProviderOptions) (llm.Client, error) { return fakeClient{}, nil })
	configure(t, st, settings.ProviderGeminiNative, "AIza-test", false)
	setImage(t, st, settings.ProviderGeminiNative, "gemini-3.1-flash-image", true)
	if _, _, err := src.ImageGenerator(context.Background(), ""); err == nil {
		t.Fatal("a client without GenerateImage must be an error")
	}
}

func TestListImageModels(t *testing.T) {
	src, st, codexHome, _ := newImageSource(t)
	configure(t, st, settings.ProviderGeminiNative, "AIza-test", false)
	src.WithFetcher(&fakeFetcher{models: []string{"models/gemini-3.1-flash-image", "gemini-3.8-flash", "gemini-2.5-flash-image", "text-embedding-004"}})

	got, err := src.ListImageModels(context.Background(), settings.ProviderGeminiNative)
	if err != nil {
		t.Fatalf("ListImageModels: %v", err)
	}
	if fmt.Sprint(got) != "[gemini-2.5-flash-image gemini-3.1-flash-image]" {
		t.Fatalf("gemini image models = %v", got)
	}

	loginCodex(t, codexHome)
	src.WithFetcher(&fakeFetcher{models: []string{"gpt-5.6-sol", "gpt-5.6-luna"}})
	got, err = src.ListImageModels(context.Background(), settings.ProviderOpenAICodex)
	if err != nil || fmt.Sprint(got) != "[gpt-5.6-sol gpt-5.6-luna]" {
		t.Fatalf("codex models = %v, %v (want the account's order)", got, err)
	}

	if _, err := src.ListImageModels(context.Background(), settings.ProviderAnthropic); reasonOf(t, err) != rpc.ReasonProviderNotConfigured {
		t.Fatalf("anthropic: %v", err)
	}
}

func TestClassifyImage(t *testing.T) {
	cases := map[string]struct {
		err    error
		reason string
	}{
		"refused":    {fmt.Errorf("wrapped: %w", llm.ErrImageRefused), rpc.ReasonImageRefused},
		"billing":    {&llm.ProviderError{Provider: "gemini-native", StatusCode: 429, Cause: llm.ErrImageBillingRequired}, rpc.ReasonImageBillingRequired},
		"rate limit": {&llm.ProviderError{Provider: "gemini-native", StatusCode: 429}, rpc.ReasonProviderRateLimited},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := reasonOf(t, ClassifyImage(settings.ProviderGeminiNative, tc.err)); got != tc.reason {
				t.Fatalf("reason = %q, want %q", got, tc.reason)
			}
		})
	}
	if ClassifyImage("x", nil) != nil {
		t.Fatal("nil must stay nil")
	}
	if !errors.Is(ClassifyImage("x", llm.ErrImageRefused), llm.ErrImageRefused) {
		t.Fatal("the cause must survive")
	}
}

type recordingImageClient struct {
	fakeClient
	got *llm.ImageRequest
	res llm.ImageResult
	err error
}

func (c recordingImageClient) GenerateImage(_ context.Context, req llm.ImageRequest) (llm.ImageResult, error) {
	*c.got = req
	return c.res, c.err
}

func TestDraw(t *testing.T) {
	src, st, _, _ := newImageSource(t)
	configure(t, st, settings.ProviderGeminiNative, "AIza-test", false)
	setImage(t, st, settings.ProviderGeminiNative, "gemini-3.1-flash-image", true)
	got := &llm.ImageRequest{}
	src.WithFactory(func(llm.ProviderOptions) (llm.Client, error) {
		return recordingImageClient{got: got, res: llm.ImageResult{Data: []byte("img"), MediaType: "image/jpeg", RevisedPrompt: "r"}}, nil
	})

	res, err := src.Draw(context.Background(), DrawRequest{
		Prompt: "p", Aspect: "portrait", References: []DrawReference{{MIME: "image/png", Data: []byte("ref")}},
	})
	if err != nil {
		t.Fatalf("Draw: %v", err)
	}
	if string(res.Data) != "img" || res.MIME != "image/jpeg" || res.RevisedPrompt != "r" || res.Provider != settings.ProviderGeminiNative || res.Model != "gemini-3.1-flash-image" {
		t.Fatalf("result = %+v", res)
	}
	if got.Prompt != "p" || got.Aspect != llm.ImageAspectPortrait || len(got.References) != 1 ||
		got.References[0].Type != "image" || got.References[0].MediaType != "image/png" || got.References[0].Data != "cmVm" {
		t.Fatalf("request = %+v", *got)
	}

	src.WithFactory(func(llm.ProviderOptions) (llm.Client, error) {
		return recordingImageClient{got: got, err: fmt.Errorf("x: %w", llm.ErrImageRefused)}, nil
	})
	if _, err := src.Draw(context.Background(), DrawRequest{Prompt: "p"}); reasonOf(t, err) != rpc.ReasonImageRefused {
		t.Fatalf("refusal: %v", err)
	}
}

func TestImageList(t *testing.T) {
	src, st, _, _ := newImageSource(t)
	configure(t, st, settings.ProviderGeminiNative, "AIza-test", false)
	setImage(t, st, settings.ProviderGeminiNative, "gemini-3.1-flash-image", true)

	got := src.ImageList()
	if len(got) != 2 || got[0].ID != settings.ProviderOpenAICodex || got[1].ID != settings.ProviderGeminiNative {
		t.Fatalf("list = %+v", got)
	}
	if got[0].Active || got[0].Configured || got[0].Consented {
		t.Fatalf("codex = %+v, want nothing set", got[0])
	}
	if g := got[1]; !g.Active || !g.Configured || !g.Consented || g.Model != "gemini-3.1-flash-image" {
		t.Fatalf("gemini = %+v", g)
	}
}
