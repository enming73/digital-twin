package mes

import "testing"

func TestFixtureLookup(t *testing.T) {
	first, ok := LookupWorkOrder("work-order-1")
	second, again := LookupWorkOrder("work-order-1")
	if !ok || !again || first != second || !first.Synthetic {
		t.Fatalf("fixture: %+v", first)
	}
	if _, ok := LookupWorkOrder("unknown"); ok {
		t.Fatal("unknown fixture exists")
	}
}
