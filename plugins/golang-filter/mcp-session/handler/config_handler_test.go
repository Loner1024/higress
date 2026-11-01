package handler

import (
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/envoyproxy/envoy/contrib/golang/common/go/api"
)

type handlerTestCAPI struct{}

func (handlerTestCAPI) Log(api.LogType, string) {}
func (handlerTestCAPI) LogLevel() api.LogType   { return api.Debug }

type handlerTestCallbacks struct {
	api.FilterCallbackHandler
	decoder *handlerTestDecoderCallbacks
}

func (c *handlerTestCallbacks) DecoderFilterCallbacks() api.DecoderFilterCallbacks {
	return c.decoder
}

type handlerTestDecoderCallbacks struct {
	api.DecoderFilterCallbacks
	statusCode int
}

func (c *handlerTestDecoderCallbacks) SendLocalReply(statusCode int, _ string, _ map[string][]string, _ int64, _ string) {
	c.statusCode = statusCode
}

type handlerTestConfigStore struct {
	getServer     string
	getConsumer   string
	storeServer   string
	storeConsumer string
	getCalls      int
	storeCalls    int
	err           error
}

func (s *handlerTestConfigStore) GetConfig(serverName, consumer string) (map[string]string, error) {
	s.getServer = serverName
	s.getConsumer = consumer
	s.getCalls++
	if s.err != nil {
		return nil, s.err
	}
	return map[string]string{"upstream": "safe"}, nil
}

func (s *handlerTestConfigStore) StoreConfig(serverName, consumer string, _ map[string]string) (*ConfigResponse, error) {
	s.storeServer = serverName
	s.storeConsumer = consumer
	s.storeCalls++
	if s.err != nil {
		return nil, s.err
	}
	return &ConfigResponse{Success: true}, nil
}

func TestHandleConfigRequestBindsPathUIDToConsumer(t *testing.T) {
	api.SetCommonCAPI(handlerTestCAPI{})
	tests := []struct {
		name          string
		method        string
		consumer      string
		body          string
		statusCode    int
		expectedReads int
		expectedWrite int
	}{
		{name: "missing consumer", method: http.MethodGet, statusCode: http.StatusUnauthorized},
		{name: "cross identity read", method: http.MethodGet, consumer: "bob", statusCode: http.StatusForbidden},
		{name: "cross identity write", method: http.MethodPost, consumer: "bob", body: `{"config":{"upstream":"other"}}`, statusCode: http.StatusForbidden},
		{name: "valid read", method: http.MethodGet, consumer: "alice", statusCode: http.StatusOK, expectedReads: 1},
		{name: "valid write", method: http.MethodPost, consumer: "alice", body: `{"config":{"upstream":"safe"}}`, statusCode: http.StatusOK, expectedWrite: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &handlerTestConfigStore{}
			decoder := &handlerTestDecoderCallbacks{}
			h := &MCPConfigHandler{configStore: store, callbacks: &handlerTestCallbacks{decoder: decoder}}
			req := &http.Request{Method: tt.method, URL: &url.URL{Path: "/server/alice/config"}}
			if handled := h.HandleConfigRequest(req, tt.consumer, []byte(tt.body)); !handled {
				t.Fatal("expected config request to be handled")
			}
			if decoder.statusCode != tt.statusCode {
				t.Fatalf("expected status %d, got %d", tt.statusCode, decoder.statusCode)
			}
			if store.getCalls != tt.expectedReads || store.storeCalls != tt.expectedWrite {
				t.Fatalf("unexpected store calls: reads=%d writes=%d", store.getCalls, store.storeCalls)
			}
			if tt.expectedReads == 1 && (store.getServer != "server" || store.getConsumer != "alice") {
				t.Fatalf("read used %q/%q", store.getServer, store.getConsumer)
			}
			if tt.expectedWrite == 1 && (store.storeServer != "server" || store.storeConsumer != "alice") {
				t.Fatalf("write used %q/%q", store.storeServer, store.storeConsumer)
			}
		})
	}
}

func TestHandleConfigRequestReturnsStoreError(t *testing.T) {
	api.SetCommonCAPI(handlerTestCAPI{})
	store := &handlerTestConfigStore{err: errors.New("redis unavailable")}
	decoder := &handlerTestDecoderCallbacks{}
	h := &MCPConfigHandler{configStore: store, callbacks: &handlerTestCallbacks{decoder: decoder}}
	req := &http.Request{Method: http.MethodGet, URL: &url.URL{Path: "/server/alice/config"}}
	h.HandleConfigRequest(req, "alice", nil)
	if decoder.statusCode != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", decoder.statusCode)
	}
}

type handlerTestEvalClient struct {
	keys   [][]string
	result interface{}
	err    error
}

func (c *handlerTestEvalClient) Eval(_ string, _ int, keys []string, _ []interface{}) (interface{}, error) {
	c.keys = append(c.keys, append([]string(nil), keys...))
	if c.err != nil {
		return nil, c.err
	}
	if c.result != nil {
		return c.result, nil
	}
	return []interface{}{int64(10), int64(9), int64(60)}, nil
}

func TestRateLimitUsesVerifiedConsumer(t *testing.T) {
	redis := &handlerTestEvalClient{}
	h := &MCPRatelimitHandler{redisClient: redis, limit: 10, window: 60}
	req := &http.Request{URL: &url.URL{Path: "/server/path-uid"}}
	if !h.HandleRatelimit(req, "server", "alice", nil) || !h.HandleRatelimit(req, "server", "bob", nil) {
		t.Fatal("expected requests within the limit to continue")
	}
	if len(redis.keys) != 2 {
		t.Fatalf("expected two Redis calls, got %d", len(redis.keys))
	}
	if redis.keys[0][0] != "mcp-server-limit:server:alice:60:10" || redis.keys[1][0] != "mcp-server-limit:server:bob:60:10" {
		t.Fatalf("unexpected bucket keys: %v", redis.keys)
	}
}

func TestRateLimitWhitelistRequiresVerifiedConsumer(t *testing.T) {
	redis := &handlerTestEvalClient{}
	h := &MCPRatelimitHandler{redisClient: redis, whitelist: []string{"alice"}, limit: 10, window: 60}
	req := &http.Request{URL: &url.URL{Path: "/server/alice"}}
	if !h.HandleRatelimit(req, "server", "bob", nil) {
		t.Fatal("expected non-whitelisted consumer to be evaluated")
	}
	if len(redis.keys) != 1 || redis.keys[0][0] != "mcp-server-limit:server:bob:60:10" {
		t.Fatalf("unexpected Redis calls: %v", redis.keys)
	}
	if !h.HandleRatelimit(req, "server", "alice", nil) || len(redis.keys) != 1 {
		t.Fatal("expected only the verified whitelist consumer to skip evaluation")
	}
}

func TestRateLimitRejectsMissingConsumerAndRedisError(t *testing.T) {
	api.SetCommonCAPI(handlerTestCAPI{})
	redis := &handlerTestEvalClient{}
	decoder := &handlerTestDecoderCallbacks{}
	h := &MCPRatelimitHandler{redisClient: redis, callbacks: &handlerTestCallbacks{decoder: decoder}}
	req := &http.Request{URL: &url.URL{Path: "/server/alice"}}
	if h.HandleRatelimit(req, "server", "", nil) || decoder.statusCode != http.StatusUnauthorized || len(redis.keys) != 0 {
		t.Fatal("expected missing consumer to return 401 without Redis access")
	}
	redis.err = errors.New("redis unavailable")
	if h.HandleRatelimit(req, "server", "alice", nil) || decoder.statusCode != http.StatusInternalServerError {
		t.Fatalf("expected Redis error to return 500, got %d", decoder.statusCode)
	}
}
