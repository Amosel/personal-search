package model

import "testing"

func TestStableID_Deterministic(t *testing.T) {
	id1 := StableID("conv123", "msg456")
	id2 := StableID("conv123", "msg456")
	if id1 != id2 {
		t.Fatalf("not stable: %s != %s", id1, id2)
	}

	// Run multiple times to verify determinism
	for i := 0; i < 1000; i++ {
		id := StableID("conv123", "msg456")
		if id != id1 {
			t.Fatalf("iteration %d: not deterministic", i)
		}
	}
}

func TestStableID_DifferentInputs(t *testing.T) {
	id1 := StableID("conv1", "msg1")
	id2 := StableID("conv2", "msg2")
	if id1 == id2 {
		t.Fatal("different inputs produced same ID")
	}

	id3 := StableID("conv1", "msg2")
	if id1 == id3 {
		t.Fatal("different message IDs produced same ID")
	}

	id4 := StableID("conv2", "msg1")
	if id1 == id4 {
		t.Fatal("different conversation IDs produced same ID")
	}
}

func TestStableID_PanicsOnEmptyConversationID(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("didn't panic on empty conversationID")
		}
	}()
	StableID("", "msg")
}

func TestStableID_PanicsOnEmptyMessageID(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("didn't panic on empty messageID")
		}
	}()
	StableID("conv", "")
}

func TestStableID_PanicsOnBothEmpty(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("didn't panic on both empty")
		}
	}()
	StableID("", "")
}
