package migrate

import (
	"reflect"
	"testing"
)

func TestColumnBuilderImmutable(t *testing.T) {
	m := NewMigrationContext(nil, SQLiteDialect{})
	base := m.String(64)

	notNull := base.NotNull()
	nullable := base.Null()

	if notNull == base || nullable == base {
		t.Fatal("builder methods must return cloned builders")
	}
	if notNull.Nullable() {
		t.Fatal("NotNull builder should not be nullable")
	}
	if !nullable.Nullable() {
		t.Fatal("Null builder should be nullable")
	}
	if !base.Nullable() {
		t.Fatal("base builder should keep its default nullable state")
	}
}

func TestColumnsKeepOrderAndReturnCopy(t *testing.T) {
	m := NewMigrationContext(nil, SQLiteDialect{})
	cols := Columns().
		Add("id", m.PrimaryKey()).
		Add("title", m.String(128)).
		Add("metadata", m.Json())

	items := cols.Items()
	got := []string{items[0].Name, items[1].Name, items[2].Name}
	want := []string{"id", "title", "metadata"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("column order = %#v, want %#v", got, want)
	}

	items[0].Name = "mutated"
	if cols.Items()[0].Name != "id" {
		t.Fatal("Items must return a copy")
	}
}
