package decision

import (
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

func TestSystemOneResponseRejectsMissingOrEmptyAnswers(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "missing answers",
			body: `{"model":"jev-1.13.0","usage":{"input_tokens":1,"output_tokens":1}}`,
			want: "answers is required",
		},
		{
			name: "empty answers",
			body: `{"model":"jev-1.13.0","answers":{}}`,
			want: "answers must not be empty",
		},
		{
			name: "null answers",
			body: `{"model":"jev-1.13.0","answers":null}`,
			want: "answers must be an object",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseSystemOneResponse([]byte(tt.body))
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestSystemOneResponseUnknownAnswerTypeBecomesRefusal(t *testing.T) {
	request, err := ParseOpenAIRequest([]byte(`{
		"model": "luna",
		"input": "hello",
		"questions": [
			{"type": "predicate", "name": "first", "instructions": "Is it?"},
			{"type": "predicate", "name": "second", "instructions": "Is it still?"}
		]
	}`))
	require.NoError(t, err)

	response, err := ParseSystemOneResponse([]byte(`{
		"model": "jev-1.13.0",
		"answers": {
			"first": {"type": "mystery", "value": 1},
			"second": {"type": "noul", "noul": 0.4}
		}
	}`))
	require.NoError(t, err)

	encoded, err := request.OpenAIResponseBody(response)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"model": "jev-1.13.0",
		"answers": [
			{"type": "refusal", "name": "first"},
			{"type": "predicate", "name": "second", "probability": 0.4}
		],
		"usage": {
			"input_tokens": 0,
			"input_tokens_details": {"cached_tokens": 0, "cache_write_tokens": 0},
			"output_tokens": 0,
			"output_tokens_details": {"reasoning_tokens": 0},
			"total_tokens": 0
		}
	}`, string(encoded))
}

func TestSystemOneRequestModel(t *testing.T) {
	model, err := SystemOneRequestModel([]byte(`{"model":"jev-latest","state":"hello","questions":{"q":{"type":"noul"}}}`))
	require.NoError(t, err)
	require.Equal(t, "jev-latest", model)

	tests := []struct {
		name string
		body string
		want string
	}{
		{"malformed json", `{"model":`, "JSON object"},
		{"not an object", `[]`, "JSON object"},
		{"missing model", `{"state":"hello","questions":{}}`, "model is required"},
		{"non-string model", `{"model":5}`, "model must be a string"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SystemOneRequestModel([]byte(tt.body))
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestValidateForSystemOneRejectsBoxAndStream(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "box question",
			body: `{"model":"m","input":"x","questions":[{"type":"box","name":"area","instructions":"Where is it?"}]}`,
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
			request, err := ParseOpenAIRequest([]byte(tt.body))
			require.NoError(t, err)
			require.ErrorContains(t, request.ValidateForSystemOne(), tt.want)
		})
	}
}

func TestOpenAIInputMessagesBecomeSystemOneState(t *testing.T) {
	body := []byte(`{
		"model": "luna",
		"input": [
			{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "first"}]},
			{"type": "message", "role": "assistant", "content": "second"}
		],
		"questions": [{"type": "predicate", "name": "ok", "instructions": "Is it ok?"}]
	}`)
	request, err := ParseOpenAIRequest(body)
	require.NoError(t, err)
	encoded, err := request.SystemOneBody()
	require.NoError(t, err)
	require.JSONEq(t, `{
		"model": "luna",
		"state": [
			{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "first"}]},
			{"type": "message", "role": "assistant", "content": "second"}
		],
		"questions": {"ok": {"type": "noul", "instructions": "Is it ok?"}}
	}`, string(encoded))
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
		{
			name: "non-string model",
			body: `{"model":5,"input":"x","questions":[{"type":"predicate","instructions":"q"}]}`,
		},
		{
			name: "non-string name",
			body: `{"model":"m","input":"x","questions":[{"type":"predicate","name":{"nested":true},"instructions":"q"}]}`,
		},
		{
			name: "non-boolean stream",
			body: `{"model":"m","input":"x","stream":"yes","questions":[{"type":"predicate","instructions":"q"}]}`,
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
