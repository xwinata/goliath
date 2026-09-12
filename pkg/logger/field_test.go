package logger

import "testing"

func TestF_BuildsField(t *testing.T) {
	f := F("user_id", 123)
	if f.Key != "user_id" {
		t.Errorf("Key = %q, want user_id", f.Key)
	}
	if f.Value != 123 {
		t.Errorf("Value = %v, want 123", f.Value)
	}
}

func TestField_Attr(t *testing.T) {
	a := F("op", "save").attr()
	if a.Key != "op" {
		t.Errorf("attr Key = %q, want op", a.Key)
	}
	if a.Value.Any() != "save" {
		t.Errorf("attr Value = %v, want save", a.Value.Any())
	}
}
