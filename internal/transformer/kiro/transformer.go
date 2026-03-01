package kiro

import (
	"context"
	"net/http"
	"net/url"

	"github.com/bestruirui/octopus/internal/transformer/model"
)

// Outbound implements the model.Outbound interface for Kiro
type Outbound struct {
	request  *RequestOutbound
	response *ResponseOutbound
}

// NewOutbound creates a new Kiro outbound transformer
func NewOutbound() *Outbound {
	return &Outbound{
		request:  NewRequestOutbound(),
		response: NewResponseOutbound(),
	}
}

// TransformRequest transforms an internal request to Kiro format
func (o *Outbound) TransformRequest(ctx context.Context, request *model.InternalLLMRequest, baseUrl, key string) (*http.Request, error) {
	return o.request.TransformRequest(ctx, request, baseUrl, key)
}

// TransformResponse transforms a Kiro HTTP response to internal format
func (o *Outbound) TransformResponse(ctx context.Context, response *http.Response) (*model.InternalLLMResponse, error) {
	return o.response.TransformResponse(ctx, response)
}

// TransformStream transforms a Kiro streaming event to internal format
func (o *Outbound) TransformStream(ctx context.Context, eventData []byte) (*model.InternalLLMResponse, error) {
	return o.response.TransformStream(ctx, eventData)
}

// SetProfileArn sets the profile ARN for Kiro requests
func (o *Outbound) SetProfileArn(arn string) {
	o.request.SetProfileArn(arn)
}

// SetModelName sets the model name for responses
func (o *Outbound) SetModelName(name string) {
	o.response.SetModelName(name)
}

// Reset resets the transformer state for reuse
func (o *Outbound) Reset() {
	o.response.Reset()
}

// BuildPassthroughRequest implements PassthroughOutbound interface
// Kiro doesn't support passthrough - it requires request transformation
func (o *Outbound) BuildPassthroughRequest(ctx context.Context, rawBody []byte, stream bool, baseUrl, key string, query url.Values) (*http.Request, error) {
	return o.request.BuildPassthroughRequest(ctx, rawBody, stream, baseUrl, key, query)
}

// IsRawStream implements RawStreamOutbound interface
// Kiro uses AWS Event Stream format, not SSE
func (o *Outbound) IsRawStream() bool {
	return true
}
