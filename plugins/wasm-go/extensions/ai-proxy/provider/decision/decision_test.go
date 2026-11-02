package decision

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIToSystemOne(t *testing.T) {
	body := []byte(`{
		"model": "luna",
		"input": "Customer was charged twice for order #4411.",
		"questions": [
			{
				"type": "predicate",
				"name": "is_refund",
				"instructions": "Is the customer asking for a refund?"
			},
			{
				"type": "choice",
				"name": "team",
				"instructions": "Which team should handle this?",
				"choices": [
					{"value": "billing", "description": "payments and invoices"},
					{"value": true},
					{"value": false}
				]
			},
			{
				"type": "score",
				"name": "urgency",
				"instructions": "How urgent is this?",
				"levels": [
					{"label": "calm"},
					{"label": "angry", "description": "uses hostile language"}
				]
			}
		]
	}`)

	request, err := ParseOpenAIRequest(body)
	require.NoError(t, err)
	require.NoError(t, request.ValidateForSystemOne())

	encoded, err := request.SystemOneBody()
	require.NoError(t, err)
	require.JSONEq(t, `{
		"model": "luna",
		"state": "Customer was charged twice for order #4411.",
		"questions": {
			"is_refund": {
				"type": "noul",
				"instructions": "Is the customer asking for a refund?"
			},
			"team": {
				"type": "choice",
				"instructions": "Which team should handle this?",
				"criteria": {
					"billing": "payments and invoices",
					"true": null,
					"false": null
				}
			},
			"urgency": {
				"type": "score",
				"instructions": "How urgent is this?",
				"criteria": ["calm", "angry: uses hostile language"]
			}
		}
	}`, string(encoded))
}

func TestSystemOneResponseToOpenAI(t *testing.T) {
	request, err := ParseOpenAIRequest([]byte(`{
		"model": "luna",
		"input": "Customer was charged twice for order #4411.",
		"questions": [
			{"type": "predicate", "name": "is_refund", "instructions": "Is this a refund?"},
			{
				"type": "choice",
				"name": "team",
				"instructions": "Which team?",
				"choices": [{"value": "billing"}, {"value": true}, {"value": false}]
			},
			{
				"type": "score",
				"name": "urgency",
				"instructions": "How urgent?",
				"levels": [{"label": "calm"}, {"label": "angry"}]
			}
		]
	}`))
	require.NoError(t, err)

	response, err := ParseSystemOneResponse([]byte(`{
		"model": "jev-1.13.0",
		"answers": {
			"is_refund": {"type": "noul", "noul": 0.98},
			"team": {
				"type": "choice",
				"choice": "billing",
				"confidence": 0.9,
				"probabilities": {"billing": 0.9, "true": 0.05, "false": 0.05}
			},
			"urgency": {
				"type": "score",
				"score": 1.2,
				"confidence": 0.8,
				"legend": {"0": "calm", "1": "angry"},
				"probabilities": {"0": 0.2, "1": 0.8}
			}
		},
		"usage": {"input_tokens": 100, "output_tokens": 10}
	}`))
	require.NoError(t, err)

	encoded, err := request.OpenAIResponseBody(response)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"model": "jev-1.13.0",
		"answers": [
			{"type": "predicate", "name": "is_refund", "probability": 0.98},
			{
				"type": "choice",
				"name": "team",
				"choice": "billing",
				"probabilities": [
					{"value": "billing", "probability": 0.9},
					{"value": true, "probability": 0.05},
					{"value": false, "probability": 0.05}
				],
				"confidence": 0.9
			},
			{
				"type": "score",
				"name": "urgency",
				"score": 1.2,
				"probabilities": [
					{"value": 0, "label": "calm", "probability": 0.2},
					{"value": 1, "label": "angry", "probability": 0.8}
				],
				"confidence": 0.8
			}
		],
		"usage": {
			"input_tokens": 100,
			"input_tokens_details": {"cached_tokens": 0, "cache_write_tokens": 0},
			"output_tokens": 10,
			"output_tokens_details": {"reasoning_tokens": 0},
			"total_tokens": 110
		}
	}`, string(encoded))
}

func TestSystemOneRoundTrip(t *testing.T) {
	body := []byte(`{
		"model": "jev-latest",
		"state": {"ticket": "payouts failing", "days": 3},
		"assets": {
			"photo": {"type": "image", "format": "png", "data_base64": "AAAA"}
		},
		"questions": {
			"urgent": {
				"type": "noul",
				"instructions": "Is this urgent?",
				"criteria": {"true": "needs action today", "false": "can wait"}
			},
			"team": {
				"type": "choice",
				"instructions": "Which team?",
				"criteria": {"billing": "payments", "technical": null}
			},
			"severity": {
				"type": "score",
				"instructions": "How severe?",
				"criteria": ["low", {"label": "high", "reason": "outage"}]
			},
			"area": {"type": "box", "instructions": "the failing chart"}
		}
	}`)

	request, err := ParseSystemOneRequest(body)
	require.NoError(t, err)
	encoded, err := request.SystemOneBody()
	require.NoError(t, err)
	require.JSONEq(t, string(body), string(encoded))
}

func TestOpenAIRejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "missing input",
			body: `{"model":"m","questions":[{"type":"predicate","instructions":"q"}]}`,
		},
		{
			name: "one choice",
			body: `{"model":"m","input":"x","questions":[{"type":"choice","instructions":"q","choices":[{"value":"a"}]}]}`,
		},
		{
			name: "duplicate choice key",
			body: `{"model":"m","input":"x","questions":[{"type":"choice","instructions":"q","choices":[{"value":true},{"value":"true"}]}]}`,
		},
		{
			name: "one score level",
			body: `{"model":"m","input":"x","questions":[{"type":"score","instructions":"q","levels":[{"label":"only"}]}]}`,
		},
		{
			name: "unsupported type",
			body: `{"model":"m","input":"x","questions":[{"type":"ranking","instructions":"q"}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseOpenAIRequest([]byte(tt.body))
			require.Error(t, err)
		})
	}
}

func TestValidateForSystemOneRejectsImages(t *testing.T) {
	request, err := ParseOpenAIRequest([]byte(`{
		"model": "m",
		"input": [{
			"type": "message",
			"role": "user",
			"content": [
				{"type": "input_text", "text": "What color is this?"},
				{"type": "input_image", "image_url": "data:image/png;base64,AAAA"}
			]
		}],
		"questions": [{"type": "predicate", "name": "red", "instructions": "Is it red?"}]
	}`))
	require.NoError(t, err)
	require.ErrorContains(t, request.ValidateForSystemOne(), "input_image")
}

func TestSystemOneResponseBody(t *testing.T) {
	request, err := ParseOpenAIRequest([]byte(`{
		"model": "luna",
		"input": "hello",
		"questions": [
			{"type": "predicate", "name": "yes", "instructions": "Is it?"},
			{"type": "choice", "name": "pick", "instructions": "Pick", "choices": [{"value": true}, {"value": false}]}
		]
	}`))
	require.NoError(t, err)

	encoded, err := request.SystemOneResponseBody(&Response{
		Model: "jev-1.13.0",
		Answers: map[string]Answer{
			"yes": {Type: TypePredicate, Noul: ptr(0.75)},
			"pick": {
				Type:          TypeChoice,
				Choice:        ptr("true"),
				Confidence:    ptr(0.8),
				Probabilities: map[string]float64{"true": 0.8, "false": 0.2},
			},
		},
		Usage: &Usage{InputTokens: 5, OutputTokens: 2},
	})
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Equal(t, "jev-1.13.0", decoded["model"])
	require.Equal(t, map[string]any{
		"yes":  map[string]any{"type": "noul", "noul": 0.75},
		"pick": map[string]any{"type": "choice", "choice": "true", "probabilities": map[string]any{"true": 0.8, "false": 0.2}, "confidence": 0.8},
	}, decoded["answers"])
}

func ptr[T any](value T) *T {
	return &value
}
