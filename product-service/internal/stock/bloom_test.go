package stock

import "testing"

func TestBloomFilter(t *testing.T) {
	filter := NewBloomFilter(2048, 3)
	filter.AddInt32(1)
	filter.AddInt32(2)

	if !filter.MightContainInt32(1) {
		t.Fatalf("expected bloom filter to contain 1")
	}
	if !filter.MightContainInt32(2) {
		t.Fatalf("expected bloom filter to contain 2")
	}
	if filter.MightContainInt32(9999) {
		t.Fatalf("unexpected positive for 9999 in small deterministic test")
	}
}
