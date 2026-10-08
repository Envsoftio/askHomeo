package localllm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	BaseURL, EmbeddingModel, EmbeddingRevision, AnswerModel, AnswerRevision string
	Dimensions                                                              int
	Provider, APIKey                                                        string
	EmbeddingProvider, EmbeddingBaseURL, EmbeddingAPIKey                    string
	EmbeddingInputStyle                                                     string
	ChatProvider, ChatBaseURL, ChatAPIKey, ChatPromptSuffix                 string
	ChatReasoningEffort                                                     string
	ChatMaxTokens                                                           int
	RequestTimeoutSeconds                                                   int
}

func FromEnv() (Config, error) {
	c := Config{BaseURL: strings.TrimRight(os.Getenv("AI_BASE_URL"), "/"), EmbeddingModel: strings.TrimSpace(os.Getenv("EMBEDDING_MODEL")), EmbeddingRevision: strings.TrimSpace(os.Getenv("EMBEDDING_REVISION")), AnswerModel: strings.TrimSpace(os.Getenv("CHAT_MODEL")), AnswerRevision: strings.TrimSpace(os.Getenv("CHAT_REVISION")), Provider: strings.TrimSpace(os.Getenv("AI_PROVIDER")), ChatPromptSuffix: os.Getenv("CHAT_PROMPT_SUFFIX"), ChatReasoningEffort: strings.TrimSpace(os.Getenv("CHAT_REASONING_EFFORT")), EmbeddingInputStyle: strings.TrimSpace(os.Getenv("EMBEDDING_INPUT_STYLE"))}
	if c.Provider == "" {
		c.Provider = "lmstudio"
	}
	c.EmbeddingProvider = firstNonempty(os.Getenv("EMBEDDING_PROVIDER"), c.Provider)
	c.ChatProvider = firstNonempty(os.Getenv("CHAT_PROVIDER"), c.Provider)
	if c.EmbeddingInputStyle != "" && c.EmbeddingInputStyle != "plain" && c.EmbeddingInputStyle != "prefixed" {
		return c, errors.New("EMBEDDING_INPUT_STYLE must be plain or prefixed")
	}
	if c.ChatReasoningEffort != "" {
		switch c.ChatReasoningEffort {
		case "none", "minimal", "low", "medium", "high", "xhigh", "max":
		default:
			return c, errors.New("CHAT_REASONING_EFFORT must be none, minimal, low, medium, high, xhigh or max")
		}
	}
	n, e := strconv.Atoi(os.Getenv("EMBEDDING_DIMENSIONS"))
	c.Dimensions = n
	if e != nil || n < 1 || n > 4096 {
		return c, errors.New("EMBEDDING_DIMENSIONS must be 1–4096")
	}
	if c.EmbeddingModel == "" || c.AnswerModel == "" {
		return c, errors.New("EMBEDDING_MODEL and CHAT_MODEL are required")
	}
	if c.EmbeddingRevision == "" {
		c.EmbeddingRevision = c.EmbeddingProvider + "/" + c.EmbeddingModel + "-unpinned"
	}
	if c.AnswerRevision == "" {
		c.AnswerRevision = c.ChatProvider + "/" + c.AnswerModel + "-unpinned"
	}
	c.EmbeddingBaseURL, c.EmbeddingAPIKey, e = providerEndpoint(c.EmbeddingProvider, firstNonempty(os.Getenv("EMBEDDING_BASE_URL"), c.BaseURL), os.Getenv("EMBEDDING_API_KEY"))
	if e != nil {
		return c, fmt.Errorf("embedding provider: %w", e)
	}
	c.ChatBaseURL, c.ChatAPIKey, e = providerEndpoint(c.ChatProvider, firstNonempty(os.Getenv("CHAT_BASE_URL"), c.BaseURL), os.Getenv("CHAT_API_KEY"))
	if e != nil {
		return c, fmt.Errorf("chat provider: %w", e)
	}
	if value := strings.TrimSpace(os.Getenv("CHAT_MAX_TOKENS")); value != "" {
		c.ChatMaxTokens, e = strconv.Atoi(value)
		if e != nil || c.ChatMaxTokens < 1 || c.ChatMaxTokens > 65536 {
			return c, errors.New("CHAT_MAX_TOKENS must be 1–65536")
		}
	}
	if value := strings.TrimSpace(os.Getenv("MODEL_REQUEST_TIMEOUT_SECONDS")); value != "" {
		c.RequestTimeoutSeconds, e = strconv.Atoi(value)
		if e != nil || c.RequestTimeoutSeconds < 1 || c.RequestTimeoutSeconds > 600 {
			return c, errors.New("MODEL_REQUEST_TIMEOUT_SECONDS must be 1–600")
		}
	}
	return c, nil
}

func firstNonempty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func providerEndpoint(provider, baseURL, key string) (string, string, error) {
	switch provider {
	case "openrouter":
		key = firstNonempty(os.Getenv("OPENROUTER_API_KEY"), key)
		if key == "" {
			return "", "", errors.New("OPENROUTER_API_KEY is required")
		}
		return "https://openrouter.ai/api/v1", key, nil
	case "deepinfra":
		key = firstNonempty(os.Getenv("DEEPINFRA_API_KEY"), key)
		if key == "" {
			return "", "", errors.New("DEEPINFRA_API_KEY is required")
		}
		return "https://api.deepinfra.com/v1/openai", key, nil
	case "lmstudio", "openai":
		baseURL = strings.TrimRight(baseURL, "/")
		u, err := url.Parse(baseURL)
		if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return "", "", errors.New("a valid base URL is required")
		}
		local := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "host.docker.internal"
		if provider == "lmstudio" && (!local || u.Scheme != "http") {
			return "", "", errors.New("LM Studio requires a local HTTP base URL")
		}
		if provider == "openai" && u.Scheme != "https" && !(local && u.Scheme == "http") {
			return "", "", errors.New("OpenAI-compatible endpoints require HTTPS or local HTTP")
		}
		return baseURL, key, nil
	default:
		return "", "", fmt.Errorf("unsupported provider %q (use lmstudio, openrouter, deepinfra, or openai)", provider)
	}
}

func (c Config) embeddingEndpoint() (string, string, string) {
	if c.EmbeddingProvider != "" {
		return c.EmbeddingProvider, c.EmbeddingBaseURL, c.EmbeddingAPIKey
	}
	return c.Provider, c.BaseURL, c.APIKey
}

func (c Config) chatEndpoint() (string, string, string) {
	if c.ChatProvider != "" {
		return c.ChatProvider, c.ChatBaseURL, c.ChatAPIKey
	}
	return c.Provider, c.BaseURL, c.APIKey
}

type Client struct {
	Config Config
	HTTP   *http.Client
}

func New(c Config) *Client {
	timeout := 120 * time.Second
	if c.RequestTimeoutSeconds > 0 {
		timeout = time.Duration(c.RequestTimeoutSeconds) * time.Second
	}
	return &Client{Config: c, HTTP: &http.Client{Timeout: timeout}}
}
func (c *Client) post(ctx context.Context, baseURL, key, path string, body any, out any) error {
	b, e := json.Marshal(body)
	if e != nil {
		return e
	}
	req, e := http.NewRequestWithContext(ctx, "POST", baseURL+path, bytes.NewReader(b))
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	res, e := c.HTTP.Do(req)
	if e != nil {
		return fmt.Errorf("model provider unavailable: %w", e)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return providerHTTPError{StatusCode: res.StatusCode, Message: strings.TrimSpace(string(msg))}
	}
	return json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(out)
}

type providerHTTPError struct {
	StatusCode int
	Message    string
}

func (e providerHTTPError) Error() string {
	return fmt.Sprintf("model provider returned HTTP %d: %s", e.StatusCode, e.Message)
}
func (c *Client) Models(ctx context.Context) (map[string]bool, error) {
	models := map[string]bool{}
	for _, endpoint := range []struct{ provider, baseURL, model string }{{c.Config.EmbeddingProvider, c.Config.EmbeddingBaseURL, c.Config.EmbeddingModel}, {c.Config.ChatProvider, c.Config.ChatBaseURL, c.Config.AnswerModel}} {
		if endpoint.provider == "" {
			endpoint.provider, endpoint.baseURL = c.Config.Provider, c.Config.BaseURL
		}
		if endpoint.provider != "lmstudio" {
			// Hosted providers validate model availability on the actual request.
			models[endpoint.model] = true
			continue
		}
		loaded, err := c.localModels(ctx, endpoint.baseURL)
		if err != nil {
			return nil, err
		}
		models[endpoint.model] = loaded[endpoint.model]
	}
	return models, nil
}

func (c *Client) localModels(ctx context.Context, baseURL string) (map[string]bool, error) {
	req, e := http.NewRequestWithContext(ctx, "GET", baseURL+"/models", nil)
	if e != nil {
		return nil, e
	}
	res, e := c.HTTP.Do(req)
	if e != nil {
		return nil, fmt.Errorf("local model unavailable: %w", e)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("model list returned HTTP %d", res.StatusCode)
	}
	var v struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if e = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&v); e != nil {
		return nil, e
	}
	m := map[string]bool{}
	for _, x := range v.Data {
		m[x.ID] = true
	}
	return m, nil
}
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	items, err := c.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return items[0], nil
}
func (c *Client) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, errors.New("embedding input is empty")
	}
	inputs := make([]string, len(texts))
	for i, input := range texts {
		inputs[i] = c.EmbeddingInput(input)
	}
	provider, baseURL, key := c.Config.embeddingEndpoint()
	var v struct {
		Model string `json:"model"`
		Data  []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	var input any = inputs
	if len(inputs) == 1 {
		input = inputs[0]
	}
	if e := c.post(ctx, baseURL, key, "/embeddings", map[string]any{"model": c.Config.EmbeddingModel, "input": input}, &v); e != nil {
		return nil, e
	}
	if len(v.Data) != len(inputs) {
		return nil, fmt.Errorf("embedding response has wrong count")
	}
	if provider == "lmstudio" && v.Model != "" && v.Model != c.Config.EmbeddingModel {
		return nil, fmt.Errorf("embedding response used model %q", v.Model)
	}
	results := make([][]float32, len(inputs))
	for _, item := range v.Data {
		if item.Index < 0 || item.Index >= len(results) || results[item.Index] != nil || len(item.Embedding) != c.Config.Dimensions {
			return nil, fmt.Errorf("embedding response has wrong index or dimensions")
		}
		norm := 0.0
		for _, f := range item.Embedding {
			if math.IsNaN(float64(f)) || math.IsInf(float64(f), 0) {
				return nil, errors.New("embedding contains non-finite value")
			}
			norm += float64(f) * float64(f)
		}
		if norm == 0 {
			return nil, errors.New("embedding has zero norm")
		}
		results[item.Index] = item.Embedding
	}
	return results, nil
}
func (c *Client) EmbeddingInput(text string) string {
	provider, _, _ := c.Config.embeddingEndpoint()
	style := c.Config.EmbeddingInputStyle
	if style == "" && provider != "lmstudio" {
		style = "plain"
	}
	if style == "plain" {
		text = strings.TrimPrefix(text, "search_document: ")
		text = strings.TrimPrefix(text, "search_query: ")
	}
	return text
}
func (c *Client) Chat(ctx context.Context, system, user string) (string, error) {
	provider, baseURL, key := c.Config.chatEndpoint()
	type chatResponse struct {
		Model string `json:"model"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	maxTokens := c.Config.ChatMaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
		if provider == "lmstudio" {
			maxTokens = 1500
		}
	}
	body := map[string]any{"model": c.Config.AnswerModel, "max_tokens": maxTokens, "stream": false, "messages": []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": user + c.Config.ChatPromptSuffix}}}
	if provider == "lmstudio" {
		body["temperature"] = 0
	}
	if c.Config.ChatReasoningEffort != "" {
		body["reasoning"] = map[string]string{"effort": c.Config.ChatReasoningEffort}
	}
	for attempt := 0; attempt < 5; attempt++ {
		var v chatResponse
		err := c.post(ctx, baseURL, key, "/chat/completions", body, &v)
		retry := false
		if err == nil {
			switch {
			case len(v.Choices) == 0:
				err = fmt.Errorf("answer model returned no choices: %s", v.Error.Message)
				retry = true
			case len(v.Choices) != 1:
				err = fmt.Errorf("answer model returned %d choices", len(v.Choices))
			case v.Choices[0].FinishReason != "stop" || strings.TrimSpace(v.Choices[0].Message.Content) == "":
				err = fmt.Errorf("answer model returned an incomplete response (finish_reason=%q, content_bytes=%d)", v.Choices[0].FinishReason, len(v.Choices[0].Message.Content))
			case provider == "lmstudio" && v.Model != "" && v.Model != c.Config.AnswerModel:
				err = fmt.Errorf("answer response used model %q", v.Model)
			default:
				return v.Choices[0].Message.Content, nil
			}
		} else {
			var providerError providerHTTPError
			retry = errors.As(err, &providerError) && (providerError.StatusCode == 429 || providerError.StatusCode >= 500)
		}
		if !retry || attempt == 4 {
			return "", err
		}
		wait := time.NewTimer(time.Duration(1<<(attempt+1)) * time.Second)
		select {
		case <-ctx.Done():
			wait.Stop()
			return "", ctx.Err()
		case <-wait.C:
		}
	}
	return "", errors.New("answer model retry limit exceeded")
}
func Vector(v []float32) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = strconv.FormatFloat(float64(f), 'g', 9, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
