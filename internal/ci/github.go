package ci

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// GitHub posts and updates issue comments via the REST API. Idempotent:
// looks for an existing comment with our marker and PATCHes it instead of
// creating duplicates.
type GitHub struct {
	Token  string // from $GITHUB_TOKEN
	Repo   string // "owner/repo"; from $GITHUB_REPOSITORY
	APIURL string // default https://api.github.com
	Client *http.Client
}

// NewGitHub builds a GitHub client with sane defaults.
func NewGitHub(token, repo string) *GitHub {
	return &GitHub{
		Token:  token,
		Repo:   repo,
		APIURL: "https://api.github.com",
		Client: &http.Client{Timeout: 30 * time.Second},
	}
}

// PostOrUpdateComment posts a comment on PR `pr`, or updates the existing
// comment if one with our marker exists.
func (g *GitHub) PostOrUpdateComment(ctx context.Context, pr, body string) (string, error) {
	if g.Token == "" {
		return "", fmt.Errorf("github: GITHUB_TOKEN is empty")
	}
	if g.Repo == "" {
		return "", fmt.Errorf("github: GITHUB_REPOSITORY is empty")
	}
	existing, err := g.findExistingComment(ctx, pr)
	if err != nil {
		return "", err
	}
	if existing != 0 {
		return g.updateComment(ctx, existing, body)
	}
	return g.createComment(ctx, pr, body)
}

type ghComment struct {
	ID   int64  `json:"id"`
	Body string `json:"body"`
}

func (g *GitHub) findExistingComment(ctx context.Context, pr string) (int64, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/issues/%s/comments?per_page=100",
		g.APIURL, g.Repo, pr)
	for endpoint != "" {
		req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
		if err != nil {
			return 0, err
		}
		g.setHeaders(req)
		resp, err := g.Client.Do(req)
		if err != nil {
			return 0, fmt.Errorf("github: list comments: %w", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 400 {
			return 0, fmt.Errorf("github: list comments %d: %s", resp.StatusCode, snippet(body))
		}
		var comments []ghComment
		if err := json.Unmarshal(body, &comments); err != nil {
			return 0, fmt.Errorf("github: decode comments: %w", err)
		}
		for _, c := range comments {
			if FoldExistingMarker(c.Body) {
				return c.ID, nil
			}
		}
		// Pagination via Link header (very lightweight parse).
		endpoint = nextLink(resp.Header.Get("Link"))
	}
	return 0, nil
}

func (g *GitHub) createComment(ctx context.Context, pr, body string) (string, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/issues/%s/comments", g.APIURL, g.Repo, pr)
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(MarshalCommentJSON(body)))
	if err != nil {
		return "", err
	}
	g.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("github: create comment: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("github: create %d: %s", resp.StatusCode, snippet(respBody))
	}
	var c ghComment
	_ = json.Unmarshal(respBody, &c)
	return fmt.Sprintf("created comment id=%d", c.ID), nil
}

func (g *GitHub) updateComment(ctx context.Context, id int64, body string) (string, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/issues/comments/%d", g.APIURL, g.Repo, id)
	req, err := http.NewRequestWithContext(ctx, "PATCH", endpoint, bytes.NewReader(MarshalCommentJSON(body)))
	if err != nil {
		return "", err
	}
	g.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("github: update comment: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("github: update %d: %s", resp.StatusCode, snippet(respBody))
	}
	return fmt.Sprintf("updated comment id=%d", id), nil
}

func (g *GitHub) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+g.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "promptlock")
}

// nextLink extracts the next-page URL from a Link header. Empty if none.
// Minimal parser: looks for `<URL>; rel="next"` segments.
func nextLink(header string) string {
	if header == "" {
		return ""
	}
	for _, part := range bytes.Split([]byte(header), []byte(",")) {
		seg := bytes.TrimSpace(part)
		// shape: <https://...>; rel="next"
		urlEnd := bytes.IndexByte(seg, '>')
		if urlEnd < 1 || seg[0] != '<' {
			continue
		}
		url := string(seg[1:urlEnd])
		if bytes.Contains(seg[urlEnd:], []byte(`rel="next"`)) {
			return url
		}
	}
	return ""
}

func snippet(b []byte) string {
	if len(b) > 500 {
		return string(b[:500]) + "..."
	}
	return string(b)
}
