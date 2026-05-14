// Package diff produces semantic diffs of prompts: structural diff over
// frontmatter, word-level diff over body sections, with template-var awareness.
package diff

// Result is the structured output of Diff.
type Result struct {
	PromptID            string             `json:"prompt_id"`
	BeforeRef           string             `json:"before_ref,omitempty"`
	AfterRef            string             `json:"after_ref,omitempty"`
	BeforeMissing       bool               `json:"before_missing,omitempty"`
	AfterMissing        bool               `json:"after_missing,omitempty"`
	VersionChange       *VersionChange     `json:"version_change,omitempty"`
	FrontmatterChanges  []FrontmatterChange `json:"frontmatter_changes,omitempty"`
	InputsChanges       []InputChange      `json:"inputs_changes,omitempty"`
	BodyChanges         []SectionChange    `json:"body_changes,omitempty"`
	HasChanges          bool               `json:"has_changes"`
}

// VersionChange records the before/after of the version field.
type VersionChange struct {
	Before string `json:"before"`
	After  string `json:"after"`
}

// FrontmatterChange describes a change to one frontmatter key (or sub-key).
type FrontmatterChange struct {
	Path   string `json:"path"`
	Before any    `json:"before,omitempty"`
	After  any    `json:"after,omitempty"`
	Kind   string `json:"kind"` // "added" | "removed" | "modified"
}

// InputChange describes an addition / removal / modification of an input.
type InputChange struct {
	Name   string         `json:"name"`
	Kind   string         `json:"kind"` // "added" | "removed" | "modified"
	Before map[string]any `json:"before,omitempty"`
	After  map[string]any `json:"after,omitempty"`
}

// SectionChange is the per-section body diff.
type SectionChange struct {
	Section    string  `json:"section"` // "system" | "user" | "assistant"
	Operations []EditOp `json:"operations"`
	Changed    bool    `json:"changed"` // true if any non-equal op
}

// EditOp is one step in the word-diff: equal / insert / delete.
type EditOp struct {
	Op   string `json:"op"`   // "equal" | "insert" | "delete"
	Text string `json:"text"`
}
