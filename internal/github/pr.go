package github

import (
	"context"
	"fmt"
)

// PRInfo captures the GitHub PR fields downstream units (image build, Helm
// values, sticky comment) need.
type PRInfo struct {
	Number    int
	HeadSHA   string
	HeadRef   string
	BaseRef   string
	RepoOwner string
	RepoName  string
	Author    string
	IsDraft   bool
	Title     string
	URL       string
}

// ShortSHA returns the first 7 characters of HeadSHA, or HeadSHA itself if
// shorter.
func (p PRInfo) ShortSHA() string {
	if len(p.HeadSHA) <= 7 {
		return p.HeadSHA
	}
	return p.HeadSHA[:7]
}

// GetPR resolves the given PR. Owner/name/number are validated against the
// GitHub API; a 404 maps to ErrPRNotFound and 401/403 to ErrAuthFailed.
func (c *Client) GetPR(ctx context.Context, owner, name string, number int) (*PRInfo, error) {
	if owner == "" || name == "" {
		return nil, fmt.Errorf("repo owner/name required")
	}
	if number <= 0 {
		return nil, fmt.Errorf("PR number must be positive, got %d", number)
	}

	pr, _, err := c.gh.PullRequests.Get(ctx, owner, name, number)
	if err != nil {
		return nil, c.classifyHTTPError(err, fmt.Sprintf("%s/%s#%d", owner, name, number))
	}

	info := &PRInfo{
		Number:    pr.GetNumber(),
		HeadSHA:   pr.GetHead().GetSHA(),
		HeadRef:   pr.GetHead().GetRef(),
		BaseRef:   pr.GetBase().GetRef(),
		RepoOwner: owner,
		RepoName:  name,
		Author:    pr.GetUser().GetLogin(),
		IsDraft:   pr.GetDraft(),
		Title:     pr.GetTitle(),
		URL:       pr.GetHTMLURL(),
	}
	if info.HeadSHA == "" {
		return nil, fmt.Errorf("PR %d returned no head SHA (deleted branch?)", number)
	}
	return info, nil
}
