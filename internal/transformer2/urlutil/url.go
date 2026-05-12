package urlutil

import (
	"net/url"
	"strings"
)

// BuildURL constructs a full URL from a base URL and an endpoint path.
// It properly handles base URLs that already contain path components.
// For example:
//   - baseURL: "https://api.openai.com/v1", endpoint: "/chat/completions"
//     -> "https://api.openai.com/v1/chat/completions"
//   - baseURL: "https://api.openai.com", endpoint: "/v1/chat/completions"
//     -> "https://api.openai.com/v1/chat/completions"
func BuildURL(baseURL, endpoint string) (string, error) {
	baseURL = strings.TrimSuffix(baseURL, "/")

	// Parse the base URL
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}

	// Append the endpoint to the existing path
	parsedURL.Path = parsedURL.Path + endpoint

	return parsedURL.String(), nil
}
