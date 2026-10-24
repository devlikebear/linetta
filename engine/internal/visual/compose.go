package visual

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/entity"
)

// InputError is a request the caller can fix: an unknown character, a missing
// scene. Its text is written for the agent or writer who sent it.
type InputError struct{ msg string }

func (e *InputError) Error() string { return e.msg }

func inputErr(format string, args ...any) error {
	return &InputError{msg: fmt.Sprintf(format, args...)}
}

// ResolveCharacters picks the characters a request names out of a work's
// entities, by id or by name or alias. With neither, it returns every
// character. The result is sorted by name.
func ResolveCharacters(all []entity.Entity, ids, names []string) ([]entity.Entity, error) {
	all = append([]entity.Entity(nil), all...)
	sort.Slice(all, func(i, j int) bool {
		if all[i].Name == all[j].Name {
			return all[i].ID < all[j].ID
		}
		return all[i].Name < all[j].Name
	})
	chars := []entity.Entity{}
	available := []string{}
	for _, e := range all {
		if e.Kind == entity.KindCharacter {
			chars = append(chars, e)
			available = append(available, e.Name)
		}
	}
	if len(ids) == 0 && len(names) == 0 {
		return chars, nil
	}
	selected := map[string]bool{}
	for _, id := range ids {
		found := false
		for _, e := range chars {
			if e.ID == strings.TrimSpace(id) {
				selected[e.ID] = true
				found = true
			}
		}
		if !found {
			return nil, inputErr("character %q is not in this work", id)
		}
	}
	for _, name := range names {
		matches := []string{}
		for _, e := range chars {
			for _, candidate := range append([]string{e.Name}, e.Aliases...) {
				if strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(name)) {
					matches = append(matches, e.ID)
					break
				}
			}
		}
		if len(matches) == 0 {
			return nil, inputErr("unknown character %q; available characters: %s", name, strings.Join(available, ", "))
		}
		if len(matches) > 1 {
			return nil, inputErr("ambiguous character %q; use entity_ids", name)
		}
		selected[matches[0]] = true
	}
	out := []entity.Entity{}
	for _, e := range chars {
		if selected[e.ID] {
			out = append(out, e)
		}
	}
	return out, nil
}

// ComposeInput is what an illustration prompt is built from.
type ComposeInput struct {
	EntityIDs []string
	Names     []string
	Scene     string
	Format    string // FormatSingle when empty
}

// Validate checks what can be checked without reading the work, so a caller
// can refuse a bad request before looking anything up.
func (in ComposeInput) Validate() error {
	if strings.TrimSpace(in.Scene) == "" {
		return inputErr("scene is required")
	}
	if len(in.EntityIDs) == 0 && len(in.Names) == 0 {
		// Unlike reading the cast, defaulting to every character here would
		// put people in the picture who are not in the scene.
		return inputErr("name the characters in the picture with names or entity_ids")
	}
	if in.Format != "" && in.Format != FormatSingle && in.Format != FormatFourPanel {
		return inputErr("format must be single or four_panel")
	}
	return nil
}

// ComposedCharacter is one character a composition drew from.
type ComposedCharacter struct {
	EntityID string `json:"entity_id"`
	Name     string `json:"name"`
}

// Composition is an illustration prompt and what went into it.
type Composition struct {
	Prompt         string              `json:"prompt"`
	NegativePrompt string              `json:"negative_prompt"`
	Format         string              `json:"format"`
	Characters     []ComposedCharacter `json:"characters"`
	// ReferenceImageIDs are the characters' reference images, in character
	// order and each character's own order.
	ReferenceImageIDs []string `json:"reference_image_ids"`
	// MissingSheets names the characters drawn without a visual sheet, whose
	// look the provider will invent.
	MissingSheets []string `json:"missing_sheets"`
}

// Compose builds the prompt for a picture of the named characters from a
// work's entities, their visual sheets and the work's art style. It calls no
// model: the same input always gives the same prompt. Every illustration
// prompt — the MCP tool's and the app's — comes from here.
func (r *Repo) Compose(ctx context.Context, projectID string, entities []entity.Entity, in ComposeInput) (Composition, error) {
	out := Composition{Characters: []ComposedCharacter{}, ReferenceImageIDs: []string{}, MissingSheets: []string{}}
	if err := in.Validate(); err != nil {
		return out, err
	}
	if in.Format == "" {
		in.Format = FormatSingle
	}
	chars, err := ResolveCharacters(entities, in.EntityIDs, in.Names)
	if err != nil {
		return out, err
	}
	style, err := r.GetArtStyle(ctx, projectID)
	if err != nil {
		return out, fmt.Errorf("could not read art style: %w", err)
	}
	designs := []CharacterVisual{}
	for _, e := range chars {
		sheet, err := r.GetSheet(ctx, e.ID)
		if err != nil {
			return out, fmt.Errorf("could not read design: %w", err)
		}
		images, err := r.ListImages(ctx, e.ID)
		if err != nil {
			return out, fmt.Errorf("could not read references: %w", err)
		}
		designs = append(designs, CharacterVisual{Name: e.Name, Role: e.Role, Sheet: sheet, Images: images})
		out.Characters = append(out.Characters, ComposedCharacter{e.ID, e.Name})
		if sheet.Empty() {
			out.MissingSheets = append(out.MissingSheets, e.Name)
		}
		for _, img := range images {
			out.ReferenceImageIDs = append(out.ReferenceImageIDs, img.ID)
		}
	}
	out.Format = in.Format
	out.NegativePrompt = style.NegativePrompt
	out.Prompt = BuildPrompt(style, designs, in.Scene, in.Format)
	return out, nil
}
