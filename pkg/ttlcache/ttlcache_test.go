package ttlcache

import (
	"testing"
	"time"
)

func TestGetSetExpiry(t *testing.T) {
	c := New[string, int](30*time.Millisecond, 8)
	c.Set("a", 1)
	if v, ok := c.Get("a"); !ok || v != 1 {
		t.Fatalf("want (1,true), got (%v,%v)", v, ok)
	}
	time.Sleep(40 * time.Millisecond)
	if _, ok := c.Get("a"); ok {
		t.Fatal("entry should have expired")
	}
}

func TestDeleteAndClear(t *testing.T) {
	c := New[string, int](time.Minute, 8)
	c.Set("a", 1)
	c.Set("b", 2)
	c.Delete("a")
	if _, ok := c.Get("a"); ok {
		t.Fatal("a should be deleted")
	}
	if _, ok := c.Get("b"); !ok {
		t.Fatal("b should remain")
	}
	c.Clear()
	if c.Len() != 0 {
		t.Fatalf("Len after Clear = %d, want 0", c.Len())
	}
}

func TestMaxEviction(t *testing.T) {
	c := New[int, string](time.Minute, 3)
	for i := 0; i < 10; i++ {
		c.Set(i, "v")
	}
	if c.Len() > 3 {
		t.Fatalf("Len = %d, want <= 3", c.Len())
	}
	// 容量保界但仍有可命中的最近写入项。
	if _, ok := c.Get(9); !ok {
		t.Fatal("most recent entry should still be cached")
	}
}
