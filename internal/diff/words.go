package diff

import (
	"strings"
)

// tokenize splits text into atomic units for word-diffing.
// Whitespace runs collapse to a single space token. Template references
// `{{ var }}` are treated as a single atomic token (so a renamed variable
// shows as one delete + one insert, not many).
func tokenize(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	i := 0
	for i < len(s) {
		// Whitespace run
		if isASCIISpace(s[i]) {
			j := i
			for j < len(s) && isASCIISpace(s[j]) {
				j++
			}
			out = append(out, " ")
			i = j
			continue
		}
		// Template var
		if i+1 < len(s) && s[i] == '{' && s[i+1] == '{' {
			if end := findClose(s, i); end > 0 {
				out = append(out, s[i:end])
				i = end
				continue
			}
		}
		// Word: anything until whitespace or `{{`
		j := i
		for j < len(s) {
			if isASCIISpace(s[j]) {
				break
			}
			if j+1 < len(s) && s[j] == '{' && s[j+1] == '{' {
				break
			}
			j++
		}
		out = append(out, s[i:j])
		i = j
	}
	return out
}

func findClose(s string, start int) int {
	end := strings.Index(s[start:], "}}")
	if end < 0 {
		return -1
	}
	return start + end + 2
}

func isASCIISpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// diffTokens computes an LCS-based edit script over two token sequences and
// emits coalesced EditOps (equal / insert / delete). O(n*m) memory; bounded
// by typical prompt size (a few thousand tokens at most).
func diffTokens(a, b []string) []EditOp {
	n, m := len(a), len(b)
	// Build LCS length table.
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}
	// Walk back, emit reverse ops.
	var rev []EditOp
	i, j := n, m
	for i > 0 || j > 0 {
		switch {
		case i > 0 && j > 0 && a[i-1] == b[j-1]:
			rev = append(rev, EditOp{Op: "equal", Text: a[i-1]})
			i--
			j--
		case j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]):
			rev = append(rev, EditOp{Op: "insert", Text: b[j-1]})
			j--
		default:
			rev = append(rev, EditOp{Op: "delete", Text: a[i-1]})
			i--
		}
	}
	// Reverse + coalesce adjacent ops of the same kind.
	out := make([]EditOp, 0, len(rev))
	for k := len(rev) - 1; k >= 0; k-- {
		op := rev[k]
		if len(out) > 0 && out[len(out)-1].Op == op.Op {
			out[len(out)-1].Text += op.Text
		} else {
			out = append(out, op)
		}
	}
	return out
}

// hasNonEqualOps reports whether ops contain any insert or delete.
func hasNonEqualOps(ops []EditOp) bool {
	for _, op := range ops {
		if op.Op != "equal" {
			return true
		}
	}
	return false
}

