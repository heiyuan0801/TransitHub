package checkin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const embedSessionTTL = 30 * time.Minute
const embedSessionKeyPrefix = "checkin:embed:session:"
const embedWorkspaceIndexKeyPrefix = "checkin:embed:workspace:"

type EmbedSessionStore struct{ client *redis.Client }

func NewEmbedSessionStore(client *redis.Client) *EmbedSessionStore {
	return &EmbedSessionStore{client: client}
}

func (s *EmbedSessionStore) Save(ctx context.Context, token string, session EmbedSession) error {
	payload, err := json.Marshal(session)
	if err != nil {
		return err
	}
	key := embedSessionKey(token)
	index := embedWorkspaceIndexKey(session.UserID, session.AdminAccountID)
	pipe := s.client.TxPipeline()
	pipe.Set(ctx, key, payload, embedSessionTTL)
	pipe.SAdd(ctx, index, key)
	pipe.Expire(ctx, index, embedSessionTTL)
	_, err = pipe.Exec(ctx)
	return err
}

func (s *EmbedSessionStore) Get(ctx context.Context, token string) (*EmbedSession, error) {
	if strings.TrimSpace(token) == "" {
		return nil, nil
	}
	raw, err := s.client.Get(ctx, embedSessionKey(token)).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var session EmbedSession
	if err := json.Unmarshal([]byte(raw), &session); err != nil {
		return nil, err
	}
	return &session, nil
}

func (s *EmbedSessionStore) DeleteWorkspace(ctx context.Context, userID, adminAccountID string) error {
	index := embedWorkspaceIndexKey(userID, adminAccountID)
	keys, err := s.client.SMembers(ctx, index).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return err
	}
	keys = append(keys, index)
	return s.client.Del(ctx, keys...).Err()
}

func embedSessionKey(token string) string { return embedSessionKeyPrefix + strings.TrimSpace(token) }
func embedWorkspaceIndexKey(userID, adminAccountID string) string {
	return embedWorkspaceIndexKeyPrefix + strings.TrimSpace(userID) + ":" + strings.TrimSpace(adminAccountID)
}
