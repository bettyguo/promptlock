// Package prompt parses, validates, discovers, and represents promptlock
// prompts. The on-disk format is locked in docs/format.md.
package prompt

import (
	"github.com/promptlock/promptlock/internal/version"
)

// Prompt is the parsed representation of a single .prompt.md file.
type Prompt struct {
	// Path is the path on disk. Relative to repo root when discovered via Discover.
	Path string
	// Frontmatter is the parsed YAML frontmatter block.
	Frontmatter Frontmatter
	// Body holds the parsed System/User/Assistant sections.
	Body Body
	// Raw is the unparsed file contents, byte-for-byte. Used for hashing.
	Raw []byte
}

// Frontmatter is the typed view of the YAML block. Unknown keys are preserved
// in Extra for forward-compat (a parser warning is emitted but the file parses).
type Frontmatter struct {
	ID          string         `yaml:"id"`
	Version     string         `yaml:"version"` // raw string; parsed via ParsedVersion
	Description string         `yaml:"description,omitempty"`
	Model       string         `yaml:"model"`
	Temperature *float64       `yaml:"temperature,omitempty"`
	MaxTokens   *int           `yaml:"max_tokens,omitempty"`
	TopP        *float64       `yaml:"top_p,omitempty"`
	Stop        []string       `yaml:"stop,omitempty"`
	Tags        []string       `yaml:"tags,omitempty"`
	Inputs      []Input        `yaml:"inputs,omitempty"`
	Outputs     *Outputs       `yaml:"outputs,omitempty"`
	Evals       []Eval         `yaml:"evals,omitempty"`
	Metadata    map[string]any `yaml:"metadata,omitempty"`

	// ParsedVersion is the validated semver. Populated by Validate.
	ParsedVersion version.Version `yaml:"-"`

	// Extra holds any frontmatter keys we don't recognize. Surfaces warnings,
	// preserves data round-trip-safe.
	Extra map[string]any `yaml:"-"`
}

// Input is one declared template variable.
type Input struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	Type        string `yaml:"type,omitempty"` // string|integer|float|boolean|array|object; default "string"
	Required    *bool  `yaml:"required,omitempty"`
	Default     any    `yaml:"default,omitempty"`
}

// IsRequired returns true unless Required is explicitly false.
func (i Input) IsRequired() bool { return i.Required == nil || *i.Required }

// EffectiveType returns Type or "string" when unset.
func (i Input) EffectiveType() string {
	if i.Type == "" {
		return "string"
	}
	return i.Type
}

// Outputs declares the expected output shape.
type Outputs struct {
	Schema map[string]any `yaml:"schema,omitempty"`
}

// Eval is one declared eval suite for the prompt.
type Eval struct {
	Dataset      string         `yaml:"dataset"`
	Metric       string         `yaml:"metric"`
	Threshold    float64        `yaml:"threshold"`
	Provider     string         `yaml:"provider,omitempty"`
	Model        string         `yaml:"model,omitempty"`
	Temperature  *float64       `yaml:"temperature,omitempty"`
	MetricConfig map[string]any `yaml:"metric_config,omitempty"`
}

// Body holds the parsed sections from the markdown body.
type Body struct {
	System    string
	User      string
	Assistant string
}
