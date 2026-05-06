package github

import (
	"context"
	"fmt"
	"strings"
	"time"

	gh "github.com/google/go-github/v68/github"
)

// stickyMarker is the first line of every deploy-pr comment. Looking for it
// in the comment body lets us update one comment per PR instead of spamming
// new comments on every re-deploy.
const stickyMarker = "<!-- deploy-pr:preview -->"

// CommentStatus distinguishes a "preview ready" body from a "torn down" body.
type CommentStatus string

const (
	StatusReady     CommentStatus = "ready"
	StatusTornDown  CommentStatus = "torn-down"
)

// CommentInput is the data the sticky comment renders from. Time is taken as
// a parameter (not time.Now()) so callers can stamp deterministic values in
// tests and so an audit run can re-render the same comment idempotently.
type CommentInput struct {
	Status    CommentStatus
	URL       string
	ImageTag  string
	Timestamp time.Time
}

// RenderBody produces the markdown body, marker included.
func (in CommentInput) RenderBody() string {
	var sb strings.Builder
	sb.WriteString(stickyMarker)
	sb.WriteString("\n\n")
	ts := in.Timestamp.UTC().Format(time.RFC3339)
	switch in.Status {
	case StatusTornDown:
		sb.WriteString("**Preview torn down**\n\n")
		if in.URL != "" {
			fmt.Fprintf(&sb, "~~Preview was at %s~~\n\n", in.URL)
		}
		fmt.Fprintf(&sb, "_Torn down at %s_\n", ts)
	default:
		sb.WriteString("**Preview ready**\n\n")
		if in.URL != "" {
			fmt.Fprintf(&sb, "- URL: %s\n", in.URL)
		}
		if in.ImageTag != "" {
			fmt.Fprintf(&sb, "- Image: `%s`\n", in.ImageTag)
		}
		fmt.Fprintf(&sb, "- Last deployed: %s\n", ts)
	}
	return sb.String()
}

// UpsertStickyComment finds the existing deploy-pr comment (if any) on the
// PR and patches it; otherwise it creates a new one. Returns the comment ID
// touched.
func (c *Client) UpsertStickyComment(ctx context.Context, owner, name string, prNumber int, body string) (int64, error) {
	existing, err := c.findStickyComment(ctx, owner, name, prNumber)
	if err != nil {
		return 0, err
	}

	if existing == nil {
		created, _, err := c.gh.Issues.CreateComment(ctx, owner, name, prNumber, &gh.IssueComment{Body: &body})
		if err != nil {
			return 0, c.classifyHTTPError(err, fmt.Sprintf("%s/%s#%d comment", owner, name, prNumber))
		}
		return created.GetID(), nil
	}

	updated, _, err := c.gh.Issues.EditComment(ctx, owner, name, existing.GetID(), &gh.IssueComment{Body: &body})
	if err != nil {
		return 0, c.classifyHTTPError(err, fmt.Sprintf("%s/%s comment %d", owner, name, existing.GetID()))
	}
	return updated.GetID(), nil
}

// findStickyComment returns the first comment on the PR whose body begins
// with stickyMarker, or nil if none exists. It paginates because PRs with
// hundreds of comments are not unusual on long-lived branches.
func (c *Client) findStickyComment(ctx context.Context, owner, name string, prNumber int) (*gh.IssueComment, error) {
	opts := &gh.IssueListCommentsOptions{
		ListOptions: gh.ListOptions{PerPage: 100},
	}
	for {
		comments, resp, err := c.gh.Issues.ListComments(ctx, owner, name, prNumber, opts)
		if err != nil {
			return nil, c.classifyHTTPError(err, fmt.Sprintf("%s/%s#%d comments", owner, name, prNumber))
		}
		for _, cmt := range comments {
			if strings.HasPrefix(strings.TrimLeft(cmt.GetBody(), " \t\r\n"), stickyMarker) {
				return cmt, nil
			}
		}
		if resp == nil || resp.NextPage == 0 {
			return nil, nil
		}
		opts.Page = resp.NextPage
	}
}
