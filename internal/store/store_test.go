package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreAddAndList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.json")
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	h := History{
		Operator:   "alice",
		Service:    "svc1",
		FileName:   "update.zip",
		FinalState: "success",
		Steps: []StepResult{
			{Name: "stop", Status: "success"},
			{Name: "deploy", Status: "success"},
			{Name: "start", Status: "success"},
		},
	}
	id, err := s.Add(h)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if id != 1 {
		t.Errorf("expected id=1, got %d", id)
	}
	list := s.List(0)
	if len(list) != 1 {
		t.Fatalf("expected 1 item, got %d", len(list))
	}
	if list[0].ID != 1 || list[0].Operator != "alice" {
		t.Errorf("unexpected item: %+v", list[0])
	}
}

func TestStoreListOrderAndLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.json")
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for i := 0; i < 5; i++ {
		h := History{Operator: "u", Service: "s", Time: time.Now().Add(time.Duration(i) * time.Second)}
		if _, err := s.Add(h); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	all := s.List(0)
	if len(all) != 5 {
		t.Fatalf("expected 5, got %d", len(all))
	}
	for i := 0; i < len(all)-1; i++ {
		if all[i].Time.Before(all[i+1].Time) {
			t.Errorf("not sorted desc at %d", i)
		}
	}
	limited := s.List(2)
	if len(limited) != 2 {
		t.Errorf("expected 2, got %d", len(limited))
	}
}

func TestStoreReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.json")
	s1, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	h := History{Operator: "bob", Service: "svc", FileName: "f.zip"}
	if _, err := s1.Add(h); err != nil {
		t.Fatalf("Add: %v", err)
	}
	s2, err := New(path)
	if err != nil {
		t.Fatalf("New reload: %v", err)
	}
	list := s2.List(0)
	if len(list) != 1 || list[0].Operator != "bob" {
		t.Errorf("reload failed: %+v", list)
	}
}

func TestStoreGet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.json")
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	id, err := s.Add(History{Operator: "x"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	got, ok := s.Get(id)
	if !ok || got.Operator != "x" {
		t.Errorf("Get failed: %+v ok=%v", got, ok)
	}
	if _, ok := s.Get(999); ok {
		t.Errorf("expected not found")
	}
}

func TestStoreFileCreated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "history.json")
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := s.Add(History{Operator: "y"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not created: %v", err)
	}
}
