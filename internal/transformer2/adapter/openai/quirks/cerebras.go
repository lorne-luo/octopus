package quirks

func init() {
	Register("cerebras", &cerebrasInterceptor{})
}

// cerebrasInterceptor strips fields unsupported by Cerebras.
type cerebrasInterceptor struct{}

func (c *cerebrasInterceptor) ModifyRequestBody(body []byte) ([]byte, error) {
	return stripFields(body,
		"metadata",
		"logit_bias",
		"logprobs",
		"top_logprobs",
		"service_tier",
		"store",
	)
}
