package localllm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
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
	if err != nil || cfg.BaseURL != "https://api.deepinfra.com/v1/openai" {
		t.Fatalf("hosted configuration: %+v, %v", cfg, err)
	}
	t.Setenv("EMBEDDING_DIMENSIONS", "768")
	if _, err := FromEnv(); err == nil {
		t.Fatal("accepted incorrect hosted embedding dimensions")
	}
	t.Setenv("EMBEDDING_DIMENSIONS", "1024")
	t.Setenv("AI_PROVIDER", "lmstudio")
	if _, err := FromEnv(); err == nil {
		t.Fatal("accepted a nonlocal LM Studio endpoint")
	}
}

func TestLMStudioConfigurationWithoutDeepInfraKey(t *testing.T) {
	t.Setenv("AI_PROVIDER", "lmstudio")
	t.Setenv("AI_BASE_URL", "http://127.0.0.1:1234/v1")
	t.Setenv("DEEPINFRA_API_KEY", "")
	t.Setenv("EMBEDDING_MODEL", "text-embedding-nomic-embed-text-v1.5")
	t.Setenv("EMBEDDING_REVISION", "local-nomic-revision")
	t.Setenv("EMBEDDING_DIMENSIONS", "768")
	t.Setenv("CHAT_MODEL", "qwen/qwen3-14b")
	t.Setenv("CHAT_REVISION", "local-qwen-revision")
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("local provider must not require a DeepInfra key: %v", err)
	}
	if cfg.Provider != "lmstudio" || cfg.APIKey != "" || cfg.Dimensions != 768 {
		t.Fatalf("unexpected local configuration: %+v", cfg)
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
