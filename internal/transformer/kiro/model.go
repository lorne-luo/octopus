package kiro

import (
	"regexp"
	"strings"
)

// ModelAlias defines a mapping from an alias to a canonical model ID
type ModelAlias struct {
	Alias     string
	Canonical string
}

// modelAliases maps common model name aliases to their canonical Kiro model IDs
var modelAliases = map[string]string{
	// Claude model aliases
	"claude-sonnet-4-5":    "claude-sonnet-4.5",
	"claude-sonnet-4.5":    "claude-sonnet-4.5",
	"claude-sonnet":        "claude-sonnet-4.5",
	"claude-3-5-sonnet":    "claude-sonnet-4.5",
	"claude-3.5-sonnet":    "claude-sonnet-4.5",
	"claude-opus-4":        "claude-opus-4",
	"claude-opus":          "claude-opus-4",
	"claude-3-opus":        "claude-opus-4",
	"claude-3-sonnet":      "claude-sonnet-4.5",
	"claude-3-haiku":       "claude-haiku-4",
	"claude-haiku-4":       "claude-haiku-4",
	"claude-haiku":         "claude-haiku-4",

	// Bedrock-style model IDs
	"anthropic.claude-sonnet-4-5-20250514-v1:0": "claude-sonnet-4.5",
	"anthropic.claude-3-5-sonnet-20241022-v2:0": "claude-sonnet-4.5",
	"anthropic.claude-3-sonnet-20240229-v1:0":   "claude-sonnet-4.5",
	"anthropic.claude-3-haiku-20240307-v1:0":    "claude-haiku-4",
	"anthropic.claude-3-opus-20240229-v1:0":     "claude-opus-4",
}

// NormalizeModelName normalizes client model name to Kiro format
// This implements the same logic as kiro-go-proxy
func NormalizeModelName(name string) string {
	if name == "" {
		return name
	}

	nameLower := strings.ToLower(name)

	// Pattern 1: Standard format - claude-{family}-{major}-{minor}(-{suffix})?
	// e.g., claude-haiku-4-5, claude-haiku-4-5-20251001
	standardPattern := regexp.MustCompile(`^(claude-(?:haiku|sonnet|opus)-\d+)-(\d{1,2})(?:-(?:\d{8}|latest|\d+))?$`)
	if match := standardPattern.FindStringSubmatch(nameLower); match != nil {
		return match[1] + "." + match[2] // claude-haiku-4.5
	}

	// Pattern 2: Standard format without minor - claude-{family}-{major}(-{date})?
	// e.g., claude-sonnet-4, claude-sonnet-4-20250514
	noMinorPattern := regexp.MustCompile(`^(claude-(?:haiku|sonnet|opus)-\d+)(?:-\d{8})?$`)
	if match := noMinorPattern.FindStringSubmatch(nameLower); match != nil {
		return match[1]
	}

	// Pattern 3: Legacy format - claude-{major}-{minor}-{family}(-{suffix})?
	// e.g., claude-3-7-sonnet, claude-3-7-sonnet-20250219
	legacyPattern := regexp.MustCompile(`^(claude)-(\d+)-(\d+)-(haiku|sonnet|opus)(?:-(?:\d{8}|latest|\d+))?$`)
	if match := legacyPattern.FindStringSubmatch(nameLower); match != nil {
		return match[1] + "-" + match[2] + "." + match[3] + "-" + match[4] // claude-3.7-sonnet
	}

	// Pattern 4: Already normalized with dot but has date suffix
	// e.g., claude-haiku-4.5-20251001
	dotWithDatePattern := regexp.MustCompile(`^(claude-(?:\d+\.\d+-)?(?:haiku|sonnet|opus)(?:-\d+\.\d+)?)-\d{8}$`)
	if match := dotWithDatePattern.FindStringSubmatch(nameLower); match != nil {
		return match[1]
	}

	// Pattern 5: Inverted format with suffix - claude-{major}.{minor}-{family}-{suffix}
	// e.g., claude-4.5-opus-high
	invertedPattern := regexp.MustCompile(`^claude-(\d+)\.(\d+)-(haiku|sonnet|opus)-(.+)$`)
	if match := invertedPattern.FindStringSubmatch(nameLower); match != nil {
		return "claude-" + match[3] + "-" + match[1] + "." + match[2] // claude-opus-4.5
	}

	return name
}

// ResolveModel resolves a model name to its canonical Kiro model ID
// If the model is not found in the aliases, it returns the normalized name
func ResolveModel(modelName string) string {
	// Check for exact match first
	if canonical, ok := modelAliases[modelName]; ok {
		return canonical
	}

	// Try lowercase match
	lowerName := strings.ToLower(modelName)
	if canonical, ok := modelAliases[lowerName]; ok {
		return canonical
	}

	// Check for Bedrock-style ARN patterns
	if strings.HasPrefix(modelName, "anthropic.claude") {
		for alias, canonical := range modelAliases {
			if strings.HasPrefix(modelName, alias) || strings.HasPrefix(modelName, strings.Split(alias, ":")[0]) {
				return canonical
			}
		}
	}

	// Apply normalization patterns from kiro-go-proxy
	return NormalizeModelName(modelName)
}

// IsClaudeModel checks if the model is a Claude model
func IsClaudeModel(modelName string) bool {
	lower := strings.ToLower(modelName)
	return strings.Contains(lower, "claude") || strings.Contains(lower, "anthropic")
}

// GetModelFamily returns the model family for a given model name
func GetModelFamily(modelName string) string {
	canonical := ResolveModel(modelName)

	switch {
	case strings.Contains(canonical, "opus"):
		return "opus"
	case strings.Contains(canonical, "sonnet"):
		return "sonnet"
	case strings.Contains(canonical, "haiku"):
		return "haiku"
	default:
		return "unknown"
	}
}
