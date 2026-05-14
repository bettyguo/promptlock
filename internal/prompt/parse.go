package prompt

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseError is returned when a file is malformed in a way the parser can locate.
type ParseError struct {
	Path string
	Line int // 1-based; 0 if unknown
	Msg  string
}

func (e *ParseError) Error() string {
	if e.Path == "" {
		if e.Line > 0 {
			return fmt.Sprintf("parse error: line %d: %s", e.Line, e.Msg)
		}
		return "parse error: " + e.Msg
	}
	if e.Line > 0 {
		return fmt.Sprintf("%s: line %d: %s", e.Path, e.Line, e.Msg)
	}
	return fmt.Sprintf("%s: %s", e.Path, e.Msg)
}

var (
	frontmatterDelim = []byte("---")

	// utf8BOM is the 3-byte UTF-8 byte-order mark we strip if present at file start.
	utf8BOM = []byte{0xEF, 0xBB, 0xBF}
)

// known keys we map into the typed Frontmatter struct. Anything else lands in Extra.
var knownFrontmatterKeys = map[string]struct{}{
	"id": {}, "version": {}, "description": {}, "model": {},
	"temperature": {}, "max_tokens": {}, "top_p": {}, "stop": {},
	"tags": {}, "inputs": {}, "outputs": {}, "evals": {}, "metadata": {},
}

// Parse parses prompt file bytes. path is recorded on the returned Prompt and
// in error messages; pass "" for in-memory data.
func Parse(path string, data []byte) (*Prompt, error) {
	fmYAML, body, err := splitFrontmatter(path, data)
	if err != nil {
		return nil, err
	}

	// Parse into a generic node first so we can capture unknown keys, then
	// decode into the typed struct.
	var raw map[string]any
	if err := yaml.Unmarshal(fmYAML, &raw); err != nil {
		return nil, &ParseError{Path: path, Msg: "invalid YAML frontmatter: " + err.Error()}
	}
	if raw == nil {
		raw = map[string]any{}
	}

	var fm Frontmatter
	if err := yaml.Unmarshal(fmYAML, &fm); err != nil {
		return nil, &ParseError{Path: path, Msg: "frontmatter does not match expected shape: " + err.Error()}
	}

	// Capture unknown keys.
	extra := map[string]any{}
	for k, v := range raw {
		if _, known := knownFrontmatterKeys[k]; !known {
			extra[k] = v
		}
	}
	if len(extra) > 0 {
		fm.Extra = extra
	}

	parsedBody, err := parseBody(path, body)
	if err != nil {
		return nil, err
	}

	return &Prompt{
		Path:        path,
		Frontmatter: fm,
		Body:        parsedBody,
		Raw:         data,
	}, nil
}

// splitFrontmatter extracts the YAML between the opening `---` and the next `---`.
// It returns frontmatter bytes (without delimiters) and body bytes (everything
// after the closing `---` line).
func splitFrontmatter(path string, data []byte) (frontmatter, body []byte, err error) {
	// Strip UTF-8 BOM if present.
	if bytes.HasPrefix(data, utf8BOM) {
		data = data[len(utf8BOM):]
	}
	// Skip leading blank lines.
	rest := data
	for len(rest) > 0 {
		end := bytes.IndexByte(rest, '\n')
		if end < 0 {
			break
		}
		line := bytes.TrimRight(rest[:end], " \t\r")
		if len(line) > 0 {
			break
		}
		rest = rest[end+1:]
	}
	if !bytes.HasPrefix(rest, frontmatterDelim) {
		return nil, nil, &ParseError{Path: path, Line: 1, Msg: "file must begin with a `---` frontmatter delimiter"}
	}
	openLineEnd := bytes.IndexByte(rest, '\n')
	if openLineEnd < 0 {
		return nil, nil, &ParseError{Path: path, Line: 1, Msg: "frontmatter never closes"}
	}
	if trimmed := bytes.TrimSpace(rest[len(frontmatterDelim):openLineEnd]); len(trimmed) > 0 {
		return nil, nil, &ParseError{Path: path, Line: 1, Msg: "unexpected content after opening `---`"}
	}

	yamlStart := openLineEnd + 1
	closeIdx := -1
	cursor := yamlStart
	for cursor < len(rest) {
		lineEnd := bytes.IndexByte(rest[cursor:], '\n')
		var line []byte
		if lineEnd < 0 {
			line = rest[cursor:]
		} else {
			line = rest[cursor : cursor+lineEnd]
		}
		if bytes.Equal(bytes.TrimRight(line, " \t\r"), frontmatterDelim) {
			closeIdx = cursor
			break
		}
		if lineEnd < 0 {
			break
		}
		cursor += lineEnd + 1
	}
	if closeIdx < 0 {
		return nil, nil, &ParseError{Path: path, Line: 1, Msg: "frontmatter never closes (no second `---` found)"}
	}
	frontmatter = rest[yamlStart:closeIdx]
	bodyStart := closeIdx + len(frontmatterDelim)
	if nl := bytes.IndexByte(rest[bodyStart:], '\n'); nl >= 0 {
		bodyStart += nl + 1
	} else {
		bodyStart = len(rest)
	}
	body = rest[bodyStart:]
	return frontmatter, body, nil
}

// sectionHeading matches a top-level # heading whose text (trimmed,
// case-insensitive) is "system", "user", or "assistant".
var sectionHeading = regexp.MustCompile(`(?im)^#[ \t]+(system|user|assistant)[ \t]*$`)

func parseBody(path string, body []byte) (Body, error) {
	out := Body{}
	indices := sectionHeading.FindAllSubmatchIndex(body, -1)
	if len(indices) == 0 {
		text := strings.TrimSpace(string(body))
		out.User = text
		return out, nil
	}
	type seg struct {
		role  string
		start int
		end   int
	}
	segs := make([]seg, 0, len(indices))
	for i, m := range indices {
		role := strings.ToLower(string(body[m[2]:m[3]]))
		nl := bytes.IndexByte(body[m[1]:], '\n')
		var startContent int
		if nl < 0 {
			startContent = len(body)
		} else {
			startContent = m[1] + nl + 1
		}
		end := len(body)
		if i+1 < len(indices) {
			end = indices[i+1][0]
		}
		segs = append(segs, seg{role: role, start: startContent, end: end})
	}
	for _, s := range segs {
		text := strings.TrimSpace(string(body[s.start:s.end]))
		switch s.role {
		case "system":
			if out.System != "" {
				return Body{}, &ParseError{Path: path, Msg: "multiple `# System` sections"}
			}
			out.System = text
		case "user":
			if out.User != "" {
				return Body{}, &ParseError{Path: path, Msg: "multiple `# User` sections"}
			}
			out.User = text
		case "assistant":
			if out.Assistant != "" {
				return Body{}, &ParseError{Path: path, Msg: "multiple `# Assistant` sections"}
			}
			out.Assistant = text
		}
	}
	return out, nil
}

// VarRefs returns all unique {{ identifier }} references in the body, in order
// of first appearance.
func (p *Prompt) VarRefs() []string {
	seen := map[string]struct{}{}
	var out []string
	for _, sect := range []string{p.Body.System, p.Body.User, p.Body.Assistant} {
		for _, m := range varRef.FindAllStringSubmatch(sect, -1) {
			id := strings.TrimSpace(m[1])
			if i := strings.IndexAny(id, " |."); i > 0 {
				id = id[:i]
			}
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	return out
}

var varRef = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*(?:\s*[|.]\s*[A-Za-z_][A-Za-z0-9_]*)*)\s*\}\}`)
