package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	wasmtest "github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

func TestTypesafeDecisionsRouteEndToEnd(t *testing.T) {
	wasmtest.RunGoTest(t, func(t *testing.T) {
		host, status := wasmtest.NewTestHost(json.RawMessage(`{
			"provider": {
				"type": "typesafe",
				"apiTokens": ["typesafe-key"]
			}
		}`))
		require.Equal(t, types.OnPluginStartStatusOK, status)
		defer host.Reset()

		require.Equal(t, types.HeaderStopIteration, host.CallOnHttpRequestHeaders([][2]string{
			{":method", "POST"},
			{":path", "/v1/decisions"},
			{":authority", "gateway.example.com"},
			{"content-type", "application/json"},
			{"authorization", "Bearer client-key"},
		}))
		headers := headerMap(host.GetRequestHeaders())
		require.Equal(t, "/v1/systemone", headers[":path"])
		require.Equal(t, "api.typesafe.ai", headers[":authority"])
		require.Equal(t, "Bearer typesafe-key", headers["authorization"])

		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody([]byte(`{
			"model": "luna",
			"input": "Customer was charged twice.",
			"questions": [
				{"type": "predicate", "name": "refund", "instructions": "Is this a refund request?"},
				{
					"type": "choice",
					"name": "team",
					"instructions": "Which team?",
					"choices": [{"value": "billing"}, {"value": "technical"}]
				}
			]
		}`)))
		require.JSONEq(t, `{
			"model": "luna",
			"state": "Customer was charged twice.",
			"questions": {
				"refund": {"type": "noul", "instructions": "Is this a refund request?"},
				"team": {
					"type": "choice",
					"instructions": "Which team?",
					"criteria": {"billing": null, "technical": null}
				}
			}
		}`, string(host.GetRequestBody()))

		require.NoError(t, host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream")))
		require.Equal(t, types.ActionContinue, host.CallOnHttpResponseHeaders([][2]string{
			{":status", "200"},
			{"content-type", "application/json"},
		}))
		require.Equal(t, types.ActionContinue, host.CallOnHttpResponseBody([]byte(`{
			"model": "jev-1.13.0",
			"answers": {
				"refund": {"type": "noul", "noul": 0.97},
				"team": {
					"type": "choice",
					"choice": "billing",
					"confidence": 0.9,
					"probabilities": {"billing": 0.9, "technical": 0.1}
				}
			},
			"usage": {"input_tokens": 20, "output_tokens": 4}
		}`)))
		require.JSONEq(t, `{
			"model": "jev-1.13.0",
			"answers": [
				{"type": "predicate", "name": "refund", "probability": 0.97},
				{
					"type": "choice",
					"name": "team",
					"choice": "billing",
					"probabilities": [
						{"value": "billing", "probability": 0.9},
						{"value": "technical", "probability": 0.1}
					],
					"confidence": 0.9
				}
			],
			"usage": {
				"input_tokens": 20,
				"input_tokens_details": {"cached_tokens": 0, "cache_write_tokens": 0},
				"output_tokens": 4,
				"output_tokens_details": {"reasoning_tokens": 0},
				"total_tokens": 24
			}
		}`, string(host.GetResponseBody()))
		host.CompleteHttp()
	})
}

func TestTypesafeSystemOneRouteEndToEnd(t *testing.T) {
	wasmtest.RunGoTest(t, func(t *testing.T) {
		host, status := wasmtest.NewTestHost(json.RawMessage(`{
			"provider": {
				"type": "typesafe",
				"apiTokens": ["typesafe-key"],
				"modelMapping": {"jev": "jev-latest"}
			}
		}`))
		require.Equal(t, types.OnPluginStartStatusOK, status)
		defer host.Reset()

		require.Equal(t, types.HeaderStopIteration, host.CallOnHttpRequestHeaders([][2]string{
			{":method", "POST"},
			{":path", "/v1/systemone"},
			{":authority", "gateway.example.com"},
			{"content-type", "application/json"},
			{"TypeSafe-Danger-Zone-Images", "true"},
		}))
		headers := headerMap(host.GetRequestHeaders())
		require.Equal(t, "/v1/systemone", headers[":path"])
		require.Equal(t, "api.typesafe.ai", headers[":authority"])
		require.Equal(t, "true", headers["typesafe-danger-zone-images"])

		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody([]byte(`{
			"model": "jev",
			"state": "hello",
			"assets": {"photo": {"type": "image", "format": "png", "data_base64": "AAAA"}},
			"questions": {"yes": {"type": "noul", "instructions": "Is it?"}}
		}`)))
		require.JSONEq(t, `{
			"model": "jev-latest",
			"state": "hello",
			"assets": {"photo": {"type": "image", "format": "png", "data_base64": "AAAA"}},
			"questions": {"yes": {"type": "noul", "instructions": "Is it?"}}
		}`, string(host.GetRequestBody()))

		response := `{"model":"jev-1.13.0","answers":{"yes":{"type":"noul","noul":0.9}},"usage":{"input_tokens":3,"output_tokens":1}}`
		require.NoError(t, host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream")))
		require.Equal(t, types.ActionContinue, host.CallOnHttpResponseHeaders([][2]string{
			{":status", "200"},
			{"content-type", "application/json"},
		}))
		require.Equal(t, types.ActionContinue, host.CallOnHttpResponseBody([]byte(response)))
		require.JSONEq(t, response, string(host.GetResponseBody()))
		host.CompleteHttp()
	})
}

func TestTypesafeForwardsClientAuthorization(t *testing.T) {
	wasmtest.RunGoTest(t, func(t *testing.T) {
		host, status := wasmtest.NewTestHost(json.RawMessage(`{
			"provider": {"type": "typesafe"}
		}`))
		require.Equal(t, types.OnPluginStartStatusOK, status)
		defer host.Reset()

		host.CallOnHttpRequestHeaders([][2]string{
			{":method", "POST"},
			{":path", "/v1/systemone"},
			{":authority", "gateway.example.com"},
			{"content-type", "application/json"},
			{"authorization", "Bearer client-key"},
		})
		headers := headerMap(host.GetRequestHeaders())
		require.Equal(t, "Bearer client-key", headers["authorization"])
		host.CompleteHttp()
	})
}

func TestTypesafeDecisionsRouteRejectsInvalidBody(t *testing.T) {
	wasmtest.RunGoTest(t, func(t *testing.T) {
		host, status := wasmtest.NewTestHost(json.RawMessage(`{
			"provider": {"type": "typesafe", "apiTokens": ["typesafe-key"]}
		}`))
		require.Equal(t, types.OnPluginStartStatusOK, status)
		defer host.Reset()

		host.CallOnHttpRequestHeaders([][2]string{
			{":method", "POST"},
			{":path", "/v1/decisions"},
			{":authority", "gateway.example.com"},
			{"content-type", "application/json"},
		})
		require.Equal(t, types.ActionPause, host.CallOnHttpRequestBody([]byte(`{
			"model": "luna", "input": "x", "questions": []
		}`)))
		local := host.GetLocalResponse()
		require.NotNil(t, local)
		require.Equal(t, uint32(400), local.StatusCode)
		require.Contains(t, string(local.Data), "at least one question")
		host.CompleteHttp()
	})
}

func TestNonTypesafeProviderDecisionPathsPassThrough(t *testing.T) {
	wasmtest.RunGoTest(t, func(t *testing.T) {
		for _, path := range []string{"/v1/decisions", "/v1/systemone"} {
			t.Run(path, func(t *testing.T) {
				host, status := wasmtest.NewTestHost(json.RawMessage(`{
					"provider": {"type": "deepseek", "apiTokens": ["deepseek-key"]}
				}`))
				require.Equal(t, types.OnPluginStartStatusOK, status)
				defer host.Reset()

				require.Equal(t, types.HeaderStopIteration, host.CallOnHttpRequestHeaders([][2]string{
					{":method", "POST"},
					{":path", path},
					{":authority", "gateway.example.com"},
					{"content-type", "application/json"},
				}))
				// deepseek does not own these API names, so the original path
				// must survive instead of being rejected as unsupported.
				require.Equal(t, path, headerMap(host.GetRequestHeaders())[":path"])
				require.Nil(t, host.GetLocalResponse())

				body := []byte(`{"model":"m","input":"x","questions":[{"type":"predicate","name":"q","instructions":"q"}]}`)
				require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(body))
				require.Nil(t, host.GetLocalResponse())
				require.JSONEq(t, string(body), string(host.GetRequestBody()))

				require.NoError(t, host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream")))
				require.Equal(t, types.ActionContinue, host.CallOnHttpResponseHeaders([][2]string{
					{":status", "200"},
					{"content-type", "application/json"},
				}))
				response := []byte(`{"ok":true}`)
				require.Equal(t, types.ActionContinue, host.CallOnHttpResponseBody(response))
				require.JSONEq(t, string(response), string(host.GetResponseBody()))
				host.CompleteHttp()
			})
		}
	})
}

func headerMap(headers [][2]string) map[string]string {
	result := make(map[string]string, len(headers))
	for _, header := range headers {
		result[strings.ToLower(header[0])] = header[1]
	}
	return result
}
