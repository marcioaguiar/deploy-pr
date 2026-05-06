// Package github wraps go-github with the small set of operations deploy-pr
// needs: resolve a PR number into actionable metadata, and post or update a
// sticky preview-URL comment (U6).
package github

import (
	"errors"
	"fmt"
	"net/http"

	gh "github.com/google/go-github/v68/github"
)

// Client is deploy-pr's typed wrapper around go-github. It carries the token
// source so auth-failure errors can name the source the user supplied.
type Client struct {
	gh     *gh.Client
	source TokenSource
}

// NewClient builds a Client using the given token. The token source is stored
// only for diagnostic messages; it does not affect API behavior.
func NewClient(token string, source TokenSource) *Client {
	c := gh.NewClient(nil).WithAuthToken(token)
	return &Client{gh: c, source: source}
}

// newClientWithBaseURL is used by tests to point the client at an httptest.Server.
func newClientWithBaseURL(token string, source TokenSource, baseURL string) (*Client, error) {
	c := gh.NewClient(nil).WithAuthToken(token)
	parsed, err := c.WithEnterpriseURLs(baseURL, baseURL)
	if err != nil {
		return nil, err
	}
	return &Client{gh: parsed, source: source}, nil
}

// classifyHTTPError turns a go-github error into a deploy-pr typed error
// when the underlying HTTP status maps cleanly to one of our categories.
func (c *Client) classifyHTTPError(err error, ownerNameNumber string) error {
	if err == nil {
		return nil
	}
	var ghErr *gh.ErrorResponse
	if errors.As(err, &ghErr) && ghErr.Response != nil {
		switch ghErr.Response.StatusCode {
		case http.StatusNotFound:
			return fmt.Errorf("%s: %w", ownerNameNumber, ErrPRNotFound)
		case http.StatusUnauthorized, http.StatusForbidden:
			return fmt.Errorf("github auth rejected token from %s (%s): %w",
				c.source, ghErr.Message, ErrAuthFailed)
		}
	}
	return err
}

// Sentinel errors callers can use with errors.Is.
var (
	ErrPRNotFound = errors.New("PR not found")
	ErrAuthFailed = errors.New("github authentication failed")
)
