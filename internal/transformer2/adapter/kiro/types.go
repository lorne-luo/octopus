package kiro

import (
	"regexp"
	"strings"
)

// ── Kiro API Types ──────────────────────────────────────────────────────

// KiroRequest is the top-level Kiro API request payload.
type KiroRequest struct {
	ConversationState ConversationState `json:"conversationState"`
}

type ConversationState struct {
	ChatTriggerType string         `json:"chatTriggerType"`
	ConversationID  string         `json:"conversationId"`
	CurrentMessage  CurrentMessage `json:"currentMessage"`
	History         []interface{}  `json:"history,omitempty"`
}

type CurrentMessage struct {
	UserInputMessage UserInputMessage `json:"userInputMessage"`
}

type UserInputMessage struct {
	Content                 string                   `json:"content"`
	ModelID                 string                   `json:"modelId"`
	Origin                  string                   `json:"origin"`
	Images                  []map[string]interface{} `json:"images,omitempty"`
	UserInputMessageContext *MessageContext          `json:"userInputMessageContext,omitempty"`
}

type MessageContext struct {
	Tools       []map[string]interface{} `json:"tools,omitempty"`
	ToolResults []map[string]interface{} `json:"toolResults,omitempty"`
}

// ── Model Resolution ────────────────────────────────────────────────────

// modelAliases maps common model name aliases to canonical Kiro model IDs
var modelAliases = map[string]string{
	"claude-sonnet-4-5": "claude-sonnet-4.5",
	"claude-sonnet-4.5": "claude-sonnet-4.5",
	"claude-sonnet":     "claude-sonnet-4.5",
	"claude-3-5-sonnet": "claude-sonnet-4.5",
	"claude-3.5-sonnet": "claude-sonnet-4.5",
	"claude-opus-4":     "claude-opus-4",
	"claude-opus":       "claude-opus-4",
	"claude-3-opus":     "claude-opus-4",
	"claude-3-sonnet":   "claude-sonnet-4.5",
	"claude-3-haiku":    "claude-haiku-4",
	"claude-haiku-4":    "claude-haiku-4",
	"claude-haiku":      "claude-haiku-4",

	"anthropic.claude-sonnet-4-5-20250514-v1:0": "claude-sonnet-4.5",
	"anthropic.claude-3-5-sonnet-20241022-v2:0": "claude-sonnet-4.5",
	"anthropic.claude-3-sonnet-20240229-v1:0":   "claude-sonnet-4.5",
	"anthropic.claude-3-haiku-20240307-v1:0":    "claude-haiku-4",
	"anthropic.claude-3-opus-20240229-v1:0":     "claude-opus-4",
}

// NormalizeModelName normalizes client model name to Kiro format.
func NormalizeModelName(name string) string {
	if name == "" {
		return name
	}
	nameLower := strings.ToLower(name)

	// Pattern 1: claude-{family}-{major}-{minor}(-{suffix})?
	p1 := regexp.MustCompile(`^(claude-(?:haiku|sonnet|opus)-\d+)-(\d{1,2})(?:-(?:\d{8}|latest|\d+))?$`)
	if m := p1.FindStringSubmatch(nameLower); m != nil {
		return m[1] + "." + m[2]
	}

	// Pattern 2: claude-{family}-{major}(-{date})?
	p2 := regexp.MustCompile(`^(claude-(?:haiku|sonnet|opus)-\d+)(?:-\d{8})?$`)
	if m := p2.FindStringSubmatch(nameLower); m != nil {
		return m[1]
	}

	// Pattern 3: legacy claude-{major}-{minor}-{family}(-{suffix})?
	p3 := regexp.MustCompile(`^(claude)-(\d+)-(\d+)-(haiku|sonnet|opus)(?:-(?:\d{8}|latest|\d+))?$`)
	if m := p3.FindStringSubmatch(nameLower); m != nil {
		return m[1] + "-" + m[2] + "." + m[3] + "-" + m[4]
	}

	// Pattern 4: already normalized with dot but has date suffix
	p4 := regexp.MustCompile(`^(claude-(?:\d+\.\d+-)?(?:haiku|sonnet|opus)(?:-\d+\.\d+)?)-\d{8}$`)
	if m := p4.FindStringSubmatch(nameLower); m != nil {
		return m[1]
	}

	// Pattern 5: inverted format - claude-{major}.{minor}-{family}-{suffix}
	p5 := regexp.MustCompile(`^claude-(\d+)\.(\d+)-(haiku|sonnet|opus)-(.+)$`)
	if m := p5.FindStringSubmatch(nameLower); m != nil {
		return "claude-" + m[3] + "-" + m[1] + "." + m[2]
	}

	return name
}

// ResolveModel resolves a model name to its canonical Kiro model ID.
func ResolveModel(modelName string) string {
	if c, ok := modelAliases[modelName]; ok {
		return c
	}
	if c, ok := modelAliases[strings.ToLower(modelName)]; ok {
		return c
	}
	if strings.HasPrefix(modelName, "anthropic.claude") {
		for alias, canonical := range modelAliases {
			if strings.HasPrefix(modelName, alias) || strings.HasPrefix(modelName, strings.Split(alias, ":")[0]) {
				return canonical
			}
		}
	}
	return NormalizeModelName(modelName)
}

// SanitizeJSONSchema removes fields that Kiro API doesn't accept.
func SanitizeJSONSchema(schema map[string]interface{}) map[string]interface{} {
	if schema == nil {
		return make(map[string]interface{})
	}
	result := make(map[string]interface{})
	for key, value := range schema {
		if key == "required" {
			if arr, ok := value.([]interface{}); ok && len(arr) == 0 {
				continue
			}
		}
		if key == "additionalProperties" {
			continue
		}
		switch v := value.(type) {
		case map[string]interface{}:
			if key == "properties" {
				props := make(map[string]interface{})
				for pk, pv := range v {
					if pm, ok := pv.(map[string]interface{}); ok {
						props[pk] = SanitizeJSONSchema(pm)
					} else {
						props[pk] = pv
					}
				}
				result[key] = props
			} else {
				result[key] = SanitizeJSONSchema(v)
			}
		case []interface{}:
			var newArr []interface{}
			for _, item := range v {
				if im, ok := item.(map[string]interface{}); ok {
					newArr = append(newArr, SanitizeJSONSchema(im))
				} else {
					newArr = append(newArr, item)
				}
			}
			result[key] = newArr
		default:
			result[key] = value
		}
	}
	return result
}
