package pipeline

import (
	"fmt"
	"sync"

	"gopkg.in/yaml.v3"
)

// InputDef describes one pipeline input (melange-style): description, optional default, required.
type InputDef struct {
	Description string
	Default     string
	Required    bool
}

// UnmarshalYAML supports short form (string = default) or long form (object with description, default, required).
func (i *InputDef) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err == nil {
		i.Default = s
		return nil
	}
	var m struct {
		Description string `yaml:"description"`
		Default     string `yaml:"default"`
		Required    bool   `yaml:"required"`
	}
	if err := value.Decode(&m); err != nil {
		return err
	}
	i.Description = m.Description
	i.Default = m.Default
	i.Required = m.Required
	return nil
}

// PipelineNeeds declares what a pipeline needs (e.g. packages to install in the build environment).
type PipelineNeeds struct {
	Packages []string `yaml:"packages,omitempty"`
}

// PipelineDef is the structure of a pipeline YAML file.
type PipelineDef struct {
	Name   string              `yaml:"name,omitempty"`
	Needs  PipelineNeeds       `yaml:"needs,omitempty"`
	Inputs map[string]InputDef `yaml:"inputs,omitempty"` // input name -> schema (default, required)
	Runs   string              `yaml:"runs,omitempty"`
}

var (
	loadedPipelines    map[string]*PipelineDef
	loadedPipelinesMu  sync.Mutex
	templateResolverFn func(name string) ([]byte, error)
)

// RegisterTemplateResolver sets the callback to locate and read step definition files.
func RegisterTemplateResolver(resolver func(name string) ([]byte, error)) {
	templateResolverFn = resolver
}

// GetPipeline loads and returns the pipeline definition for the given name (e.g. "fetch", "autoconf/configure").
func GetPipeline(name string) (*PipelineDef, error) {
	loadedPipelinesMu.Lock()
	defer loadedPipelinesMu.Unlock()
	if loadedPipelines == nil {
		loadedPipelines = make(map[string]*PipelineDef)
	}
	if def, ok := loadedPipelines[name]; ok {
		return def, nil
	}
	if templateResolverFn == nil {
		return nil, fmt.Errorf("no pipeline template resolver registered")
	}
	data, err := templateResolverFn(name)
	if err != nil {
		return nil, fmt.Errorf("pipeline %q not found: %w", name, err)
	}
	var def PipelineDef
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("pipeline %q: %w", name, err)
	}
	if def.Runs == "" {
		return nil, fmt.Errorf("pipeline %q: missing runs", name)
	}
	if def.Inputs == nil {
		def.Inputs = make(map[string]InputDef)
	}
	loadedPipelines[name] = &def
	return &def, nil
}
