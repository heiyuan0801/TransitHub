package connection_health

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	defaultEmbedRefreshInterval = 30
	minEmbedRefreshInterval     = 10
	maxEmbedRefreshInterval     = 3600
)

type embedConfigRepository interface {
	GetEmbedHealthConfigByWorkspace(context.Context, string, string) (*EmbedHealthConfig, error)
	GetEmbedHealthConfigByToken(context.Context, string) (*EmbedHealthConfig, error)
	SaveEmbedHealthConfig(context.Context, string, string, EmbedHealthConfig) error
	RotateEmbedHealthToken(context.Context, string, string, string) error
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
	sort.Slice(models, func(i, j int) bool {
		if models[i].GroupName != models[j].GroupName {
			return models[i].GroupName < models[j].GroupName
		}
		if models[i].ConnectionID != models[j].ConnectionID {
			return models[i].ConnectionID < models[j].ConnectionID
		}
		return models[i].ModelName < models[j].ModelName
	})
	return EmbedHealthResponse{GeneratedAt: time.Now(), RefreshIntervalSeconds: normalizeEmbedInterval(config.RefreshIntervalSeconds), Models: models}, nil
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
