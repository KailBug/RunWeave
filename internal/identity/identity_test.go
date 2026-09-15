package identity

import "testing"

func TestTokensAreDistinctAndRoleBound(t *testing.T) {
	a, _ := NewToken(Principal)
	b, _ := NewToken(Principal)
	if a == b {
		t.Fatal("tokens repeated")
	}
	if digest, err := Digest(a, Principal); err != nil || len(digest) != 32 {
		t.Fatal("bad digest", err)
	}
	for _, token := range []string{"", a + "\n", a[:47], "rw_p_" + a[5:47] + "!"} {
		if _, err := Digest(token, Principal); err == nil {
			t.Fatal("malformed token accepted")
		}
	}
	if _, err := Digest(a, Node); err == nil {
		t.Fatal("role confusion")
	}
	if _, err := NewToken(Role("admin")); err == nil {
		t.Fatal("unknown role accepted")
	}
}
