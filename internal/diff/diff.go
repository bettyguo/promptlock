package diff

import (
	"github.com/promptlock/promptlock/internal/prompt"
)

// Diff computes a semantic diff between two parsed prompts. Either side may be
// nil to indicate a new file (before==nil) or deleted file (after==nil).
func Diff(before, after *prompt.Prompt) *Result {
	r := &Result{}
	switch {
	case before == nil && after == nil:
		return r
	case before == nil:
		r.PromptID = after.Frontmatter.ID
		r.BeforeMissing = true
		r.HasChanges = true
		// Show the after as a series of "insert" ops on each section.
		r.BodyChanges = []SectionChange{
			{Section: "system", Operations: insertOps(after.Body.System), Changed: after.Body.System != ""},
			{Section: "user", Operations: insertOps(after.Body.User), Changed: after.Body.User != ""},
			{Section: "assistant", Operations: insertOps(after.Body.Assistant), Changed: after.Body.Assistant != ""},
		}
		r.VersionChange = &VersionChange{Before: "", After: after.Frontmatter.Version}
		r.InputsChanges = diffInputs(nil, after.Frontmatter.Inputs)
		return r
	case after == nil:
		r.PromptID = before.Frontmatter.ID
		r.AfterMissing = true
		r.HasChanges = true
		r.BodyChanges = []SectionChange{
			{Section: "system", Operations: deleteOps(before.Body.System), Changed: before.Body.System != ""},
			{Section: "user", Operations: deleteOps(before.Body.User), Changed: before.Body.User != ""},
			{Section: "assistant", Operations: deleteOps(before.Body.Assistant), Changed: before.Body.Assistant != ""},
		}
		r.VersionChange = &VersionChange{Before: before.Frontmatter.Version, After: ""}
		r.InputsChanges = diffInputs(before.Frontmatter.Inputs, nil)
		return r
	}

	r.PromptID = after.Frontmatter.ID
	if before.Frontmatter.Version != after.Frontmatter.Version {
		r.VersionChange = &VersionChange{Before: before.Frontmatter.Version, After: after.Frontmatter.Version}
	}
	r.FrontmatterChanges = diffFrontmatter(before.Frontmatter, after.Frontmatter)
	r.InputsChanges = diffInputs(before.Frontmatter.Inputs, after.Frontmatter.Inputs)
	r.BodyChanges = []SectionChange{
		diffSection("system", before.Body.System, after.Body.System),
		diffSection("user", before.Body.User, after.Body.User),
		diffSection("assistant", before.Body.Assistant, after.Body.Assistant),
	}
	r.HasChanges = r.VersionChange != nil ||
		len(r.FrontmatterChanges) > 0 ||
		len(r.InputsChanges) > 0 ||
		anySectionChanged(r.BodyChanges)
	return r
}

func diffSection(name, before, after string) SectionChange {
	if before == after {
		return SectionChange{Section: name, Operations: nil, Changed: false}
	}
	ops := diffTokens(tokenize(before), tokenize(after))
	return SectionChange{Section: name, Operations: ops, Changed: hasNonEqualOps(ops)}
}

func anySectionChanged(secs []SectionChange) bool {
	for _, s := range secs {
		if s.Changed {
			return true
		}
	}
	return false
}

func insertOps(s string) []EditOp {
	if s == "" {
		return nil
	}
	return []EditOp{{Op: "insert", Text: s}}
}

func deleteOps(s string) []EditOp {
	if s == "" {
		return nil
	}
	return []EditOp{{Op: "delete", Text: s}}
}
