package relay

import "testing"

func TestNormalizeLogJSONPayload(t *testing.T) {
	t.Run("valid json normalized", func(t *testing.T) {
		got := normalizeLogJSONPayload([]byte("{\n  \"a\": 1,\n  \"b\": [2,3]\n}"))
		want := `{"a":1,"b":[2,3]}`
		if got != want {
			t.Fatalf("unexpected normalized payload: got %q want %q", got, want)
		}
	})

	t.Run("invalid json fallback", func(t *testing.T) {
		got := normalizeLogJSONPayload([]byte("not-json"))
		want := "not-json"
		if got != want {
			t.Fatalf("unexpected fallback payload: got %q want %q", got, want)
		}
	})

	t.Run("empty payload", func(t *testing.T) {
		got := normalizeLogJSONPayload([]byte{})
		if got != "" {
			t.Fatalf("unexpected empty payload result: got %q", got)
		}
	})
}

func TestErrorResponseBody(t *testing.T) {
	got := errorResponseBody(502, "all channels failed")
	want := `{"error":{"code":502,"message":"all channels failed","type":"relay_error"}}`
	if got != want {
		t.Fatalf("unexpected error response body: got %q want %q", got, want)
	}
}

