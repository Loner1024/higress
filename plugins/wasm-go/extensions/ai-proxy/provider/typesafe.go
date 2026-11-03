package provider

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/provider/decision"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/util"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/sjson"
)

const (
	defaultTypesafeDomain = "api.typesafe.ai"
	ctxKeyDecisionRequest = "decisionRequest"
)

// typesafeProvider serves TypeSafe's System One API and translates the OpenAI
// Decisions wire format to it.
type typesafeProviderInitializer struct{}

func (m *typesafeProviderInitializer) ValidateConfig(config *ProviderConfig) error {
	// The OpenAI /v1/decisions translation only runs when the plugin owns the
	// protocol. With protocol: original the body would pass through untranslated
	// while the path still becomes /v1/systemone, and the response could not be
	// mapped back.
	if config.IsOriginal() {
		return errors.New("typesafe provider does not support protocol: original")
	}
	return nil
}

func (m *typesafeProviderInitializer) DefaultCapabilities() map[string]string {
	return map[string]string{
		string(ApiNameSystemOne): PathSystemOne,
		string(ApiNameDecisions): PathSystemOne,
	}
}

func (m *typesafeProviderInitializer) CreateProvider(config ProviderConfig) (Provider, error) {
	config.setDefaultCapabilities(m.DefaultCapabilities())
	return &typesafeProvider{config: config}, nil
}

type typesafeProvider struct {
	config ProviderConfig
}

func (m *typesafeProvider) GetProviderType() string {
	return providerTypeTypesafe
}

func (m *typesafeProvider) OnRequestHeaders(ctx wrapper.HttpContext, apiName ApiName) error {
	m.config.handleRequestHeaders(m, ctx, apiName)
	return nil
}

func (m *typesafeProvider) OnRequestBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) (types.Action, error) {
	if !m.config.isSupportedAPI(apiName) {
		return types.ActionContinue, errUnsupportedApiName
	}
	action, err := m.config.handleRequestBody(m, nil, ctx, apiName, body)
	if err != nil {
		log.Warnf("[typesafe] rejected %s request: %v", apiName, err)
		return sendDecisionBadRequest(err), nil
	}
	return action, nil
}

func (m *typesafeProvider) TransformRequestHeaders(ctx wrapper.HttpContext, apiName ApiName, headers http.Header) {
	util.OverwriteRequestPathHeaderByCapability(headers, string(apiName), m.config.capabilities)
	util.OverwriteRequestHostHeader(headers, defaultTypesafeDomain)
	if token := m.config.GetApiTokenInUse(ctx); token != "" {
		util.OverwriteRequestAuthorizationHeader(headers, "Bearer "+token)
	}
	headers.Del("Content-Length")
}

func (m *typesafeProvider) TransformRequestBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) ([]byte, error) {
	switch apiName {
	case ApiNameSystemOne:
		// Keep the native body intact: assets, images, and future fields must
		// reach TypeSafe unchanged. Only the model mapping is applied.
		model, err := decision.SystemOneRequestModel(body)
		if err != nil {
			return nil, err
		}
		mapped := getMappedModel(model, m.config.modelMapping)
		ctx.SetContext(ctxKeyOriginalRequestModel, model)
		ctx.SetContext(ctxKeyFinalRequestModel, mapped)
		return sjson.SetBytes(body, "model", mapped)
	case ApiNameDecisions:
		request, err := decision.ParseOpenAIRequest(body)
		if err != nil {
			return nil, err
		}
		if err := request.ValidateForSystemOne(); err != nil {
			return nil, err
		}
		ctx.SetContext(ctxKeyOriginalRequestModel, request.Model)
		request.Model = getMappedModel(request.Model, m.config.modelMapping)
		ctx.SetContext(ctxKeyFinalRequestModel, request.Model)
		ctx.SetContext(ctxKeyDecisionRequest, request)
		return request.SystemOneBody()
	default:
		return body, nil
	}
}

func (m *typesafeProvider) TransformResponseBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) ([]byte, error) {
	if apiName != ApiNameDecisions {
		return body, nil
	}
	request, ok := ctx.GetContext(ctxKeyDecisionRequest).(*decision.Request)
	if !ok || request == nil {
		return nil, errors.New("decisions request context is missing")
	}
	response, err := decision.ParseSystemOneResponse(body)
	if err != nil {
		return nil, err
	}
	return request.OpenAIResponseBody(response)
}

func sendDecisionBadRequest(err error) types.Action {
	body, _ := json.Marshal(map[string]any{
		"error": map[string]any{
			"message": err.Error(),
			"type":    "invalid_request_error",
			"param":   nil,
			"code":    "400",
		},
	})
	_ = proxywasm.SendHttpResponseWithDetail(
		http.StatusBadRequest,
		"ai-proxy.decision_invalid_request",
		util.CreateHeaders(util.HeaderContentType, util.MimeTypeApplicationJson),
		body,
		-1,
	)
	return types.ActionPause
}
