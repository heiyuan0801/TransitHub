package checkin

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) EnsureSchema(ctx context.Context) error {
	_, err := r.db.Exec(ctx, `SELECT 1 FROM checkin_configs LIMIT 0`)
	return err
}

func (r *Repository) GetConfigByWorkspace(ctx context.Context, userID, adminAccountID string) (*Config, error) {
	config, err := scanConfig(r.db.QueryRow(ctx, `SELECT user_id,admin_account_id,embed_token,sub2api_source_origin,enabled,daily_min,daily_max,daily_user_reward_cap,timezone,created_at,updated_at FROM checkin_configs WHERE user_id=$1 AND admin_account_id=$2`, userID, adminAccountID))
	if err != nil || config == nil {
		return config, err
	}
	config.Milestones, err = r.listMilestones(ctx, userID, adminAccountID)
	return config, err
}

func (r *Repository) GetConfigByToken(ctx context.Context, token string) (*Config, error) {
	config, err := scanConfig(r.db.QueryRow(ctx, `SELECT user_id,admin_account_id,embed_token,sub2api_source_origin,enabled,daily_min,daily_max,daily_user_reward_cap,timezone,created_at,updated_at FROM checkin_configs WHERE embed_token=$1`, token))
	if err != nil || config == nil {
		return config, err
	}
	config.Milestones, err = r.listMilestones(ctx, config.UserID, config.AdminAccountID)
	return config, err
}

func (r *Repository) InsertConfig(ctx context.Context, config Config) error {
	_, err := r.db.Exec(ctx, `INSERT INTO checkin_configs (user_id,admin_account_id,embed_token,sub2api_source_origin,enabled,daily_min,daily_max,daily_user_reward_cap,timezone) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT (user_id,admin_account_id) DO NOTHING`, config.UserID, config.AdminAccountID, config.EmbedToken, config.Sub2apiSourceOrigin, config.Enabled, config.DailyMin, config.DailyMax, config.DailyUserRewardCap, config.Timezone)
	return err
}

func (r *Repository) SaveConfig(ctx context.Context, config Config) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE checkin_configs SET sub2api_source_origin=$3,enabled=$4,daily_min=$5,daily_max=$6,daily_user_reward_cap=$7,timezone=$8,updated_at=now() WHERE user_id=$1 AND admin_account_id=$2`, config.UserID, config.AdminAccountID, config.Sub2apiSourceOrigin, config.Enabled, config.DailyMin, config.DailyMax, config.DailyUserRewardCap, config.Timezone)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return requestError(ErrorValidation)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM checkin_milestones WHERE user_id=$1 AND admin_account_id=$2`, config.UserID, config.AdminAccountID); err != nil {
		return err
	}
	for _, milestone := range config.Milestones {
		if _, err := tx.Exec(ctx, `INSERT INTO checkin_milestones (user_id,admin_account_id,days,bonus_amount) VALUES ($1,$2,$3,$4)`, config.UserID, config.AdminAccountID, milestone.Days, milestone.BonusAmount); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *Repository) RotateEmbedToken(ctx context.Context, userID, adminAccountID, token string) error {
	_, err := r.db.Exec(ctx, `UPDATE checkin_configs SET embed_token=$3,updated_at=now() WHERE user_id=$1 AND admin_account_id=$2`, userID, adminAccountID, token)
	return err
}

func (r *Repository) UpdateSourceOrigin(ctx context.Context, userID, adminAccountID, origin string) error {
	_, err := r.db.Exec(ctx, `UPDATE checkin_configs SET sub2api_source_origin=$3,updated_at=now() WHERE user_id=$1 AND admin_account_id=$2`, userID, adminAccountID, origin)
	return err
}

func (r *Repository) GetRecordForDate(ctx context.Context, userID, adminAccountID, sub2apiUserID string, date time.Time) (*Record, error) {
	return scanRecord(r.db.QueryRow(ctx, `SELECT `+recordColumns+` FROM checkin_records WHERE user_id=$1 AND admin_account_id=$2 AND sub2api_user_id=$3 AND checkin_date=$4`, userID, adminAccountID, sub2apiUserID, date))
}

func (r *Repository) GetLatestRecord(ctx context.Context, userID, adminAccountID, sub2apiUserID string) (*Record, error) {
	return scanRecord(r.db.QueryRow(ctx, `SELECT `+recordColumns+` FROM checkin_records WHERE user_id=$1 AND admin_account_id=$2 AND sub2api_user_id=$3 AND reward_status='fulfilled' ORDER BY checkin_date DESC LIMIT 1`, userID, adminAccountID, sub2apiUserID))
}

func (r *Repository) InsertRecord(ctx context.Context, record Record) (*Record, bool, error) {
	_, err := r.db.Exec(ctx, `INSERT INTO checkin_records (id,user_id,admin_account_id,sub2api_user_id,email,masked_email,checkin_date,streak_days,base_reward,milestone_reward,total_reward,reward_status,idempotency_key) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, record.ID, record.UserID, record.AdminAccountID, record.Sub2apiUserID, record.Email, record.MaskedEmail, record.CheckinDate, record.StreakDays, record.BaseReward, record.MilestoneReward, record.TotalReward, record.RewardStatus, record.IdempotencyKey)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			existing, getErr := r.GetRecordForDate(ctx, record.UserID, record.AdminAccountID, record.Sub2apiUserID, record.CheckinDate)
			return existing, false, getErr
		}
		return nil, false, err
	}
	created, err := r.GetRecordForDate(ctx, record.UserID, record.AdminAccountID, record.Sub2apiUserID, record.CheckinDate)
	return created, true, err
}

func (r *Repository) MarkReward(ctx context.Context, id, status, remoteRef, errorKey, detail string) error {
	_, err := r.db.Exec(ctx, `UPDATE checkin_records SET reward_status=$2,attempt_count=attempt_count+1,remote_reference=$3,error_key=$4,error_detail=$5,updated_at=now(),fulfilled_at=CASE WHEN $2='fulfilled' THEN now() ELSE fulfilled_at END WHERE id=$1`, id, status, remoteRef, errorKey, detail)
	return err
}

func (r *Repository) ListUserRecords(ctx context.Context, userID, adminAccountID, sub2apiUserID string, limit int) ([]Record, error) {
	rows, err := r.db.Query(ctx, `SELECT `+recordColumns+` FROM checkin_records WHERE user_id=$1 AND admin_account_id=$2 AND sub2api_user_id=$3 AND reward_status='fulfilled' ORDER BY checkin_date DESC LIMIT $4`, userID, adminAccountID, sub2apiUserID, limit)
	return scanRecordRows(rows, err)
}

func (r *Repository) ListWorkspaceRecords(ctx context.Context, userID, adminAccountID string, query AdminRecordsQuery) ([]Record, int, error) {
	args := []any{userID, adminAccountID, query.DateFrom, query.DateTo, strings.TrimSpace(query.UserQuery)}
	where := ` WHERE user_id=$1 AND admin_account_id=$2
		AND (NULLIF($3,'')::date IS NULL OR checkin_date >= NULLIF($3,'')::date)
		AND (NULLIF($4,'')::date IS NULL OR checkin_date <= NULLIF($4,'')::date)
		AND ($5='' OR sub2api_user_id ILIKE '%' || $5 || '%' OR email ILIKE '%' || $5 || '%' OR masked_email ILIKE '%' || $5 || '%')`
	var total int
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM checkin_records`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, query.PageSize, (query.Page-1)*query.PageSize)
	rows, err := r.db.Query(ctx, `SELECT `+recordColumns+` FROM checkin_records`+where+` ORDER BY created_at DESC,id DESC LIMIT $6 OFFSET $7`, args...)
	records, err := scanRecordRows(rows, err)
	return records, total, err
}

func (r *Repository) TodaySummary(ctx context.Context, userID, adminAccountID string, date time.Time) (int, float64, error) {
	var count int
	var rewards float64
	err := r.db.QueryRow(ctx, `SELECT count(*) FILTER (WHERE reward_status='fulfilled'),COALESCE(sum(total_reward) FILTER (WHERE reward_status='fulfilled'),0) FROM checkin_records WHERE user_id=$1 AND admin_account_id=$2 AND checkin_date=$3`, userID, adminAccountID, date).Scan(&count, &rewards)
	return count, rewards, err
}

func (r *Repository) WorkspaceSummary(ctx context.Context, userID, adminAccountID string) (int, int, float64, error) {
	var users int
	var checkins int
	var rewards float64
	err := r.db.QueryRow(ctx, `SELECT count(DISTINCT sub2api_user_id),count(*),COALESCE(sum(total_reward),0) FROM checkin_records WHERE user_id=$1 AND admin_account_id=$2 AND reward_status='fulfilled'`, userID, adminAccountID).Scan(&users, &checkins, &rewards)
	return users, checkins, rewards, err
}

func (r *Repository) ListLeaderboard(ctx context.Context, userID, adminAccountID string, limit int, dateFrom, dateTo string) ([]LeaderboardEntry, error) {
	rows, err := r.db.Query(ctx, leaderboardQuery+` ORDER BY rank,sub2api_user_id LIMIT $5`, userID, adminAccountID, dateFrom, dateTo, limit)
	return scanLeaderboardRows(rows, err)
}

func (r *Repository) GetLeaderboardEntry(ctx context.Context, userID, adminAccountID, sub2apiUserID string) (*LeaderboardEntry, error) {
	return scanLeaderboardEntry(r.db.QueryRow(ctx, leaderboardQuery+` WHERE sub2api_user_id=$5`, userID, adminAccountID, "", "", sub2apiUserID))
}

func (r *Repository) listMilestones(ctx context.Context, userID, adminAccountID string) ([]Milestone, error) {
	rows, err := r.db.Query(ctx, `SELECT days,bonus_amount FROM checkin_milestones WHERE user_id=$1 AND admin_account_id=$2 ORDER BY days`, userID, adminAccountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Milestone{}
	for rows.Next() {
		var item Milestone
		if err := rows.Scan(&item.Days, &item.BonusAmount); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

const recordColumns = `id,user_id,admin_account_id,sub2api_user_id,email,masked_email,checkin_date,streak_days,base_reward,milestone_reward,total_reward,reward_status,attempt_count,idempotency_key,remote_reference,error_key,error_detail,created_at,updated_at,fulfilled_at`

const leaderboardQuery = `WITH totals AS (
	SELECT sub2api_user_id,max(email) AS email,max(masked_email) AS masked_email,count(*)::int AS total_days,
		(array_agg(streak_days ORDER BY checkin_date DESC))[1]::int AS latest_streak,
		max(streak_days)::int AS longest_streak,sum(total_reward)::float8 AS total_rewards,max(checkin_date) AS last_checkin_date
	FROM checkin_records
	WHERE user_id=$1 AND admin_account_id=$2 AND reward_status='fulfilled'
		AND (NULLIF($3,'')::date IS NULL OR checkin_date >= NULLIF($3,'')::date)
		AND (NULLIF($4,'')::date IS NULL OR checkin_date <= NULLIF($4,'')::date)
	GROUP BY sub2api_user_id
), ranked AS (
	SELECT rank() OVER (ORDER BY total_days DESC,total_rewards DESC,last_checkin_date ASC,sub2api_user_id ASC)::int AS rank,
		sub2api_user_id,email,masked_email,total_days,latest_streak,longest_streak,total_rewards,last_checkin_date
	FROM totals
)
SELECT rank,sub2api_user_id,email,masked_email,total_days,latest_streak,longest_streak,total_rewards,last_checkin_date FROM ranked`

type scanner interface{ Scan(dest ...any) error }

func scanConfig(row scanner) (*Config, error) {
	var config Config
	err := row.Scan(&config.UserID, &config.AdminAccountID, &config.EmbedToken, &config.Sub2apiSourceOrigin, &config.Enabled, &config.DailyMin, &config.DailyMax, &config.DailyUserRewardCap, &config.Timezone, &config.CreatedAt, &config.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &config, err
}

func scanRecord(row scanner) (*Record, error) {
	var record Record
	err := row.Scan(&record.ID, &record.UserID, &record.AdminAccountID, &record.Sub2apiUserID, &record.Email, &record.MaskedEmail, &record.CheckinDate, &record.StreakDays, &record.BaseReward, &record.MilestoneReward, &record.TotalReward, &record.RewardStatus, &record.AttemptCount, &record.IdempotencyKey, &record.RemoteReference, &record.ErrorKey, &record.ErrorDetail, &record.CreatedAt, &record.UpdatedAt, &record.FulfilledAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &record, err
}

func scanRecordRows(rows pgx.Rows, err error) ([]Record, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Record{}
	for rows.Next() {
		item, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func scanLeaderboardEntry(row scanner) (*LeaderboardEntry, error) {
	var entry LeaderboardEntry
	err := row.Scan(&entry.Rank, &entry.Sub2apiUserID, &entry.Email, &entry.MaskedEmail, &entry.TotalDays, &entry.LatestStreak, &entry.LongestStreak, &entry.TotalRewards, &entry.LastCheckinDate)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &entry, err
}

func scanLeaderboardRows(rows pgx.Rows, err error) ([]LeaderboardEntry, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []LeaderboardEntry{}
	for rows.Next() {
		item, err := scanLeaderboardEntry(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}
