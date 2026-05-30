package githubrelease

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.github.com"
const latestReleasePath = "/repos/bnema/tmux-session-sidebar/releases/latest"

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	Timeout    time.Duration
}

func (c Client) LatestReleaseTag(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.baseURL(), "/")+latestReleasePath, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "tmux-session-sidebar")

	response, err := c.httpClient().Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github latest release status: %s", response.Status)
	}

	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return "", err
	}
	if strings.TrimSpace(payload.TagName) == "" {
		return "", fmt.Errorf("github latest release missing tag_name")
	}
	return strings.TrimSpace(payload.TagName), nil
}

func (c Client) baseURL() string {
	if strings.TrimSpace(c.BaseURL) == "" {
		return defaultBaseURL
	}
	return c.BaseURL
}

func (c Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

func (c Client) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return 2 * time.Second
}
