package store

import "testing"

func TestKVRoundTrip(t *testing.T) {
	s := newStore(t)
	if err := s.SetKV("foo", "bar"); err != nil {
		t.Fatal(err)
	}
	v, ok, err := s.GetKV("foo")
	if err != nil || !ok || v != "bar" {
		t.Errorf("GetKV foo = %q, %v, %v; want bar, true, nil", v, ok, err)
	}

	if err := s.SetKV("foo", "baz"); err != nil {
		t.Fatal(err)
	}
	v, _, _ = s.GetKV("foo")
	if v != "baz" {
		t.Errorf("after update, foo = %q, want baz", v)
	}

	_, ok, _ = s.GetKV("missing")
	if ok {
		t.Error("missing key should return ok=false")
	}
}

func TestGetKVIntDefault(t *testing.T) {
	s := newStore(t)
	if got := s.GetKVInt("missing", 42); got != 42 {
		t.Errorf("missing key default: got %d, want 42", got)
	}
	s.SetKV("good", "100")
	if got := s.GetKVInt("good", 0); got != 100 {
		t.Errorf("parsed value: got %d, want 100", got)
	}
	s.SetKV("bad", "not-an-int")
	if got := s.GetKVInt("bad", 7); got != 7 {
		t.Errorf("unparseable value falls back to default: got %d, want 7", got)
	}
}
