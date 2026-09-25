package connection_health

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ProbeTimeout 是单次真实探活请求的总超时时间。模型检测使用 SSE 流式响应，
// 代码生成模型可能需要超过一分钟才完成，因此不能用短连接超时误判为降级。
const ProbeTimeout = 5 * time.Minute

const defaultProbePrompt = `请生成可直接运行的单文件HTML，使用内联SVG绘制鹈鹕骑自行车的二维循环动画。画面以鹈鹕和自行车为主体，展示清晰的身体结构、踩踏动作和车轮转动，配合协调的背景、配色与层次。动画应流畅自然、衔接连续，并适配不同屏幕尺寸。禁止依赖外部资源，只输出完整HTML，不要代码围栏或解释文字。`
const defaultProbeMaxTokens = 512
const maxProbePreviewBytes = 1024 * 1024

// ProbeRequest 是发起一次真实轻量探活所需的全部参数。UpstreamKey 只用于构造请求凭据，
// 探活结果（ProbeOutcome）绝不回填明文 key。
type ProbeRequest struct {
	BaseURL        string
	UpstreamKey    string
	ProviderFamily string
	ModelName      string
	MaxTokens      int
	ProbePrompt    string
}

// RealProbeRunner 按 provider family 构造最小请求，对上游 AI 端点发起一次流式轻量调用。
// 不经过任何现有请求转发路径，独立的 http.Client，总超时 ProbeTimeout。
type RealProbeRunner struct {
	client *http.Client
}

func NewRealProbeRunner() *RealProbeRunner {
	return &RealProbeRunner{client: &http.Client{Timeout: ProbeTimeout}}
}

// Probe 发起一次真实轻量探活，返回分类后的结果。err 只用于调用方感知调用本身是否被 ctx 取消，
// 正常的上游错误都归类进 ProbeOutcome.Result，不通过 error 返回。
func (r *RealProbeRunner) Probe(ctx context.Context, req ProbeRequest) ProbeOutcome {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultProbeMaxTokens
	}
	prompt := strings.TrimSpace(req.ProbePrompt)
	if prompt == "" {
		prompt = defaultProbePrompt
	}

	httpReq, buildErr := buildProbeRequest(ctx, req, prompt, maxTokens)
	if buildErr != nil {
		return ProbeOutcome{Result: ResultInvalidResponse, Detail: redact(buildErr.Error(), req.UpstreamKey)}
	}

	started := time.Now()
	resp, err := r.client.Do(httpReq)
	if err != nil {
		latencyMs := int(time.Since(started).Milliseconds())
		return ProbeOutcome{Result: classifyTransportError(err), LatencyMs: latencyMs, Detail: redact(err.Error(), req.UpstreamKey)}
	}
	defer resp.Body.Close()

	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		return classifyStreamingResponse(resp, req.UpstreamKey, started, prompt)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	latencyMs := int(time.Since(started).Milliseconds())
	outcome := classifyHTTPResponse(resp.StatusCode, body, req.UpstreamKey, latencyMs, prompt)
	return outcome
}

// buildProbeRequest 统一走 OpenAI 兼容的 /v1/chat/completions 网关端点。
//
// 背景（对应整改任务书第 4 项）：real_connections.upstream_key 和
// upstream_site_id -> upstream_sites.base_url 代表的是已对接的 new-api / sub2api 网关凭据
// 和网关地址（一个可转发 Gemini/Anthropic/OpenAI 等多种模型的中转站点），不是 provider
// 官方凭据。new-api 和 sub2api 都以 OpenAI 兼容协议对外暴露 /v1/chat/completions，
// 内部按 model 名称路由到实际 provider。如果对这些网关直接打 Gemini
// generateContent / Anthropic messages 原生端点，网关大概率不认识这些路径，会导致
// Gemini/Anthropic 模型被系统性误判为失败，进而错误触发自动降级。
// providerFamily 目前只用于在 ModelName 为空时选择一个合理的默认模型名做探活，
// 不再影响实际请求的 endpoint/鉴权方式。
func buildProbeRequest(ctx context.Context, req ProbeRequest, prompt string, maxTokens int) (*http.Request, error) {
	baseURL := strings.TrimRight(req.BaseURL, "/")
	model := req.ModelName
	if model == "" {
		model = defaultModelForProvider(req.ProviderFamily)
	}

	endpoint := baseURL + "/v1/chat/completions"
	payload := map[string]any{
		"model":      model,
		"max_tokens": maxTokens,
		"stream":     true,
		"messages":   []map[string]any{{"role": "user", "content": prompt}},
	}
	headers := map[string]string{
		"Authorization": "Bearer " + req.UpstreamKey,
		"Accept":        "text/event-stream",
		"Cache-Control": "no-cache",
	}
	return newJSONRequest(ctx, http.MethodPost, endpoint, payload, headers)
}

// classifyStreamingResponse 读取 OpenAI-compatible SSE 响应。首字节时间用于区分
// “上游完全没有响应”和“已经开始生成但代码较长”，LatencyMs 始终表示完整响应耗时。
func classifyStreamingResponse(resp *http.Response, upstreamKey string, started time.Time, prompt string) ProbeOutcome {
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	firstByteMs := 0
	var content strings.Builder
	var detail strings.Builder

	for scanner.Scan() {
		if firstByteMs == 0 {
			firstByteMs = int(time.Since(started).Milliseconds())
			if firstByteMs == 0 {
				firstByteMs = 1
			}
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
			Error any `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) > 0 {
			part := chunk.Choices[0].Delta.Content
			if part == "" {
				part = chunk.Choices[0].Message.Content
			}
			content.WriteString(part)
		}
		if chunk.Error != nil {
			detail.WriteString(truncate(data, 500))
		}
	}

	latencyMs := int(time.Since(started).Milliseconds())
	if err := scanner.Err(); err != nil {
		return ProbeOutcome{Result: classifyTransportError(err), FirstByteLatencyMs: firstByteMs, LatencyMs: latencyMs, Detail: redact(truncate(err.Error(), 500), upstreamKey)}
	}
	if detail.Len() > 0 {
		return ProbeOutcome{Result: ResultInvalidResponse, FirstByteLatencyMs: firstByteMs, LatencyMs: latencyMs, Detail: redact(detail.String(), upstreamKey)}
	}

	body := []byte(`{"choices":[{"message":{"content":` + mustJSONString(content.String()) + `}}]}`)
	outcome := classifyHTTPResponse(resp.StatusCode, body, upstreamKey, latencyMs, prompt)
	outcome.FirstByteLatencyMs = firstByteMs
	outcome.GeneratedContent = truncate(content.String(), maxProbePreviewBytes)
	return outcome
}

func mustJSONString(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return `""`
	}
	return string(encoded)
}

func defaultModelForProvider(providerFamily string) string {
	switch providerFamily {
	case ProviderGemini:
		return "gemini-1.5-flash"
	case ProviderAnthropic:
		return "claude-3-haiku-20240307"
	default:
		return "gpt-4o-mini"
	}
}

func newJSONRequest(ctx context.Context, method string, endpoint string, payload any, headers map[string]string) (*http.Request, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}
	return httpReq, nil
}

// classifyTransportError 归类连接层面的错误（超时、DNS、连接被拒等）为网络波动。
func classifyTransportError(err error) ResultKey {
	if errors.Is(err, context.DeadlineExceeded) {
		return ResultNetworkFluctuation
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return ResultNetworkFluctuation
	}
	return ResultNetworkFluctuation
}

// classifyHTTPResponse 按状态码和响应体归类为 7 种错误分类之一，或 ok。
func classifyHTTPResponse(status int, body []byte, upstreamKey string, latencyMs int, prompt string) ProbeOutcome {
	detail := redact(truncate(string(body), 500), upstreamKey)

	switch {
	case status == http.StatusOK || status == http.StatusCreated:
		if !json.Valid(body) {
			return ProbeOutcome{Result: ResultInvalidResponse, LatencyMs: latencyMs, Detail: detail}
		}
		qualityStatus, qualityScore, qualityReason := classifyModelQuality(prompt, body)
		return ProbeOutcome{Result: ResultOK, LatencyMs: latencyMs, Detail: "", QualityStatus: qualityStatus, QualityScore: qualityScore, QualityReason: qualityReason, GeneratedContent: extractProbeContent(body)}

	case status == http.StatusTooManyRequests:
		return ProbeOutcome{Result: ResultRateLimited, LatencyMs: latencyMs, Detail: detail}

	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return ProbeOutcome{Result: ResultAuth, LatencyMs: latencyMs, Detail: detail}

	case status == http.StatusNotFound:
		return ProbeOutcome{Result: ResultModelNotFound, LatencyMs: latencyMs, Detail: detail}

	case status >= 500:
		return ProbeOutcome{Result: ResultServerError, LatencyMs: latencyMs, Detail: detail}

	default:
		// 其余 4xx（参数错误等）无法安全归类为上游不可用，按响应无法解析处理，避免误判暂停。
		return ProbeOutcome{Result: ResultInvalidResponse, LatencyMs: latencyMs, Detail: detail}
	}
}

func extractProbeContent(body []byte) string {
	var payload struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Text string `json:"text"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || len(payload.Choices) == 0 {
		return ""
	}
	content := strings.TrimSpace(payload.Choices[0].Message.Content)
	if content == "" {
		content = strings.TrimSpace(payload.Choices[0].Text)
	}
	return content
}

// classifyModelQuality 对需要生成 HTML/SVG 动画的检测提示词做结构化判定。
// 这里只保存判定结果，不保存模型原文，避免把生成内容或密钥相关信息写入健康状态。
func classifyModelQuality(prompt string, body []byte) (string, int, string) {
	lowerPrompt := strings.ToLower(prompt)
	if !strings.Contains(lowerPrompt, "html") || !strings.Contains(lowerPrompt, "svg") {
		return "unknown", 0, "该探活提示词不是 HTML/SVG 质量检测提示词"
	}

	content := extractProbeContent(body)
	if content == "" {
		return "degraded", 0, "响应缺少可读取的模型内容"
	}
	lower := strings.ToLower(content)
	if strings.Contains(content, "```") {
		return "degraded", 35, "返回了 Markdown 代码围栏"
	}
	if strings.Contains(lower, "无法") || strings.Contains(lower, "不能") || strings.Contains(lower, "抱歉") || strings.Contains(lower, "i can't") || strings.Contains(lower, "i cannot") {
		return "degraded", 20, "模型拒绝生成目标内容"
	}
	// SVG 文档自身通常带有 W3C 命名空间 URL，这不是外部资源，不能因此把完整
	// 的单文件 HTML 判为降级。只检查去掉标准命名空间后仍存在的网络 URL。
	withoutNamespaces := strings.ReplaceAll(strings.ReplaceAll(lower, "http://www.w3.org/2000/svg", ""), "http://www.w3.org/1999/xlink", "")
	if strings.Contains(withoutNamespaces, "http://") || strings.Contains(withoutNamespaces, "https://") || strings.Contains(lower, "<link ") || strings.Contains(lower, "@import") {
		return "degraded", 30, "HTML 依赖外部资源"
	}

	score := 0
	if strings.Contains(lower, "<!doctype html") || strings.Contains(lower, "<html") {
		score += 25
	}
	if strings.Contains(lower, "<svg") {
		score += 30
	}
	if strings.Contains(lower, "@keyframes") || strings.Contains(lower, "<animate") || strings.Contains(lower, "animation:") || strings.Contains(lower, "animation-name") || strings.Contains(lower, "transition:") || strings.Contains(lower, "requestanimationframe") || strings.Contains(lower, "setinterval(") || strings.Contains(lower, "settimeout(") {
		score += 25
	}
	if strings.Contains(lower, "<style") || strings.Contains(lower, "<script") {
		score += 10
	}
	if len(content) >= 300 {
		score += 10
	}
	if score >= 90 {
		return "not_degraded", score, "包含完整 HTML、内联 SVG 和动画结构"
	}
	if score >= 50 {
		return "degraded", score, "只满足部分 HTML/SVG 动画要求"
	}
	return "degraded", score, "缺少 HTML、SVG 或连续动画关键结构"
}

// redact 把探活凭据从错误信息/响应体中裁剪掉，事件和日志绝不落地明文 key。
func redact(s string, key string) string {
	if key == "" {
		return s
	}
	return strings.ReplaceAll(s, key, "***")
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
