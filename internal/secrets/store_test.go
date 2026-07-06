package secrets

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateKey(t *testing.T) {
	cases := []struct {
		key string
		ok  bool
	}{
		{"APP_SECRET", true},
		{"STRIPE_API_KEY", true},
		{"A", true},
		{"app_secret", false},
		{"1KEY", false},
		{"KEY-WITH-DASH", false},
		{"", false},
		{"KEY WITH SPACE", false},
	}
	for _, c := range cases {
		err := ValidateKey(c.key)
		if c.ok && err != nil {
			t.Errorf("ValidateKey(%q) = %v, want nil", c.key, err)
		}
		if !c.ok && err == nil {
			t.Errorf("ValidateKey(%q) = nil, want error", c.key)
		}
	}
}

func TestStore_SetGet(t *testing.T) {
	root := t.TempDir()
	s := New(root)

	if err := s.Set("APP_SECRET", "hunter2"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := s.Get("APP_SECRET")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "hunter2" {
		t.Errorf("Get() = %q, want %q", got, "hunter2")
	}

	// Persisted with 0600 permissions, not group/world readable.
	info, err := os.Stat(filepath.Join(root, ".acthur", "secrets", "kv", "APP_SECRET"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("file mode = %o, want 0600", perm)
	}
}

func TestStore_Set_InvalidKey(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Set("bad key", "x"); err == nil {
		t.Fatal("expected error for invalid key")
	}
}

func TestStore_Get_NotSet(t *testing.T) {
	s := New(t.TempDir())
	_, err := s.Get("MISSING")
	if err == nil {
		t.Fatal("expected error for unset key")
	}
}

func TestStore_List(t *testing.T) {
	root := t.TempDir()
	s := New(root)

	keys, err := s.List()
	if err != nil {
		t.Fatalf("List on empty store: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("List on empty store = %v, want empty", keys)
	}

	_ = s.Set("B_KEY", "1")
	_ = s.Set("A_KEY", "2")

	keys, err = s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"A_KEY", "B_KEY"}
	if len(keys) != len(want) || keys[0] != want[0] || keys[1] != want[1] {
		t.Errorf("List() = %v, want %v (sorted)", keys, want)
	}
}

func TestStore_Remove(t *testing.T) {
	s := New(t.TempDir())
	_ = s.Set("KEY", "v")

	if err := s.Remove("KEY"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if s.Has("KEY") {
		t.Error("Has() = true after Remove")
	}
	if err := s.Remove("KEY"); err == nil {
		t.Fatal("expected error removing already-removed key")
	}
}

func TestStore_All(t *testing.T) {
	root := t.TempDir()
	s := New(root)

	all, err := s.All()
	if err != nil {
		t.Fatalf("All on empty store: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("All on empty store = %v, want empty", all)
	}

	_ = s.Set("ONE", "1")
	_ = s.Set("TWO", "2")

	all, err = s.All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if all["ONE"] != "1" || all["TWO"] != "2" || len(all) != 2 {
		t.Errorf("All() = %v", all)
	}
}

func TestStore_Rotate(t *testing.T) {
	s := New(t.TempDir())

	// Rotating an unset key is an error — rotate refreshes, it doesn't create.
	_, err := s.Rotate("KEY", nil)
	if err == nil {
		t.Fatal("expected error rotating an unset key")
	}

	_ = s.Set("KEY", "old-value")

	calls := 0
	fixedGen := func() (string, error) {
		calls++
		return "new-value", nil
	}
	got, err := s.Rotate("KEY", fixedGen)
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if got != "new-value" {
		t.Errorf("Rotate() = %q, want %q", got, "new-value")
	}
	if calls != 1 {
		t.Errorf("generate called %d times, want 1", calls)
	}
	stored, _ := s.Get("KEY")
	if stored != "new-value" {
		t.Errorf("stored value = %q, want %q", stored, "new-value")
	}
}

func TestStore_Rotate_GenerateError(t *testing.T) {
	s := New(t.TempDir())
	_ = s.Set("KEY", "v")

	wantErr := errors.New("boom")
	_, err := s.Rotate("KEY", func() (string, error) { return "", wantErr })
	if !errors.Is(err, wantErr) {
		t.Errorf("Rotate error = %v, want wrapping %v", err, wantErr)
	}
}

func TestRandomValue(t *testing.T) {
	a, err := RandomValue()
	if err != nil {
		t.Fatalf("RandomValue: %v", err)
	}
	b, err := RandomValue()
	if err != nil {
		t.Fatalf("RandomValue: %v", err)
	}
	if a == b {
		t.Error("RandomValue() returned the same value twice — not random")
	}
	if len(a) != 64 { // 32 bytes hex-encoded
		t.Errorf("len(RandomValue()) = %d, want 64", len(a))
	}
}
