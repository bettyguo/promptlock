// Package lock manages promptlock.lock: per-prompt content hash and last-eval
// scores, used by `promptlock check` to detect unreviewed edits.
package lock

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// HashFile returns the SHA256 of the normalized prompt file as "sha256:<hex>".
// Normalization: LF line endings, trailing whitespace stripped per line,
// trailing blank lines collapsed, frontmatter YAML re-serialized with sorted
// keys. Whitespace-only and key-reorder edits don't change the hash.
func HashFile(data []byte) (string, error) {
	normalized, err := Normalize(data)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(normalized)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// HashDataset returns the SHA256 of a dataset file (no normalization beyond
// CRLF→LF). Datasets are user-provided and we keep them byte-identical to disk.
func HashDataset(data []byte) string {
	normalized := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	sum := sha256.Sum256(normalized)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Normalize returns the normalized form of a prompt file (used by HashFile and
// exported so tests can verify intermediate state).
func Normalize(data []byte) ([]byte, error) {
	// 0. Strip UTF-8 BOM if present. (PowerShell 5.x's Set-Content -Encoding
	// utf8 adds one; we don't want hashes to flip just because the file got
	// round-tripped through an editor that added a BOM.)
	if bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		data = data[3:]
	}
	// 1. Line endings.
	s := strings.ReplaceAll(string(data), "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	// Split frontmatter / body.
	const delim = "---"
	if !strings.HasPrefix(strings.TrimLeft(s, "\n"), delim) {
		// Permissive: if no frontmatter, treat whole file as body.
		return []byte(stripTrailingBlanks(stripTrailingWhitespacePerLine(s)) + "\n"), nil
	}
	// Skip leading blank lines.
	for strings.HasPrefix(s, "\n") {
		s = s[1:]
	}
	if !strings.HasPrefix(s, delim) {
		return nil, fmt.Errorf("normalize: missing opening `---`")
	}
	// Find first newline after opening delim.
	nl := strings.IndexByte(s, '\n')
	if nl < 0 {
		return nil, fmt.Errorf("normalize: malformed frontmatter")
	}
	rest := s[nl+1:]
	closeIdx := strings.Index(rest, "\n"+delim)
	if closeIdx < 0 {
		return nil, fmt.Errorf("normalize: frontmatter never closes")
	}
	yamlBlock := rest[:closeIdx]
	bodyStart := closeIdx + 1 + len(delim)
	if bodyStart < len(rest) && rest[bodyStart] == '\n' {
		bodyStart++
	}
	body := rest[bodyStart:]

	// 2. Per-line: strip trailing whitespace.
	yamlBlock = stripTrailingWhitespacePerLine(yamlBlock)
	body = stripTrailingWhitespacePerLine(body)

	// 4. Re-serialize YAML with sorted keys.
	yamlBlock, err := canonicalizeYAML(yamlBlock)
	if err != nil {
		return nil, fmt.Errorf("normalize: %w", err)
	}

	// 3. Trailing blank lines on body collapse to single "\n".
	body = stripTrailingBlanks(body)

	var out bytes.Buffer
	out.WriteString(delim)
	out.WriteByte('\n')
	out.WriteString(yamlBlock)
	if !strings.HasSuffix(yamlBlock, "\n") {
		out.WriteByte('\n')
	}
	out.WriteString(delim)
	out.WriteByte('\n')
	if body != "" {
		// Single blank line between frontmatter and body if there's any body.
		out.WriteByte('\n')
		out.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			out.WriteByte('\n')
		}
	}
	return out.Bytes(), nil
}

// canonicalizeYAML re-emits a YAML mapping. yaml.v3 marshals string-keyed maps
// in sorted-key order by default, which is exactly the canonicalization
// docs/format.md § Hashing requires. We round-trip through `any` so the result
// is deterministic regardless of input key order.
func canonicalizeYAML(in string) (string, error) {
	var node any
	if err := yaml.Unmarshal([]byte(in), &node); err != nil {
		return "", fmt.Errorf("yaml unmarshal: %w", err)
	}
	out, err := yaml.Marshal(node)
	if err != nil {
		return "", fmt.Errorf("yaml marshal: %w", err)
	}
	return string(out), nil
}

func stripTrailingWhitespacePerLine(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}

func stripTrailingBlanks(s string) string {
	for strings.HasSuffix(s, "\n\n") {
		s = s[:len(s)-1]
	}
	return strings.TrimRight(s, "\n")
}
