// Package git wraps the small subset of git commands promptlock needs.
// We shell out rather than depend on go-git: smaller binary, fewer surprises,
// and most of our users already have git on PATH (we're a git-workflow tool).
package git

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Runner runs git commands rooted at a working directory.
type Runner struct {
	WorkDir string
	gitBin  string // resolved at construction; "" until first call
}

// New returns a Runner rooted at workDir.
func New(workDir string) *Runner { return &Runner{WorkDir: workDir} }

// Available reports whether `git` is on PATH.
func Available() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

func (r *Runner) bin() (string, error) {
	if r.gitBin != "" {
		return r.gitBin, nil
	}
	p, err := exec.LookPath("git")
	if err != nil {
		return "", fmt.Errorf("git not found on PATH (promptlock requires git)")
	}
	r.gitBin = p
	return p, nil
}

// Run executes `git <args...>` and returns stdout. Stderr is folded into the
// returned error on non-zero exit.
func (r *Runner) Run(args ...string) ([]byte, error) {
	bin, err := r.bin()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = r.WorkDir
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		stderr := strings.TrimSpace(errb.String())
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, stderr)
	}
	return out.Bytes(), nil
}

// ShowFile returns the contents of `path` at git ref `ref`. Returns
// ErrPathNotInRef when the file does not exist at that ref.
func (r *Runner) ShowFile(ref, path string) ([]byte, error) {
	// Use forward slashes — git speaks them on every platform.
	gitPath := strings.ReplaceAll(path, "\\", "/")
	spec := ref + ":" + gitPath
	out, err := r.Run("show", spec)
	if err != nil {
		if isPathNotInRefErr(err) {
			return nil, &PathNotInRefError{Ref: ref, Path: gitPath}
		}
		return nil, err
	}
	return out, nil
}

// PathNotInRefError is returned when a file is missing at a given ref.
type PathNotInRefError struct {
	Ref  string
	Path string
}

func (e *PathNotInRefError) Error() string {
	return fmt.Sprintf("path %q not in ref %s", e.Path, e.Ref)
}

// IsPathNotInRef reports whether err indicates the file did not exist at the ref.
func IsPathNotInRef(err error) bool {
	var p *PathNotInRefError
	return errors.As(err, &p)
}

func isPathNotInRefErr(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "exists on disk, but not in") ||
		strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "fatal: path") ||
		strings.Contains(msg, "fatal: bad object") ||
		strings.Contains(msg, "Not a valid object name")
}

// LogForFile runs `git log --pretty=format:... -- path` and returns
// commits affecting that file.
func (r *Runner) LogForFile(path string, max int) ([]Commit, error) {
	gitPath := strings.ReplaceAll(path, "\\", "/")
	args := []string{"log", "--pretty=format:%H%x09%aI%x09%an%x09%s"}
	if max > 0 {
		args = append(args, fmt.Sprintf("-n%d", max))
	}
	args = append(args, "--", gitPath)
	out, err := r.Run(args...)
	if err != nil {
		return nil, err
	}
	var commits []Commit
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) != 4 {
			continue
		}
		commits = append(commits, Commit{
			SHA:        parts[0],
			AuthorDate: parts[1],
			Author:     parts[2],
			Subject:    parts[3],
		})
	}
	return commits, nil
}

// Commit is a minimal representation of a git commit for `promptlock log`.
type Commit struct {
	SHA        string
	AuthorDate string // ISO8601
	Author     string
	Subject    string
}

// IsRepo returns true if WorkDir is inside a git work tree.
func (r *Runner) IsRepo() bool {
	out, err := r.Run("rev-parse", "--is-inside-work-tree")
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

// Toplevel returns the absolute path of the git repo root containing WorkDir.
func (r *Runner) Toplevel() (string, error) {
	out, err := r.Run("rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Prefix returns the path from the repo root to WorkDir, including a trailing
// "/" when non-empty (matches git rev-parse --show-prefix).
func (r *Runner) Prefix() (string, error) {
	out, err := r.Run("rev-parse", "--show-prefix")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
