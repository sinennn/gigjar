package database

import "testing"

func TestMigrateDriverURL(t *testing.T) {
	cases := map[string]string{
		"postgres://u:p@db:5432/gigjar?sslmode=disable":   "pgx5://u:p@db:5432/gigjar?sslmode=disable",
		"postgresql://u:p@db:5432/gigjar?sslmode=disable": "pgx5://u:p@db:5432/gigjar?sslmode=disable",
		"pgx5://u:p@db/gigjar":                            "pgx5://u:p@db/gigjar",
	}
	for input, want := range cases {
		if got := migrateDriverURL(input); got != want {
			t.Errorf("migrateDriverURL(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestLatestEmbeddedVersionFindsHighestUpMigration(t *testing.T) {
	version, err := LatestEmbeddedVersion()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if version < 1 {
		t.Errorf("latest version = %d, want at least 1", version)
	}
}
