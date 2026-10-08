package localllm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestEmbeddingResponseValidation(t *testing.T) {
	body := `{"model":"embed","data":[{"index":0,"embedding":[1,2]}]}`
	c := New(Config{BaseURL: "http://127.0.0.1:1234/v1", EmbeddingModel: "embed", Dimensions: 2})
	c.HTTP.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	if _, err := c.Embed(context.Background(), "test"); err != nil {
		t.Fatal(err)
	}
	body = `{"model":"embed","data":[{"index":1,"embedding":[1,2]}]}`
	if _, err := c.Embed(context.Background(), "test"); err == nil {
		t.Fatal("accepted wrong response index")
	}
}

func TestProviderConfiguration(t *testing.T) {
	t.Setenv("EMBEDDING_PROVIDER", "")
	t.Setenv("CHAT_PROVIDER", "")
	t.Setenv("EMBEDDING_BASE_URL", "")
	t.Setenv("CHAT_BASE_URL", "")
	t.Setenv("CHAT_MAX_TOKENS", "")
	t.Setenv("EMBEDDING_MODEL", "BAAI/bge-m3")
	t.Setenv("EMBEDDING_REVISION", "deepinfra-bge-m3-unpinned")
	t.Setenv("EMBEDDING_DIMENSIONS", "1024")
	t.Setenv("CHAT_MODEL", "openai/gpt-oss-120b")
	t.Setenv("CHAT_REVISION", "deepinfra-gpt-oss-120b-unpinned")
	t.Setenv("AI_PROVIDER", "deepinfra")
	t.Setenv("AI_BASE_URL", "http://example.com/v1")
	t.Setenv("DEEPINFRA_API_KEY", "")
	if _, err := FromEnv(); err == nil {
		t.Fatal("accepted hosted provider without an API key")
	}
	t.Setenv("DEEPINFRA_API_KEY", "test-key")
	cfg, err := FromEnv()
	if err != nil || cfg.EmbeddingBaseURL != "https://api.deepinfra.com/v1/openai" || cfg.ChatBaseURL != cfg.EmbeddingBaseURL {
		t.Fatalf("hosted configuration: %+v, %v", cfg, err)
	}
	t.Setenv("AI_PROVIDER", "lmstudio")
	if _, err := FromEnv(); err == nil {
		t.Fatal("accepted a nonlocal LM Studio endpoint")
	}
}

func TestIndependentOpenRouterChatAndEmbeddingConfiguration(t *testing.T) {
	t.Setenv("AI_PROVIDER", "")
	t.Setenv("AI_BASE_URL", "http://127.0.0.1:1234/v1")
	t.Setenv("EMBEDDING_PROVIDER", "lmstudio")
	t.Setenv("CHAT_PROVIDER", "openrouter")
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("EMBEDDING_MODEL", "local-embed")
	t.Setenv("EMBEDDING_REVISION", "")
	t.Setenv("EMBEDDING_DIMENSIONS", "768")
	t.Setenv("CHAT_MODEL", "nvidia/nemotron-3-ultra-550b-a55b:free")
	t.Setenv("CHAT_REVISION", "")
	t.Setenv("CHAT_MAX_TOKENS", "8192")
	t.Setenv("MODEL_REQUEST_TIMEOUT_SECONDS", "300")
	if _, err := FromEnv(); err == nil {
		t.Fatal("accepted OpenRouter without a key")
	}
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	cfg, err := FromEnv()
	if err != nil || cfg.EmbeddingBaseURL != "http://127.0.0.1:1234/v1" || cfg.ChatBaseURL != "https://openrouter.ai/api/v1" || cfg.ChatMaxTokens != 8192 || cfg.AnswerRevision != "openrouter/nvidia/nemotron-3-ultra-550b-a55b:free-unpinned" || New(cfg).HTTP.Timeout != 300*time.Second {
		t.Fatalf("mixed configuration: %+v, %v", cfg, err)
	}
	t.Setenv("EMBEDDING_PROVIDER", "openrouter")
	t.Setenv("EMBEDDING_MODEL", "baai/bge-m3")
	t.Setenv("EMBEDDING_DIMENSIONS", "1024")
	cfg, err = FromEnv()
	if err != nil || cfg.EmbeddingBaseURL != "https://openrouter.ai/api/v1" || cfg.EmbeddingRevision != "openrouter/baai/bge-m3-unpinned" {
		t.Fatalf("OpenRouter-only configuration: %+v, %v", cfg, err)
	}
}

func TestOpenRouterRoutesChatAndEmbeddingsWithoutModelSpecificPrompt(t *testing.T) {
	c := New(Config{EmbeddingProvider: "openrouter", EmbeddingBaseURL: "https://openrouter.ai/api/v1", EmbeddingAPIKey: "test-key", EmbeddingModel: "baai/bge-m3", Dimensions: 2, ChatProvider: "openrouter", ChatBaseURL: "https://openrouter.ai/api/v1", ChatAPIKey: "test-key", AnswerModel: "nvidia/nemotron-3-ultra-550b-a55b:free", ChatMaxTokens: 8192, ChatReasoningEffort: "low"})
	calls := 0
	c.HTTP.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "openrouter.ai" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("request did not use OpenRouter bearer authentication")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if r.URL.Path == "/api/v1/embeddings" {
			if body["input"] != "source passage" || body["model"] != "baai/bge-m3" {
				t.Fatalf("wrong embedding request: %v", body)
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"model":"baai/bge-m3","data":[{"index":0,"embedding":[1,2]}]}`))}, nil
		}
		if r.URL.Path != "/api/v1/chat/completions" || body["model"] != "nvidia/nemotron-3-ultra-550b-a55b:free" || body["max_tokens"] != float64(8192) || body["reasoning"].(map[string]any)["effort"] != "low" {
			t.Fatalf("wrong chat request: %v", body)
		}
		messages := body["messages"].([]any)
		if messages[1].(map[string]any)["content"] != "question" {
			t.Fatalf("model-specific suffix was added: %v", messages[1])
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"model":"nvidia/nemotron-3.5-lightning:provider","choices":[{"message":{"content":"answer"},"finish_reason":"stop"}]}`))}, nil
	})
	if _, err := c.Embed(context.Background(), "search_document: source passage"); err != nil {
		t.Fatal(err)
	}
	answer, err := c.Chat(context.Background(), "system", "question")
	if err != nil || answer != "answer" || calls != 2 {
		t.Fatalf("OpenRouter calls: answer=%q calls=%d err=%v", answer, calls, err)
	}
}

func TestChatRetriesEmptyAndTransientProviderResponses(t *testing.T) {
	c := New(Config{ChatProvider: "openrouter", ChatBaseURL: "https://openrouter.ai/api/v1", ChatAPIKey: "test-key", AnswerModel: "example/chat"})
	calls := 0
	c.HTTP.Transport = roundTrip(func(_ *http.Request) (*http.Response, error) {
		calls++
		switch calls {
		case 1:
			return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"rate limited"}}`))}, nil
		case 2:
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"choices":[]}`))}, nil
		default:
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"READY"},"finish_reason":"stop"}]}`))}, nil
		}
	})
	answer, err := c.Chat(context.Background(), "system", "user")
	if err != nil || answer != "READY" || calls != 3 {
		t.Fatalf("answer=%q calls=%d err=%v", answer, calls, err)
	}
}

func TestEmbedBatchOrdersProviderResultsByIndex(t *testing.T) {
	c := New(Config{EmbeddingProvider: "openrouter", EmbeddingBaseURL: "https://openrouter.ai/api/v1", EmbeddingAPIKey: "test-key", EmbeddingModel: "baai/bge-m3", Dimensions: 2})
	c.HTTP.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		var body struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.Input) != 2 || body.Input[0] != "first" || body.Input[1] != "second" {
			t.Fatalf("batch input=%v", body.Input)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[{"index":1,"embedding":[2,3]},{"index":0,"embedding":[1,4]}]}`))}, nil
	})
	vectors, err := c.EmbedBatch(context.Background(), []string{"search_document: first", "search_document: second"})
	if err != nil || len(vectors) != 2 || vectors[0][0] != 1 || vectors[1][0] != 2 {
		t.Fatalf("batch vectors=%v err=%v", vectors, err)
	}
}

func TestMixedProvidersCheckOnlyLocalModelList(t *testing.T) {
	c := New(Config{EmbeddingProvider: "lmstudio", EmbeddingBaseURL: "http://127.0.0.1:1234/v1", EmbeddingModel: "local-embed", ChatProvider: "openrouter", ChatBaseURL: "https://openrouter.ai/api/v1", AnswerModel: "nvidia/nemotron-3.5-lightning"})
	calls := 0
	c.HTTP.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != "http://127.0.0.1:1234/v1/models" {
			t.Fatalf("unexpected model-list request: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"local-embed"}]}`))}, nil
	})
	models, err := c.Models(context.Background())
	if err != nil || calls != 1 || !models["local-embed"] || !models["nvidia/nemotron-3.5-lightning"] {
		t.Fatalf("models=%v calls=%d err=%v", models, calls, err)
	}
}

func TestGenericOpenAICompatibleEndpoint(t *testing.T) {
	t.Setenv("AI_PROVIDER", "")
	t.Setenv("AI_BASE_URL", "")
	t.Setenv("EMBEDDING_PROVIDER", "openai")
	t.Setenv("EMBEDDING_BASE_URL", "https://embed.example.test/v1")
	t.Setenv("EMBEDDING_API_KEY", "embed-key")
	t.Setenv("EMBEDDING_MODEL", "example/embed")
	t.Setenv("EMBEDDING_REVISION", "")
	t.Setenv("EMBEDDING_DIMENSIONS", "1024")
	t.Setenv("CHAT_PROVIDER", "openai")
	t.Setenv("CHAT_BASE_URL", "https://chat.example.test/v1")
	t.Setenv("CHAT_API_KEY", "chat-key")
	t.Setenv("CHAT_MODEL", "example/chat")
	t.Setenv("CHAT_REVISION", "")
	cfg, err := FromEnv()
	if err != nil || cfg.ChatBaseURL != "https://chat.example.test/v1" || cfg.EmbeddingBaseURL != "https://embed.example.test/v1" || cfg.ChatAPIKey != "chat-key" || cfg.EmbeddingAPIKey != "embed-key" {
		t.Fatalf("generic endpoints: %+v, %v", cfg, err)
	}
}

func TestEmbeddingInputStyleCanOverrideProviderDefault(t *testing.T) {
	local := New(Config{EmbeddingProvider: "lmstudio", EmbeddingInputStyle: "plain"})
	if got := local.EmbeddingInput("search_query: headache"); got != "headache" {
		t.Fatalf("plain local input=%q", got)
	}
	hosted := New(Config{EmbeddingProvider: "openrouter", EmbeddingInputStyle: "prefixed"})
	if got := hosted.EmbeddingInput("search_document: passage"); got != "search_document: passage" {
		t.Fatalf("prefixed hosted input=%q", got)
	}
}

func TestDeepInfraRequests(t *testing.T) {
	c := New(Config{Provider: "deepinfra", BaseURL: "https://api.deepinfra.com/v1/openai", APIKey: "test-key", EmbeddingModel: "BAAI/bge-m3", Dimensions: 2, AnswerModel: "openai/gpt-oss-120b"})
	calls := 0
	c.HTTP.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Authorization") != "Bearer test-key" || r.URL.Host != "api.deepinfra.com" {
			t.Fatalf("wrong hosted request destination or authorization")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if r.URL.Path == "/v1/openai/embeddings" {
			if body["input"] != "source passage" {
				t.Fatalf("wrong embedding input: %v", body["input"])
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"model":"BAAI/bge-m3","data":[{"index":0,"embedding":[1,2]}]}`))}, nil
		}
		if r.URL.Path != "/v1/openai/chat/completions" {
			t.Fatalf("wrong chat path: %s", r.URL.Path)
		}
		messages := body["messages"].([]any)
		if messages[1].(map[string]any)["content"] != "question" {
			t.Fatalf("chat prompt was changed: %v", messages[1])
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"model":"openai/gpt-oss-120b","choices":[{"message":{"content":"answer"},"finish_reason":"stop"}]}`))}, nil
	})
	if _, err := c.Embed(context.Background(), "search_document: source passage"); err != nil {
		t.Fatal(err)
	}
	answer, err := c.Chat(context.Background(), "system", "question")
	if err != nil || answer != "answer" || calls != 2 {
		t.Fatalf("hosted calls: answer=%q calls=%d err=%v", answer, calls, err)
	}
}
