package test

import (
	"embed"
	"encoding/json"
	"fmt"

	"github.com/bestruirui/octopus/internal/transformer2/canonical"
)

//go:embed fixtures/*.json
var fixtureFS embed.FS

// TestCase represents a single test case from the JSON fixture.
type TestCase struct {
	Name        string `json:"name"`
	Description string `json:"description"`

	// Formats
	InboundFormat  string `json:"inbound_format"`
	OutboundFormat string `json:"outbound_format"`

	// Phase 1 – Inbound request transform
	ClientRequest         json.RawMessage `json:"client_request"`
	ExpectedInternalRequest json.RawMessage `json:"expected_internal_request"`

	// Phase 2 – Outbound request transform
	ExpectedProviderRequest json.RawMessage `json:"expected_provider_request"`

	// Phase 3 – Outbound response transform
	ProviderResponse     json.RawMessage `json:"provider_response"`
	ProviderStatusCode   int             `json:"provider_status_code"`
	ExpectedInternalResponse json.RawMessage `json:"expected_internal_response"`

	// Phase 4 – Inbound response transform
	ExpectedClientResponse json.RawMessage `json:"expected_client_response"`

	// Streaming
	Stream                     bool            `json:"stream"`
	ProviderStreamChunks       json.RawMessage `json:"provider_stream_chunks"`
	ExpectedClientStreamChunks json.RawMessage `json:"expected_client_stream_chunks"`
	ExpectedAggregatedResponse json.RawMessage `json:"expected_aggregated_response"`

	// Variants for parameterized tests
	Variants []Variant `json:"variants"`

	// Documentation fields (not used in assertions)
	Notes             string                   `json:"notes"`
	FinishReasonMappings []FinishReasonMapping `json:"finish_reason_mappings"`
}

// Variant represents a test variant.
type Variant struct {
	Label         string          `json:"label"`
	ClientRequest json.RawMessage `json:"client_request"`
	ExpectedStop  json.RawMessage `json:"expected_stop"`
}

// FinishReasonMapping represents finish reason mapping documentation.
type FinishReasonMapping struct {
	Anthropic string `json:"anthropic"`
	Internal  string `json:"internal"`
}

// LoadFixtures loads test cases from the embedded fixture files.
func LoadFixtures(filenames ...string) ([]TestCase, error) {
	var cases []TestCase

	for _, filename := range filenames {
		data, err := fixtureFS.ReadFile("fixtures/" + filename)
		if err != nil {
			return nil, fmt.Errorf("failed to read fixture %s: %w", filename, err)
		}

		var fileCases []TestCase
		if err := json.Unmarshal(data, &fileCases); err != nil {
			return nil, fmt.Errorf("failed to parse fixture %s: %w", filename, err)
		}

		cases = append(cases, fileCases...)
	}

	return cases, nil
}

// LoadAllFixtures loads all test fixtures from the fixtures directory.
func LoadAllFixtures() ([]TestCase, error) {
	files, err := fixtureFS.ReadDir("fixtures")
	if err != nil {
		return nil, fmt.Errorf("failed to read fixtures directory: %w", err)
	}

	var filenames []string
	for _, file := range files {
		if !file.IsDir() {
			filenames = append(filenames, file.Name())
		}
	}

	return LoadFixtures(filenames...)
}

// AssertFieldsMatch performs deep partial comparison between expected and actual.
// Only fields present in expected are checked; other fields in actual are ignored.
func AssertFieldsMatch(expected, actual interface{}) bool {
	e, ok1 := expected.(map[string]interface{})
	a, ok2 := actual.(map[string]interface{})

	if !ok1 || !ok2 {
		// For non-map types, do direct comparison
		return compareValues(expected, actual)
	}

	for key, expectedValue := range e {
		actualValue, exists := a[key]
		if !exists {
			return false
		}
		if !compareValues(expectedValue, actualValue) {
			return false
		}
	}

	return true
}

func compareValues(expected, actual interface{}) bool {
	switch e := expected.(type) {
	case map[string]interface{}:
		a, ok := actual.(map[string]interface{})
		if !ok {
			return false
		}
		return AssertFieldsMatch(e, a)

	case []interface{}:
		a, ok := actual.([]interface{})
		if !ok {
			return false
		}
		if len(e) != len(a) {
			return false
		}
		for i := range e {
			if !compareValues(e[i], a[i]) {
				return false
			}
		}
		return true

	default:
		return fmt.Sprintf("%v", expected) == fmt.Sprintf("%v", actual)
	}
}

// ParseInternalRequest parses the expected internal request assertion.
func ParseInternalRequest(data json.RawMessage) (*canonical.Request, error) {
	var req canonical.Request
	if len(data) == 0 {
		return nil, nil
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("failed to parse internal request: %w", err)
	}
	return &req, nil
}

// ParseInternalResponse parses the expected internal response assertion.
func ParseInternalResponse(data json.RawMessage) (*canonical.Response, error) {
	var resp canonical.Response
	if len(data) == 0 {
		return nil, nil
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse internal response: %w", err)
	}
	return &resp, nil
}