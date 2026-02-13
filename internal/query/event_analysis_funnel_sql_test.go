package query

import (
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestBuildFunnelCountsSQL_UsesBigintEpochAndWithinArgs(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	start := time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 2, 11, 0, 0, 0, 0, time.UTC)
	withinSec := int64(24 * 60 * 60)

	sql, args, err := buildFunnelCountsSQL(db, 1, []string{"signup", "checkout", "paid"}, start, end, withinSec, funnelTableSpec{
		Table:       "track_events",
		NameCol:     "name",
		RootFilter:  "distinct_id IS NOT NULL AND distinct_id <> ''",
		AliasFilter: "l.distinct_id IS NOT NULL AND l.distinct_id <> ''",
	})
	if err != nil {
		t.Fatalf("build sql: %v", err)
	}

	if !strings.Contains(sql, "CAST(MIN(") || !strings.Contains(sql, "AS BIGINT") {
		t.Fatalf("expected BIGINT casts in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "CAST(? AS BIGINT)") {
		t.Fatalf("expected within to be cast as BIGINT in SQL, got: %s", sql)
	}

	withinUS := withinSec * 1_000_000
	var found int
	for _, a := range args {
		v, ok := a.(int64)
		if ok && v == withinUS {
			found++
		}
	}
	want := (len([]string{"signup", "checkout", "paid"}) - 1) * 1
	if found != want {
		t.Fatalf("expected withinUS=%d to appear %d times in args, found=%d (args=%#v)", withinUS, want, found, args)
	}
}
