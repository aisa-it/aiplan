package types

import (
	"encoding/json"
	"testing"
)

func TestHideFields_MarshalJSON_Nil(t *testing.T) {
	var hf HideFields

	bytes, err := json.Marshal(hf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	res := string(bytes)

	if res != "[]" {
		t.Fatalf("expected [], got %s", string(bytes))
	}
}

func TestHideFields_MarshalJSON_Empty(t *testing.T) {
	hf := HideFields{}

	bytes, err := json.Marshal(hf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	res := string(bytes)

	if res != "[]" {
		t.Fatalf("expected [], got %s", string(bytes))
	}
}

func TestHideFields_MarshalJSON_NonEmpty(t *testing.T) {
	hf := HideFields{"priority", "name"}

	bytes, err := json.Marshal(hf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	res := string(bytes)

	if res != "[\"priority\",\"name\"]" {
		t.Fatalf("expected [\"priority\",\"name\"], got %s", string(bytes))
	}
}
