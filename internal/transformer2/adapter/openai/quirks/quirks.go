package quirks

import (
	"encoding/json"
	"strings"
)

// Interceptor modifies serialized request bodies to strip unsupported fields
// for specific OpenAI-compatible providers.
type Interceptor interface {
	// ModifyRequestBody removes unsupported fields from the serialized JSON body.
	ModifyRequestBody(body []byte) ([]byte, error)
}

// registry maps base URL patterns to their interceptor.
var registry = map[string]Interceptor{}

// Register registers an interceptor for a URL pattern.
func Register(pattern string, interceptor Interceptor) {
	registry[pattern] = interceptor
}

// Get returns the interceptor for the given base URL, or nil if none matches.
func Get(baseURL string) Interceptor {
	lowerURL := strings.ToLower(baseURL)
	for pattern, interceptor := range registry {
		if strings.Contains(lowerURL, pattern) {
			return interceptor
		}
	}
	return nil
}

// ApplyQuirks applies provider-specific quirks to a serialized request body.
// Returns the original body unchanged if no quirk matches the base URL.
func ApplyQuirks(body []byte, baseURL string) ([]byte, error) {
	interceptor := Get(baseURL)
	if interceptor == nil {
		return body, nil
	}
	return interceptor.ModifyRequestBody(body)
}

// stripFields removes specified keys from a JSON object body.
func stripFields(body []byte, fields ...string) ([]byte, error) {
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return body, nil // non-object, return as-is
	}

	for _, f := range fields {
		delete(m, f)
	}

	return json.Marshal(m)
}
