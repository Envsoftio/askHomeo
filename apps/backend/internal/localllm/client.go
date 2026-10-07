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
}

func FromEnv() (Config, error) {
	c := Config{BaseURL: strings.TrimRight(os.Getenv("AI_BASE_URL"), "/"), EmbeddingModel: os.Getenv("EMBEDDING_MODEL"), EmbeddingRevision: os.Getenv("EMBEDDING_REVISION"), AnswerModel: os.Getenv("CHAT_MODEL"), AnswerRevision: os.Getenv("CHAT_REVISION"), Provider: os.Getenv("AI_PROVIDER")}
	if c.Provider == "" {
		c.Provider = "lmstudio"
	}
	n, e := strconv.Atoi(os.Getenv("EMBEDDING_DIMENSIONS"))
	c.Dimensions = n
	if e != nil || n < 1 || n > 4096 {
		return c, errors.New("EMBEDDING_DIMENSIONS must be 1–4096")
	}
	if c.EmbeddingModel == "" || c.EmbeddingRevision == "" || c.AnswerModel == "" || c.AnswerRevision == "" {
		return c, errors.New("EMBEDDING_MODEL, EMBEDDING_REVISION, CHAT_MODEL and CHAT_REVISION are required")
	}
	if c.Provider == "deepinfra" {
		if c.EmbeddingModel != "BAAI/bge-m3" || c.Dimensions != 1024 || c.AnswerModel != "openai/gpt-oss-120b" {
			return c, errors.New("deepinfra requires BAAI/bge-m3 (1024 dimensions) and openai/gpt-oss-120b")
		}
		c.APIKey = os.Getenv("DEEPINFRA_API_KEY")
		if c.APIKey == "" {
			return c, errors.New("DEEPINFRA_API_KEY is required when AI_PROVIDER=deepinfra")
		}
		c.BaseURL = "https://api.deepinfra.com/v1/openai"
		return c, nil
	}
	if c.Provider != "lmstudio" {
		return c, errors.New("AI_PROVIDER must be lmstudio or deepinfra")
	}
	if c.BaseURL == "" {
		return c, errors.New("AI_BASE_URL is required when AI_PROVIDER=lmstudio")
	}
	u, e := url.Parse(c.BaseURL)
	if e != nil || u.Scheme != "http" || u.Host == "" {
		return c, errors.New("AI_BASE_URL must be a local HTTP endpoint")
	}
	host := u.Hostname()
	if host != "localhost" && host != "127.0.0.1" && host != "host.docker.internal" {
		return c, errors.New("AI_BASE_URL must be local")
	}
	return c, nil
}

type Client struct {
	Config Config
	HTTP   *http.Client
}

func New(c Config) *Client { return &Client{Config: c, HTTP: &http.Client{Timeout: 120 * time.Second}} }
func (c *Client) post(ctx context.Context, path string, body any, out any) error {
	b, e := json.Marshal(body)
	if e != nil {
		return e
	}
	req, e := http.NewRequestWithContext(ctx, "POST", c.Config.BaseURL+path, bytes.NewReader(b))
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Config.Provider == "deepinfra" {
		req.Header.Set("Authorization", "Bearer "+c.Config.APIKey)
	}
	res, e := c.HTTP.Do(req)
	if e != nil {
		return fmt.Errorf("model provider unavailable: %w", e)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return fmt.Errorf("model provider returned HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(msg)))
	}
	return json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(out)
}
func (c *Client) Models(ctx context.Context) (map[string]bool, error) {
	if c.Config.Provider == "deepinfra" {
		// DeepInfra does not expose a documented OpenAI-compatible model-list
		// endpoint. The configured models are checked by each actual API call.
		return map[string]bool{c.Config.EmbeddingModel: true, c.Config.AnswerModel: true}, nil
	}
	req, e := http.NewRequestWithContext(ctx, "GET", c.Config.BaseURL+"/models", nil)
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
	text = c.EmbeddingInput(text)
	var v struct {
		Model string `json:"model"`
		Data  []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if e := c.post(ctx, "/embeddings", map[string]any{"model": c.Config.EmbeddingModel, "input": text}, &v); e != nil {
		return nil, e
	}
	if len(v.Data) != 1 || v.Data[0].Index != 0 || len(v.Data[0].Embedding) != c.Config.Dimensions {
		return nil, fmt.Errorf("embedding response has wrong count, index or dimensions")
	}
	if v.Model != "" && v.Model != c.Config.EmbeddingModel {
		return nil, fmt.Errorf("embedding response used model %q", v.Model)
	}
	norm := 0.0
	for _, f := range v.Data[0].Embedding {
		if math.IsNaN(float64(f)) || math.IsInf(float64(f), 0) {
			return nil, errors.New("embedding contains non-finite value")
		}
		norm += float64(f) * float64(f)
	}
	if norm == 0 {
		return nil, errors.New("embedding has zero norm")
	}
	return v.Data[0].Embedding, nil
}
func (c *Client) EmbeddingInput(text string) string {
	if c.Config.Provider == "deepinfra" && c.Config.EmbeddingModel == "BAAI/bge-m3" {
		text = strings.TrimPrefix(text, "search_document: ")
		text = strings.TrimPrefix(text, "search_query: ")
	}
	return text
}
func (c *Client) Chat(ctx context.Context, system, user string) (string, error) {
	var v struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if c.Config.Provider == "deepinfra" {
		body := map[string]any{"model": c.Config.AnswerModel, "max_tokens": 4096, "stream": false, "messages": []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": user}}}
		if e := c.post(ctx, "/chat/completions", body, &v); e != nil {
			return "", e
		}
	} else {
		body := map[string]any{"model": c.Config.AnswerModel, "temperature": 0, "max_tokens": 1500, "stream": false, "messages": []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": user + "\n/no_think"}}}
		if e := c.post(ctx, "/chat/completions", body, &v); e != nil {
			return "", e
		}
	}
	if len(v.Choices) != 1 {
		return "", fmt.Errorf("answer model returned %d choices", len(v.Choices))
	}
	if v.Choices[0].FinishReason != "stop" || strings.TrimSpace(v.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("answer model returned an incomplete response (finish_reason=%q, content_bytes=%d)", v.Choices[0].FinishReason, len(v.Choices[0].Message.Content))
	}
	if v.Model != "" && v.Model != c.Config.AnswerModel {
		return "", fmt.Errorf("answer response used model %q", v.Model)
	}
	return v.Choices[0].Message.Content, nil
}
func Vector(v []float32) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = strconv.FormatFloat(float64(f), 'g', 9, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
