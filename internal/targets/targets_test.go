package targets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreSaveAndGet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "targets.json")
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	list := []Target{
		{Path: "bin", Desc: "可执行文件"},
		{Path: "config", Desc: ""},
	}
	if err := s.Save("svc1", list); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got := s.Get("svc1")
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	// 验证排序后顺序
	if got[0].Path != "bin" || got[1].Path != "config" {
		t.Errorf("unexpected order: %+v", got)
	}
}

func TestStoreDedupAndEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "targets.json")
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	list := []Target{
		{Path: "a", Desc: "1"},
		{Path: "a", Desc: "2"}, // 重复
		{Path: "", Desc: "x"},  // 空
		{Path: "b", Desc: ""},
	}
	if err := s.Save("svc", list); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got := s.Get("svc")
	if len(got) != 2 {
		t.Fatalf("expected 2 after dedup, got %d: %+v", len(got), got)
	}
}

func TestStoreReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "targets.json")
	s1, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s1.Save("svc", []Target{{Path: "x", SourceDir: `C:\build\release`, DeleteSource: true}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	s2, err := New(path)
	if err != nil {
		t.Fatalf("New reload: %v", err)
	}
	got := s2.Get("svc")
	if len(got) != 1 || got[0].Path != "x" {
		t.Errorf("reload failed: %+v", got)
	}
	if len(got) == 1 && got[0].SourceDir != `C:\build\release` {
		t.Errorf("source_dir 未持久化: %+v", got[0])
	}
	if len(got) == 1 && !got[0].DeleteSource {
		t.Errorf("delete_source 未持久化: %+v", got[0])
	}
}

// 旧配置文件没有 delete_source 字段时，应按 false 兼容。
func TestStoreOldJSONWithoutDeleteSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "targets.json")
	old := `{"svc":[{"path":"bin","desc":"","source_dir":"C:\\build"}]}`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got := s.Get("svc")
	if len(got) != 1 || got[0].DeleteSource {
		t.Errorf("旧配置应默认不删除源文件: %+v", got)
	}
}

func TestStoreAll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "targets.json")
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_ = s.Save("a", []Target{{Path: "p1"}})
	_ = s.Save("b", []Target{{Path: "p2"}})
	all := s.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 services, got %d", len(all))
	}
	if len(all["a"]) != 1 || all["a"][0].Path != "p1" {
		t.Errorf("unexpected a: %+v", all["a"])
	}
}

func TestStoreFileCreated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "targets.json")
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Save("svc", []Target{{Path: "x"}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

func TestStoreGetEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "targets.json")
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got := s.Get("nonexistent")
	if len(got) != 0 {
		t.Errorf("expected empty, got %+v", got)
	}
}
