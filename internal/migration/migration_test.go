package migration

import "testing"

func TestSplitStatements(t *testing.T) {
	got := splitStatements(`
-- comment
SET NAMES utf8mb4;
CREATE TABLE users (
  id BIGINT NOT NULL
);
`)
	want := []string{
		"SET NAMES utf8mb4",
		"CREATE TABLE users (\n  id BIGINT NOT NULL\n)",
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("statement[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestMigrationFilesAreOrdered(t *testing.T) {
	for i := 1; i < len(migrations); i++ {
		if migrations[i-1].Version >= migrations[i].Version {
			t.Fatalf("migrations are not ordered: %q >= %q", migrations[i-1].Version, migrations[i].Version)
		}
	}
}
