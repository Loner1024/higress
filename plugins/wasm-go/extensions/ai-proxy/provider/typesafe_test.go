package provider

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestTypesafeProviderInitializer_DefaultCapabilities(t *testing.T) {
	initializer := &typesafeProviderInitializer{}
	require.Equal(t, map[string]string{
		string(ApiNameSystemOne): PathSystemOne,
		string(ApiNameDecisions): PathSystemOne,
	}, initializer.DefaultCapabilities())
}

func TestTypesafeProviderValidateConfigRejectsOriginalProtocol(t *testing.T) {
	var config ProviderConfig
	config.FromJson(gjson.Parse(`{"type":"typesafe","apiTokens":["test-key"],"protocol":"original"}`))
	err := config.Validate()
	require.ErrorContains(t, err, "protocol: original")
}

func TestTypesafeProvider_TransformRequestAndResponse(t *testing.T) {
	provider, err := (&typesafeProviderInitializer{}).CreateProvider(ProviderConfig{
		typ:       providerTypeTypesafe,
		apiTokens: []string{"test-key"},
		modelMapping: map[string]string{
			"luna": "jev-latest",
		},
	})
	require.NoError(t, err)

	ctx := newMockMultipartHttpContext()
	body := []byte(`{
		"model": "luna",
		"input": "Customer wants a refund.",
		"questions": [
			{"type": "predicate", "name": "refund", "instructions": "Is this a refund request?"}
		]
	}`)
	encoded, err := provider.(*typesafeProvider).TransformRequestBody(ctx, ApiNameDecisions, body)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"model": "jev-latest",
		"state": "Customer wants a refund.",
		"questions": {
			"refund": {"type": "noul", "instructions": "Is this a refund request?"}
		}
	}`, string(encoded))

	response, err := provider.(*typesafeProvider).TransformResponseBody(ctx, ApiNameDecisions, []byte(`{
		"model": "jev-1.13.0",
		"answers": {"refund": {"type": "noul", "noul": 0.97}},
		"usage": {"input_tokens": 12, "output_tokens": 3}
	}`))
	require.NoError(t, err)
	require.JSONEq(t, `{
		"model": "jev-1.13.0",
		"answers": [{"type": "predicate", "name": "refund", "probability": 0.97}],
		"usage": {
			"input_tokens": 12,
			"input_tokens_details": {"cached_tokens": 0, "cache_write_tokens": 0},
			"output_tokens": 3,
			"output_tokens_details": {"reasoning_tokens": 0},
			"total_tokens": 15
		}
	}`, string(response))
}

func TestTypesafeProvider_SystemOneIsPassThrough(t *testing.T) {
	provider, err := (&typesafeProviderInitializer{}).CreateProvider(ProviderConfig{
		typ:          providerTypeTypesafe,
		modelMapping: map[string]string{"jev": "jev-latest"},
	})
	require.NoError(t, err)

	ctx := newMockMultipartHttpContext()
	body := []byte(`{
		"model": "jev",
		"state": "hello",
		"assets": {"photo": {"type": "image", "format": "png", "data_base64": "AAAA"}},
		"questions": {
			"yes": {"type": "noul", "instructions": "Is it?"},
			"area": {"type": "box", "instructions": "Where is it?"}
		}
	}`)
	encoded, err := provider.(*typesafeProvider).TransformRequestBody(ctx, ApiNameSystemOne, body)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"model": "jev-latest",
		"state": "hello",
		"assets": {"photo": {"type": "image", "format": "png", "data_base64": "AAAA"}},
		"questions": {
			"yes": {"type": "noul", "instructions": "Is it?"},
			"area": {"type": "box", "instructions": "Where is it?"}
		}
	}`, string(encoded))
}

func TestTypesafeProvider_SystemOneRejectsInvalidJSON(t *testing.T) {
	provider, err := (&typesafeProviderInitializer{}).CreateProvider(ProviderConfig{
		typ:       providerTypeTypesafe,
		apiTokens: []string{"test-key"},
	})
	require.NoError(t, err)

	tests := []struct {
		name string
		body string
		want string
	}{
		{"malformed", `{"model":`, "JSON object"},
		{"not an object", `[]`, "JSON object"},
		{"missing model", `{"state":"hello","questions":{}}`, "model is required"},
		{"non-string model", `{"model":5,"state":"hello","questions":{}}`, "model must be a string"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newMockMultipartHttpContext()
			_, err := provider.(*typesafeProvider).TransformRequestBody(ctx, ApiNameSystemOne, []byte(tt.body))
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestTypesafeProvider_DecisionsRejectsBoxAndStream(t *testing.T) {
	provider, err := (&typesafeProviderInitializer{}).CreateProvider(ProviderConfig{
		typ:       providerTypeTypesafe,
		apiTokens: []string{"test-key"},
	})
	require.NoError(t, err)

	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "box",
			body: `{"model":"m","input":"x","questions":[{"type":"box","name":"area","instructions":"Where?"}]}`,
			want: "box questions are not supported",
		},
		{
			name: "stream",
			body: `{"model":"m","input":"x","stream":true,"questions":[{"type":"predicate","name":"q","instructions":"q"}]}`,
			want: "stream is not supported",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newMockMultipartHttpContext()
			_, err := provider.(*typesafeProvider).TransformRequestBody(ctx, ApiNameDecisions, []byte(tt.body))
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestTypesafeProvider_DecisionsResponseRejectsEmptyAnswers(t *testing.T) {
	provider, err := (&typesafeProviderInitializer{}).CreateProvider(ProviderConfig{
		typ:       providerTypeTypesafe,
		apiTokens: []string{"test-key"},
	})
	require.NoError(t, err)

	ctx := newMockMultipartHttpContext()
	_, err = provider.(*typesafeProvider).TransformRequestBody(ctx, ApiNameDecisions, []byte(`{
		"model": "luna",
		"input": "hello",
		"questions": [{"type": "predicate", "name": "q", "instructions": "Is it?"}]
	}`))
	require.NoError(t, err)

	_, err = provider.(*typesafeProvider).TransformResponseBody(ctx, ApiNameDecisions, []byte(`{"model":"jev-1.13.0","answers":{}}`))
	require.ErrorContains(t, err, "answers must not be empty")
}
