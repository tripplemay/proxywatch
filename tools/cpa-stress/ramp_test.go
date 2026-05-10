package main

import (
	"reflect"
	"testing"
)

func TestParseRampEmptyReturnsDefault(t *testing.T) {
	dflt := []int{1, 2, 4, 8, 16, 32, 64}
	got, err := parseRamp("", dflt)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !reflect.DeepEqual(got, dflt) {
		t.Errorf("got %v, want default %v", got, dflt)
	}
}

func TestParseRampSingleValue(t *testing.T) {
	got, err := parseRamp("8", nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !reflect.DeepEqual(got, []int{8}) {
		t.Errorf("got %v, want [8]", got)
	}
}

func TestParseRampList(t *testing.T) {
	got, err := parseRamp("1,2,4,8", nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !reflect.DeepEqual(got, []int{1, 2, 4, 8}) {
		t.Errorf("got %v, want [1 2 4 8]", got)
	}
}

func TestParseRampWithSpaces(t *testing.T) {
	got, err := parseRamp(" 16, 32 , 64", nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !reflect.DeepEqual(got, []int{16, 32, 64}) {
		t.Errorf("got %v, want [16 32 64]", got)
	}
}

func TestParseRampInvalid(t *testing.T) {
	cases := []string{"abc", "0", "-1", "1,abc,2"}
	for _, c := range cases {
		if _, err := parseRamp(c, nil); err == nil {
			t.Errorf("expected error for %q, got nil", c)
		}
	}
}
