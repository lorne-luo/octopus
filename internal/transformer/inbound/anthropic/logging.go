package anthropic

import (
	"encoding/json"

	"github.com/bestruirui/octopus/internal/utils/log"
)

func logAnthropicRequest(req *MessageRequest) {
	// Log the request
	reqBytes, err := json.Marshal(req)
	if err != nil {
		log.Errorf("Failed to marshal Anthropic request for logging: %v", err)
		return
	}
	log.Infof("[Anthropic Inbound Request] Model: %s, Body: %s", req.Model, string(reqBytes))
}

func logAnthropicResponse(resp *Message) {
	// Log the response
	respBytes, err := json.Marshal(resp)
	if err != nil {
		log.Errorf("Failed to marshal Anthropic response for logging: %v", err)
		return
	}
	log.Infof("[Anthropic Inbound Response] ID: %s, Body: %s", resp.ID, string(respBytes))
}

func logAnthropicStreamEvent(eventName string, event *StreamEvent) {
	eventBytes, err := json.Marshal(event)
	if err != nil {
		log.Errorf("Failed to marshal Anthropic stream event for logging: %v", err)
		return
	}
	log.Infof("[Anthropic Inbound Stream] Event: %s, Type: %s, Body: %s", eventName, event.Type, string(eventBytes))
}
