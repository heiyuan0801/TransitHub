package checkin

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRepositoryLeaderboardPostgres(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	userID, workspaceID := "checkin-test-user-"+suffix, "checkin-test-workspace-"+suffix
	t.Cleanup(func() {
		_, _ = db.Exec(ctx, `DELETE FROM checkin_records WHERE user_id=$1 AND admin_account_id=$2`, userID, workspaceID)
	})

	insert := func(id, remoteUser, email, maskedEmail, date string, streak int, reward float64) {
		t.Helper()
		_, err := db.Exec(ctx, `INSERT INTO checkin_records (id,user_id,admin_account_id,sub2api_user_id,email,masked_email,checkin_date,streak_days,base_reward,milestone_reward,total_reward,reward_status,idempotency_key) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,0,$9,'fulfilled',$10)`, id, userID, workspaceID, remoteUser, email, maskedEmail, date, streak, reward, "idem-"+id)
		if err != nil {
			t.Fatal(err)
		}
	}
	insert("cin-a1-"+suffix, "101", "alpha@example.com", "a***@example.com", "2026-07-13", 1, 0.2)
	insert("cin-a2-"+suffix, "101", "alpha@example.com", "a***@example.com", "2026-07-14", 2, 0.3)
	insert("cin-a3-"+suffix, "101", "alpha@example.com", "a***@example.com", "2026-07-15", 3, 0.4)
	insert("cin-b1-"+suffix, "202", "beta@example.com", "b***@example.com", "2026-07-14", 1, 0.8)
	insert("cin-b2-"+suffix, "202", "beta@example.com", "b***@example.com", "2026-07-15", 2, 0.9)

	repository := NewRepository(db)
	entries, err := repository.ListLeaderboard(ctx, userID, workspaceID, 10, "", "2026-07-15")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Sub2apiUserID != "101" || entries[0].Rank != 1 || entries[0].TotalDays != 3 || entries[0].LatestStreak != 3 {
		t.Fatalf("unexpected leaderboard: %+v", entries)
	}
	todayEntries, err := repository.ListLeaderboard(ctx, userID, workspaceID, 10, "2026-07-15", "2026-07-15")
	if err != nil {
		t.Fatal(err)
	}
	if len(todayEntries) != 2 || todayEntries[0].Email != "beta@example.com" || todayEntries[0].TotalDays != 1 || todayEntries[0].TotalRewards != 0.9 {
		t.Fatalf("unexpected today leaderboard: %+v", todayEntries)
	}
	filtered, total, err := repository.ListWorkspaceRecords(ctx, userID, workspaceID, AdminRecordsQuery{Page: 1, PageSize: 1, DateFrom: "2026-07-14", DateTo: "2026-07-15", UserQuery: "alpha@"})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(filtered) != 1 || filtered[0].Email != "alpha@example.com" || filtered[0].CheckinDate.Format(time.DateOnly) != "2026-07-15" {
		t.Fatalf("unexpected filtered records total=%d items=%+v", total, filtered)
	}
	entry, err := repository.GetLeaderboardEntry(ctx, userID, workspaceID, "202")
	if err != nil {
		t.Fatal(err)
	}
	if entry == nil || entry.Rank != 2 || entry.TotalDays != 2 || entry.TotalRewards != 1.7 {
		t.Fatalf("unexpected user entry: %+v", entry)
	}
	users, checkins, rewards, err := repository.WorkspaceSummary(ctx, userID, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if users != 2 || checkins != 5 || rewards != 2.6 {
		t.Fatalf("unexpected summary users=%d checkins=%d rewards=%v", users, checkins, rewards)
	}
}
