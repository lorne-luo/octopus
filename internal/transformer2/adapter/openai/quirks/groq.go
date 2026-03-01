package quirks

func init() {
	Register("groq", &groqInterceptor{})
}

// groqInterceptor strips fields unsupported by Groq.
type groqInterceptor struct{}

func (g *groqInterceptor) ModifyRequestBody(body []byte) ([]byte, error) {
	return stripFields(body,
		"metadata",
		"logit_bias",
		"service_tier",
		"store",
	)
}
