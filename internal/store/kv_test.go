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
