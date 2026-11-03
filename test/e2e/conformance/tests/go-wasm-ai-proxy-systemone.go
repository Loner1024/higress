// Copyright (c) 2026 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tests

import (
	"testing"

	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/http"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/suite"
)

func init() {
	Register(WasmPluginsAiProxySystemOne)
}

var WasmPluginsAiProxySystemOne = suite.ConformanceTest{
	ShortName:   "WasmPluginAiProxySystemOne",
	Description: "The ai-proxy typesafe provider serves System One and OpenAI Decisions requests through one IR.",
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Manifests:   []string{"tests/go-wasm-ai-proxy-systemone.yaml"},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		testcases := []http.Assertion{
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "systemone route: native request and response",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "api.typesafe.ai",
						Path:        "/v1/systemone",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body: []byte(`{
							"model": "jev",
							"state": "Customer wants a refund",
							"questions": {
								"refund": {"type": "noul", "instructions": "Is this a refund?"},
								"team": {"type": "choice", "instructions": "Which team?", "criteria": {"billing": "payments", "technical": "bugs"}},
								"urgency": {"type": "score", "instructions": "How urgent?", "criteria": ["low", "high"]}
							}
						}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body: []byte(`{
							"model": "jev-latest",
							"answers": {
								"refund": {"type": "noul", "noul": 0.75},
								"team": {
									"type": "choice",
									"choice": "billing",
									"confidence": 0.9,
									"probabilities": {"billing": 0.9, "technical": 0.1}
								},
								"urgency": {
									"type": "score",
									"score": 0.5,
									"confidence": 0.8,
									"legend": {"0": "low", "1": "high"},
									"probabilities": {"0": 0.5, "1": 0.5}
								}
							},
							"usage": {"input_tokens": 10, "output_tokens": 2}
						}`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "systemone route: box question passes through",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "api.typesafe.ai",
						Path:        "/v1/systemone",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body: []byte(`{
							"model": "jev",
							"state": "Find the failing chart",
							"questions": {
								"area": {"type": "box", "instructions": "Find the failing chart"}
							}
						}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body: []byte(`{
							"model": "jev-latest",
							"answers": {
								"area": {
									"type": "box",
									"box": {"top": 0.1, "bottom": 0.2, "left": 0.3, "right": 0.4},
									"confidence": 0.9
								}
							},
							"usage": {"input_tokens": 10, "output_tokens": 2}
						}`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "decisions route: OpenAI request converted to System One and back",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "api.typesafe.ai",
						Path:        "/v1/decisions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body: []byte(`{
							"model": "luna",
							"input": "Customer wants a refund",
							"questions": [
								{"type": "predicate", "name": "refund", "instructions": "Is this a refund?"},
								{"type": "choice", "name": "team", "instructions": "Which team?", "choices": [{"value": "billing"}, {"value": "technical"}]},
								{"type": "score", "name": "urgency", "instructions": "How urgent?", "levels": [{"label": "low"}, {"label": "high"}]}
							]
						}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body: []byte(`{
							"model": "jev-latest",
							"answers": [
								{"type": "predicate", "name": "refund", "probability": 0.75},
								{
									"type": "choice",
									"name": "team",
									"choice": "billing",
									"probabilities": [
										{"value": "billing", "probability": 0.9},
										{"value": "technical", "probability": 0.1}
									],
									"confidence": 0.9
								},
								{
									"type": "score",
									"name": "urgency",
									"score": 0.5,
									"probabilities": [
										{"value": 0, "label": "low", "probability": 0.5},
										{"value": 1, "label": "high", "probability": 0.5}
									],
									"confidence": 0.8
								}
							],
							"usage": {
								"input_tokens": 10,
								"input_tokens_details": {"cached_tokens": 0, "cache_write_tokens": 0},
								"output_tokens": 2,
								"output_tokens_details": {"reasoning_tokens": 0},
								"total_tokens": 12
							}
						}`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "decisions route: box question is rejected locally",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "api.typesafe.ai",
						Path:        "/v1/decisions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body: []byte(`{
							"model": "luna",
							"input": "Find the failing chart",
							"questions": [
								{"type": "box", "name": "area", "instructions": "Find the failing chart"}
							]
						}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{StatusCode: 400},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "decisions route: malformed body is rejected locally",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "api.typesafe.ai",
						Path:        "/v1/decisions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"luna","input":"x","questions":[]}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{StatusCode: 400},
				},
			},
		}

		for _, testcase := range testcases {
			t.Run(testcase.Meta.TestCaseName, func(t *testing.T) {
				http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)
			})
		}
	},
}
