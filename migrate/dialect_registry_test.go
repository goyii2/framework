package migrate

import "testing"

func TestDialectNamesReturnsSortedRegistryNames(t *testing.T) {
	names := DialectNames()
	want := []string{"mysql", "postgres", "sqlite", "sqlserver"}
	if len(names) != len(want) {
		t.Fatalf("DialectNames() = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("DialectNames() = %v, want %v", names, want)
		}
	}
}

func TestDialectByNameResolvesEveryRegisteredName(t *testing.T) {
	for _, name := range DialectNames() {
		dialect, ok := DialectByName(name)
		if !ok {
			t.Fatalf("DialectByName(%q) not found", name)
		}
		if dialect.Name() != name {
			t.Fatalf("DialectByName(%q) returned dialect named %q", name, dialect.Name())
		}
	}
}

func TestDialectByNameNormalizesInput(t *testing.T) {
	dialect, ok := DialectByName(" MySQL ")
	if !ok {
		t.Fatal(`DialectByName(" MySQL ") not found`)
	}
	if _, isMySQL := dialect.(MySQLDialect); !isMySQL {
		t.Fatalf("DialectByName(\" MySQL \") = %T, want migrate.MySQLDialect", dialect)
	}
}

func TestDialectByNameRejectsUnknownName(t *testing.T) {
	if _, ok := DialectByName("oracle"); ok {
		t.Fatal(`DialectByName("oracle") found an unregistered dialect`)
	}
}

// Every adapter the conformance matrix walks must be reachable through the
// registry, otherwise the matrix and the registry drift apart.
func TestDialectRegistryCoversConformanceDialects(t *testing.T) {
	for _, d := range conformanceDialects {
		if _, ok := DialectByName(d.Name()); !ok {
			t.Fatalf("dialect %q from the conformance matrix is not registered", d.Name())
		}
	}
}
