package main

import "testing"

func TestSplitSQLStatements(t *testing.T) {
	got := splitSQL("select 1; select 2;")
	want := []string{"select 1", "select 2"}
	assertSQL(t, got, want)
}

func TestSplitSQLKeepsSemicolonInsideString(t *testing.T) {
	got := splitSQL("select 'a;b';")
	assertSQL(t, got, []string{"select 'a;b'"})
}

func TestSplitSQLKeepsSemicolonInsideQuotedIdentifier(t *testing.T) {
	got := splitSQL(`select "a;b";`)
	assertSQL(t, got, []string{`select "a;b"`})
}

func TestSplitSQLKeepsSemicolonInsideDollarQuotes(t *testing.T) {
	got := splitSQL("select $$a;b$$; select $body$ x; y $body$;")
	assertSQL(t, got, []string{"select $$a;b$$", "select $body$ x; y $body$"})
}

func TestSplitSQLIgnoresComments(t *testing.T) {
	got := splitSQL("select 1; -- tail ;\n/* ; */ select 2;")
	assertSQL(t, got, []string{"select 1", "select 2"})
}

func TestSplitSQLDropsEmptyStatements(t *testing.T) {
	got := splitSQL(" ; \n -- only a comment\n")
	if len(got) != 0 {
		t.Fatalf("got %#v, want no statements", got)
	}
}

func assertSQL(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("statement %d: got %q, want %q", i, got[i], want[i])
		}
	}
}
