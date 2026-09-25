package migrations

import (
	"strings"
	"testing"
)

func TestStrategyModeMigrationAddsPriorityModeBeforeBackfill(t *testing.T) {
	sqlBytes, err := migrationFiles.ReadFile("000018_connection_health_strategy_mode.sql")
	if err != nil {
		t.Fatalf("read strategy mode migration: %v", err)
	}

	sql := string(sqlBytes)
	addColumn := strings.Index(sql, "ADD COLUMN IF NOT EXISTS priority_mode")
	backfill := strings.Index(sql, "WHERE policy.priority_mode")
	if addColumn < 0 {
		t.Fatal("strategy mode migration must add priority_mode for legacy installations")
	}
	if backfill < 0 {
		t.Fatal("strategy mode migration must retain the priority_mode compatibility backfill")
	}
	if addColumn > backfill {
		t.Fatal("strategy mode migration must add priority_mode before using it in the backfill")
	}
}
