package qdrant

import (
	"encoding/json"
	"testing"
)

func TestPointIDWireContract(t *testing.T) {
	for _, wire := range []string{"0", "9007199254740991", "9007199254740993", "18446744073709551615", `"01234567-89ab-cdef-0123-456789abcdef"`} {
		t.Run(wire, func(t *testing.T) {
			var id PointID
			if err := json.Unmarshal([]byte(wire), &id); err != nil {
				t.Fatal(err)
			}
			got, err := json.Marshal(id)
			if err != nil || string(got) != wire {
				t.Fatalf("got %s, err=%v", got, err)
			}
		})
	}
	for _, wire := range []string{"-1", "1.5", "1e2", "18446744073709551616", "true", "{}", "[]", "null", `""`, `"123"`, `"not-a-uuid"`} {
		t.Run("reject_"+wire, func(t *testing.T) {
			var id PointID
			if err := json.Unmarshal([]byte(wire), &id); err == nil {
				t.Fatalf("accepted %s", wire)
			}
		})
	}
	var cursor *PointID
	if err := json.Unmarshal([]byte("null"), &cursor); err != nil || cursor != nil {
		t.Fatal("null cursor was not absent")
	}
	if err := json.Unmarshal([]byte("0"), &cursor); err != nil || cursor == nil || cursor.String() != "0" {
		t.Fatal("zero cursor was lost")
	}
}
