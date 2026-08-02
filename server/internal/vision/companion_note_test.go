package vision

import "testing"

func TestSanitizeCompanionNote(t *testing.T) {
	cases := []struct{ in, want string }{
		{"没拍到具体物件", "没看清楚具体物件"},
		{"未看清", "没看清楚"},
	}
	for _, c := range cases {
		got := SanitizeCompanionNote(c.in)
		if got != c.want {
			t.Fatalf("SanitizeCompanionNote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
