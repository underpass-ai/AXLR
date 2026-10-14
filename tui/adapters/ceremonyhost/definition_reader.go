package ceremonyhost

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/underpass-ai/AXLR/tui/application"
)

var _ application.CeremonyDefinitionPort = Engine{}

// definitionYAML is the part of a MADE definition the generic driver needs:
// the inputs, each step's state and prompt, and the terminal states.
// made_explain_ceremony_draft would describe it in prose, but MADE's work
// grant cannot call it (13 October 2026), so the YAML is read back.
type definitionYAML struct {
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	Description string `yaml:"description"`
	Inputs      struct {
		Required []string `yaml:"required"`
		Optional []string `yaml:"optional"`
	} `yaml:"inputs"`
	States []struct {
		ID       string `yaml:"id"`
		Initial  bool   `yaml:"initial"`
		Terminal bool   `yaml:"terminal"`
	} `yaml:"states"`
	Steps []struct {
		ID     string `yaml:"id"`
		State  string `yaml:"state"`
		Config struct {
			Prompt string `yaml:"prompt"`
		} `yaml:"config"`
	} `yaml:"steps"`
	Transitions []struct {
		From    string `yaml:"from"`
		To      string `yaml:"to"`
		Trigger string `yaml:"trigger"`
	} `yaml:"transitions"`
}

// Describe reads one published definition back and keeps what the generic
// driver needs.
func (e Engine) Describe(ctx context.Context, name, version string) (application.CeremonyDefinition, error) {
	found, err := e.made(ctx, "made_get_ceremony_definition", map[string]any{"ceremony": name, "version": version})
	if err != nil {
		return application.CeremonyDefinition{}, err
	}
	text, _ := found["definition_yaml"].(string)
	if strings.TrimSpace(text) == "" {
		return application.CeremonyDefinition{}, fmt.Errorf("MADE returned no YAML for %s %s", name, version)
	}
	return parseDefinition(text)
}

// parseDefinition reads a definition's YAML.
func parseDefinition(text string) (application.CeremonyDefinition, error) {
	var parsed definitionYAML
	if err := yaml.Unmarshal([]byte(text), &parsed); err != nil {
		return application.CeremonyDefinition{}, fmt.Errorf("read the definition: %w", err)
	}
	if parsed.Name == "" || len(parsed.Steps) == 0 {
		return application.CeremonyDefinition{}, errors.New("the definition names no steps")
	}
	definition := application.CeremonyDefinition{Name: parsed.Name, Version: parsed.Version, Description: parsed.Description, Inputs: parsed.Inputs.Required, Prompts: map[string]string{}, Transitions: map[string][]string{}}
	for _, state := range parsed.States {
		if state.Terminal {
			definition.Terminal = append(definition.Terminal, state.ID)
		}
		if state.Initial {
			definition.Initial = state.ID
		}
	}
	for _, step := range parsed.Steps {
		definition.Steps = append(definition.Steps, application.DefinitionStep{ID: step.ID, State: step.State})
		definition.Prompts[step.ID] = strings.TrimSpace(step.Config.Prompt)
	}
	for _, transition := range parsed.Transitions {
		definition.Transitions[transition.From] = append(definition.Transitions[transition.From], transition.Trigger)
	}
	sort.Strings(definition.Terminal)
	return definition, nil
}

// List names the published definitions, newest version of each name
// first among equals by name.
func (e Engine) List(ctx context.Context) ([]application.CeremonyDefinitionSummary, error) {
	listed, err := e.made(ctx, "made_list_ceremony_definitions", map[string]any{})
	if err != nil {
		return nil, err
	}
	var out []application.CeremonyDefinitionSummary
	items, _ := listed["definitions"].([]any)
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		name, _ := item["ceremony"].(string)
		version, _ := item["version"].(string)
		if name == "" || version == "" {
			continue
		}
		description, _ := item["description"].(string)
		steps, _ := item["step_count"].(float64)
		out = append(out, application.CeremonyDefinitionSummary{Name: name, Version: version, Description: description, Steps: int(steps)})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return application.VersionLess(out[j].Version, out[i].Version)
	})
	return out, nil
}
