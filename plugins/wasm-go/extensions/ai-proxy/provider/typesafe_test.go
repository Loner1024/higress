package provider

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTypesafeProviderInitializer_DefaultCapabilities(t *testing.T) {
	initializer := &typesafeProviderInitializer{}
	require.Equal(t, map[string]string{
		string(ApiNameSystemOne): PathSystemOne,
		string(ApiNameDecisions): PathSystemOne,
	}, initializer.DefaultCapabilities())
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
		"questions": {"yes": {"type": "noul", "instructions": "Is it?"}}
	}`)
	encoded, err := provider.(*typesafeProvider).TransformRequestBody(ctx, ApiNameSystemOne, body)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"model": "jev-latest",
		"state": "hello",
		"assets": {"photo": {"type": "image", "format": "png", "data_base64": "AAAA"}},
		"questions": {"yes": {"type": "noul", "instructions": "Is it?"}}
	}`, string(encoded))
}
