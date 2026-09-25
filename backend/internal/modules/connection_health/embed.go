package connection_health

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	defaultEmbedRefreshInterval = 30
	minEmbedRefreshInterval     = 10
	maxEmbedRefreshInterval     = 3600
	maxEmbedPreviewBytes        = 512 * 1024
)

type embedConfigRepository interface {
	GetEmbedHealthConfigByWorkspace(context.Context, string, string) (*EmbedHealthConfig, error)
	GetEmbedHealthConfigByToken(context.Context, string) (*EmbedHealthConfig, error)
	SaveEmbedHealthConfig(context.Context, string, string, EmbedHealthConfig) error
	RotateEmbedHealthToken(context.Context, string, string, string) error
	ListEmbedHealthLogs(context.Context, string, string, int) ([]EmbedHealthLog, error)
	InsertEmbedHealthLog(context.Context, EmbedHealthLog, string, string) error
	ListEnabledEmbedHealthConfigs(context.Context) ([]EmbedHealthConfig, error)
}

func (s *Service) embedRepository() (embedConfigRepository, error) {
	repo, ok := s.repo.(embedConfigRepository)
	if !ok {
		return nil, errors.New("connection_health: embed repository is not configured")
	}
	return repo, nil
}

func newEmbedToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func normalizeEmbedInterval(value int) int {
	if value < minEmbedRefreshInterval {
		return defaultEmbedRefreshInterval
	}
	if value > maxEmbedRefreshInterval {
		return maxEmbedRefreshInterval
	}
	return value
}

func normalizeAllowedOrigin(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", requestError(ErrorEmbedInvalidOrigin)
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host), nil
}

func normalizeCustomBaseURL(raw string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if strings.HasSuffix(strings.ToLower(raw), "/v1") {
		raw = strings.TrimSuffix(raw, "/v1")
	}
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", requestError(ErrorEmbedCustomConfigInvalid)
	}
	return raw, nil
}

func setEmbedURL(config *EmbedHealthConfig) {
	if config == nil || strings.TrimSpace(config.EmbedToken) == "" {
		return
	}
	config.EmbedURL = "/embed/connection-health?embed_token=" + url.QueryEscape(config.EmbedToken)
}

func (s *Service) GetEmbedHealthConfig(ctx context.Context, userID string) (EmbedHealthConfig, error) {
	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return EmbedHealthConfig{}, err
	}
	repo, err := s.embedRepository()
	if err != nil {
		return EmbedHealthConfig{}, err
	}
	config, err := repo.GetEmbedHealthConfigByWorkspace(ctx, userID, adminAccountID)
	if err != nil {
		return EmbedHealthConfig{}, err
	}
	if config == nil {
		token, tokenErr := newEmbedToken()
		if tokenErr != nil {
			return EmbedHealthConfig{}, tokenErr
		}
		config = &EmbedHealthConfig{EmbedToken: token, Enabled: false, RefreshIntervalSeconds: defaultEmbedRefreshInterval, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		if err := repo.SaveEmbedHealthConfig(ctx, userID, adminAccountID, *config); err != nil {
			return EmbedHealthConfig{}, err
		}
	}
	config.RefreshIntervalSeconds = normalizeEmbedInterval(config.RefreshIntervalSeconds)
	if strings.TrimSpace(config.CustomProviderFamily) == "" {
		config.CustomProviderFamily = ProviderOpenAI
	}
	config.CustomAPIKeyConfigured = strings.TrimSpace(config.CustomAPIKeyCiphertext) != ""
	setEmbedURL(config)
	return *config, nil
}

func (s *Service) UpdateEmbedHealthConfig(ctx context.Context, userID string, input EmbedHealthConfigInput) (EmbedHealthConfig, error) {
	config, err := s.GetEmbedHealthConfig(ctx, userID)
	if err != nil {
		return EmbedHealthConfig{}, err
	}
	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return EmbedHealthConfig{}, err
	}
	repo, err := s.embedRepository()
	if err != nil {
		return EmbedHealthConfig{}, err
	}
	origin, err := normalizeAllowedOrigin(input.AllowedOrigin)
	if err != nil {
		return EmbedHealthConfig{}, err
	}
	config.AllowedOrigin = origin
	baseURL, err := normalizeCustomBaseURL(input.CustomBaseURL)
	if err != nil {
		return EmbedHealthConfig{}, err
	}
	if input.CustomBaseURL != "" {
		config.CustomBaseURL = baseURL
	}
	if input.CustomModel != "" {
		config.CustomModel = strings.TrimSpace(input.CustomModel)
	}
	if input.CustomProviderFamily != "" {
		provider := strings.ToLower(strings.TrimSpace(input.CustomProviderFamily))
		if provider != ProviderOpenAI && provider != ProviderGemini && provider != ProviderAnthropic && provider != ProviderCustom {
			return EmbedHealthConfig{}, requestError(ErrorEmbedCustomConfigInvalid)
		}
		config.CustomProviderFamily = provider
	}
	if strings.TrimSpace(input.CustomAPIKey) != "" {
		ciphertext, encryptErr := s.encryptCustomAPIKey(userID, adminAccountID, strings.TrimSpace(input.CustomAPIKey))
		if encryptErr != nil {
			return EmbedHealthConfig{}, encryptErr
		}
		config.CustomAPIKeyCiphertext = ciphertext
	}
	if input.CustomCheckEnabled != nil {
		config.CustomCheckEnabled = *input.CustomCheckEnabled
	}
	if config.CustomCheckEnabled {
		if strings.TrimSpace(config.CustomBaseURL) == "" || strings.TrimSpace(config.CustomModel) == "" || strings.TrimSpace(config.CustomAPIKeyCiphertext) == "" {
			return EmbedHealthConfig{}, requestError(ErrorEmbedCustomConfigInvalid)
		}
	}
	config.CustomAPIKeyConfigured = strings.TrimSpace(config.CustomAPIKeyCiphertext) != ""
	if input.Enabled != nil {
		config.Enabled = *input.Enabled
	}
	if input.RefreshIntervalSeconds != 0 {
		if input.RefreshIntervalSeconds < minEmbedRefreshInterval || input.RefreshIntervalSeconds > maxEmbedRefreshInterval {
			return EmbedHealthConfig{}, requestError(ErrorEmbedInvalidInterval)
		}
		config.RefreshIntervalSeconds = input.RefreshIntervalSeconds
	}
	config.UpdatedAt = time.Now()
	if err := repo.SaveEmbedHealthConfig(ctx, userID, adminAccountID, config); err != nil {
		return EmbedHealthConfig{}, err
	}
	setEmbedURL(&config)
	return config, nil
}

func (s *Service) RotateEmbedHealthToken(ctx context.Context, userID string) (EmbedHealthConfig, error) {
	config, err := s.GetEmbedHealthConfig(ctx, userID)
	if err != nil {
		return EmbedHealthConfig{}, err
	}
	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return EmbedHealthConfig{}, err
	}
	repo, err := s.embedRepository()
	if err != nil {
		return EmbedHealthConfig{}, err
	}
	token, err := newEmbedToken()
	if err != nil {
		return EmbedHealthConfig{}, err
	}
	if err := repo.RotateEmbedHealthToken(ctx, userID, adminAccountID, token); err != nil {
		return EmbedHealthConfig{}, err
	}
	config.EmbedToken = token
	config.UpdatedAt = time.Now()
	setEmbedURL(&config)
	return config, nil
}

func embedHealthResult(config EmbedHealthConfig, outcome ProbeOutcome, probedAt time.Time) EmbedHealthTestResult {
	var firstByteLatencyMs *int
	if outcome.FirstByteLatencyMs > 0 {
		firstByteLatencyMs = intPtr(outcome.FirstByteLatencyMs)
	}
	return EmbedHealthTestResult{
		ModelName: config.CustomModel, Result: outcome.Result, Healthy: outcome.Result == ResultOK,
		FirstByteLatencyMs: firstByteLatencyMs, LatencyMs: outcome.LatencyMs,
		ErrorKey: string(outcome.Result), ErrorDetail: truncate(outcome.Detail, 500),
		QualityStatus: outcome.QualityStatus, QualityScore: outcome.QualityScore,
		QualityReason: outcome.QualityReason, PreviewHTML: normalizeEmbedPreviewHTML(outcome.GeneratedContent), ProbedAt: probedAt,
	}
}

func normalizeEmbedPreviewHTML(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```") {
		if newline := strings.IndexByte(raw, '\n'); newline >= 0 {
			raw = strings.TrimSpace(raw[newline+1:])
		}
		if strings.HasSuffix(raw, "```") {
			raw = strings.TrimSpace(strings.TrimSuffix(raw, "```"))
		}
	}
	if len(raw) > maxEmbedPreviewBytes {
		raw = raw[:maxEmbedPreviewBytes]
	}
	return raw
}

func embedHealthLogFromResult(result EmbedHealthTestResult) EmbedHealthLog {
	return EmbedHealthLog{
		ID: newEmbedLogID(), ModelName: result.ModelName, Result: result.Result, Healthy: result.Healthy,
		FirstByteLatencyMs: result.FirstByteLatencyMs, LatencyMs: result.LatencyMs,
		ErrorKey: result.ErrorKey, ErrorDetail: truncate(result.ErrorDetail, 500),
		QualityStatus: result.QualityStatus, QualityScore: result.QualityScore,
		QualityReason: result.QualityReason, PreviewHTML: normalizeEmbedPreviewHTML(result.PreviewHTML), ProbedAt: result.ProbedAt,
	}
}

func newEmbedLogID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(buf)
}

func embedModelFromLog(logEntry EmbedHealthLog) EmbedHealthModel {
	state := StateDegraded
	if logEntry.Healthy {
		state = StateHealthy
	}
	return EmbedHealthModel{
		ConnectionID: "custom", GroupName: "custom", ModelName: logEntry.ModelName, State: state,
		FirstByteLatencyMs: logEntry.FirstByteLatencyMs, LatencyMs: intPtr(logEntry.LatencyMs),
		LastProbeAt: &logEntry.ProbedAt, ErrorKey: logEntry.ErrorKey,
		QualityStatus: logEntry.QualityStatus, QualityScore: logEntry.QualityScore, QualityReason: logEntry.QualityReason,
		PreviewHTML: normalizeEmbedPreviewHTML(logEntry.PreviewHTML),
	}
}

func refreshEmbedLogQuality(logEntry *EmbedHealthLog) {
	if logEntry == nil || !logEntry.Healthy || strings.TrimSpace(logEntry.PreviewHTML) == "" {
		return
	}
	status, score, reason := classifyModelContentQuality(logEntry.PreviewHTML)
	logEntry.QualityStatus = status
	logEntry.QualityScore = score
	logEntry.QualityReason = reason
}

func (s *Service) testEmbedConfig(ctx context.Context, config EmbedHealthConfig) (EmbedHealthTestResult, error) {
	if !config.CustomCheckEnabled || !config.CustomAPIKeyConfigured ||
		strings.TrimSpace(config.CustomBaseURL) == "" || strings.TrimSpace(config.CustomModel) == "" {
		return EmbedHealthTestResult{ModelName: config.CustomModel, Result: ResultUnsupported, Healthy: false, ErrorKey: ErrorEmbedCustomConfigInvalid, ProbedAt: time.Now()}, requestError(ErrorEmbedCustomConfigInvalid)
	}
	apiKey, err := s.decryptCustomAPIKey(config)
	if err != nil {
		return EmbedHealthTestResult{ModelName: config.CustomModel, Result: ResultUnsupported, Healthy: false, ErrorKey: err.Error(), ProbedAt: time.Now()}, err
	}
	outcome := s.probeRunner.Probe(ctx, ProbeRequest{
		BaseURL: config.CustomBaseURL, UpstreamKey: apiKey, ProviderFamily: config.CustomProviderFamily,
		ModelName: config.CustomModel, MaxTokens: defaultProbeMaxTokens,
	})
	return embedHealthResult(config, outcome, time.Now()), nil
}

func (s *Service) persistEmbedHealthLog(ctx context.Context, config EmbedHealthConfig, result EmbedHealthTestResult) {
	repo, err := s.embedRepository()
	if err != nil {
		return
	}
	if err := repo.InsertEmbedHealthLog(ctx, embedHealthLogFromResult(result), config.UserID, config.AdminAccountID); err != nil {
		// A failed log write should not hide the actual upstream result from an admin test.
		// The scheduler logs this separately when it is running in the background.
		log.Printf("[connection-health] persist embed health log failed: %v", err)
	}
}

// TestEmbedHealth 执行一次管理员主动发起的自定义嵌入检测，并记录成功或失败日志。
func (s *Service) TestEmbedHealth(ctx context.Context, userID string) (EmbedHealthTestResult, error) {
	config, err := s.GetEmbedHealthConfig(ctx, userID)
	if err != nil {
		return EmbedHealthTestResult{}, err
	}
	result, testErr := s.testEmbedConfig(ctx, config)
	if result.ProbedAt.IsZero() {
		result.ProbedAt = time.Now()
	}
	s.persistEmbedHealthLog(ctx, config, result)
	if testErr != nil {
		return EmbedHealthTestResult{}, testErr
	}
	return result, nil
}

func (s *Service) GetEmbedHealth(ctx context.Context, token string) (EmbedHealthResponse, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return EmbedHealthResponse{}, requestError(ErrorEmbedSessionInvalid)
	}
	repo, err := s.embedRepository()
	if err != nil {
		return EmbedHealthResponse{}, err
	}
	config, err := repo.GetEmbedHealthConfigByToken(ctx, token)
	if err != nil {
		return EmbedHealthResponse{}, err
	}
	if config == nil || !config.Enabled || config.EmbedToken != token {
		return EmbedHealthResponse{}, requestError(ErrorEmbedSessionInvalid)
	}
	states, err := s.repo.ListStatesByWorkspace(ctx, config.UserID, config.AdminAccountID)
	if err != nil {
		return EmbedHealthResponse{}, err
	}
	models := make([]EmbedHealthModel, 0, len(states))
	for _, state := range states {
		if strings.TrimSpace(state.ModelName) == "" || state.ModelName == "*" {
			continue
		}
		groupName := state.OwnGroupName
		if strings.TrimSpace(groupName) == "" {
			groupName = state.UpstreamGroupName
		}
		models = append(models, EmbedHealthModel{
			ConnectionID:  state.ConnectionID,
			GroupName:     groupName,
			ModelName:     state.ModelName,
			State:         state.State,
			LatencyMs:     state.LastLatencyMs,
			LastProbeAt:   state.LastProbeAt,
			ErrorKey:      state.LastErrorKey,
			QualityStatus: state.QualityStatus,
			QualityScore:  state.QualityScore,
			QualityReason: state.QualityReason,
		})
	}
	logs, err := repo.ListEmbedHealthLogs(ctx, config.UserID, config.AdminAccountID, 20)
	if err != nil {
		return EmbedHealthResponse{}, err
	}
	for i := range logs {
		refreshEmbedLogQuality(&logs[i])
	}
	// 历史预览只保留最近 6 次成功结果，避免公开嵌入接口一次性传输过多 HTML；
	// 日志本身仍返回全部最近记录的时间、状态和耗时。
	previewCount := 0
	for i := range logs {
		if !logs[i].Healthy || strings.TrimSpace(logs[i].PreviewHTML) == "" || previewCount >= 6 {
			logs[i].PreviewHTML = ""
			continue
		}
		previewCount++
	}
	if config.CustomCheckEnabled && strings.TrimSpace(config.CustomModel) != "" {
		if len(logs) > 0 {
			models = append(models, embedModelFromLog(logs[0]))
		} else {
			models = append(models, EmbedHealthModel{
				ConnectionID: "custom", GroupName: "custom", ModelName: config.CustomModel,
				State: StateObserving, ErrorKey: "not_probed", QualityStatus: "unknown",
			})
		}
	}
	sort.Slice(models, func(i, j int) bool {
		if models[i].GroupName != models[j].GroupName {
			return models[i].GroupName < models[j].GroupName
		}
		if models[i].ConnectionID != models[j].ConnectionID {
			return models[i].ConnectionID < models[j].ConnectionID
		}
		return models[i].ModelName < models[j].ModelName
	})
	return EmbedHealthResponse{GeneratedAt: time.Now(), RefreshIntervalSeconds: normalizeEmbedInterval(config.RefreshIntervalSeconds), Models: models, Logs: logs}, nil
}

func (s *Service) FrameAncestorOrigin(ctx context.Context, token string) (string, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", false
	}
	repo, err := s.embedRepository()
	if err != nil {
		return "", false
	}
	config, err := repo.GetEmbedHealthConfigByToken(ctx, token)
	if err != nil || config == nil || !config.Enabled || config.EmbedToken != token {
		return "", false
	}
	return config.AllowedOrigin, config.AllowedOrigin != ""
}
