package service

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func makeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)
	for name, content := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func TestVerifyZipValid(t *testing.T) {
	data := makeZip(t, map[string]string{"a.txt": "hello"})
	n, err := VerifyZip(data)
	if err != nil {
		t.Fatalf("VerifyZip: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1, got %d", n)
	}
}

func TestVerifyZipEmpty(t *testing.T) {
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	_, err := VerifyZip(buf.Bytes())
	if err == nil {
		t.Errorf("expected error for empty zip")
	}
}

func TestVerifyZipCorrupt(t *testing.T) {
	_, err := VerifyZip([]byte("not a zip"))
	if err == nil {
		t.Errorf("expected error for corrupt zip")
	}
}

func TestSafeJoinValid(t *testing.T) {
	root := t.TempDir()
	got, err := SafeJoin(root, "sub/file.txt")
	if err != nil {
		t.Fatalf("SafeJoin: %v", err)
	}
	expected := filepath.Join(root, "sub", "file.txt")
	if got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}

func TestSafeJoinEscape(t *testing.T) {
	root := t.TempDir()
	_, err := SafeJoin(root, "../../etc/passwd")
	if err == nil {
		t.Errorf("expected error for path escape")
	}
}

func TestSafeJoinAbsolute(t *testing.T) {
	root := t.TempDir()
	// 使用当前 OS 的绝对路径
	abs := filepath.Join(filepath.VolumeName(root), string(filepath.Separator), "etc", "passwd")
	_, err := SafeJoin(root, abs)
	if err == nil {
		t.Errorf("expected error for absolute path")
	}
}

func TestExtractZipBasic(t *testing.T) {
	root := t.TempDir()
	data := makeZip(t, map[string]string{
		"a.txt":          "content-a",
		"sub/b.txt":      "content-b",
		"sub/nested/c.txt": "content-c",
	})
	written, err := ExtractZip(data, root)
	if err != nil {
		t.Fatalf("ExtractZip: %v", err)
	}
	if len(written) != 3 {
		t.Errorf("expected 3 files, got %d", len(written))
	}
	for _, p := range written {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("file not found: %s", p)
		}
	}
	if content, _ := os.ReadFile(filepath.Join(root, "a.txt")); string(content) != "content-a" {
		t.Errorf("a.txt content mismatch")
	}
	if content, _ := os.ReadFile(filepath.Join(root, "sub", "b.txt")); string(content) != "content-b" {
		t.Errorf("b.txt content mismatch")
	}
}

func TestExtractZipOverwrite(t *testing.T) {
	root := t.TempDir()
	oldPath := filepath.Join(root, "a.txt")
	if err := os.WriteFile(oldPath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	data := makeZip(t, map[string]string{"a.txt": "new"})
	if _, err := ExtractZip(data, root); err != nil {
		t.Fatalf("ExtractZip: %v", err)
	}
	content, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "new" {
		t.Errorf("expected new content, got %s", content)
	}
}

func TestExtractZipEscape(t *testing.T) {
	root := t.TempDir()
	data := makeZip(t, map[string]string{"../../escape.txt": "bad"})
	_, err := ExtractZip(data, root)
	if err == nil {
		t.Errorf("expected error for path escape")
	}
}

func TestExtractZipWithDir(t *testing.T) {
	root := t.TempDir()
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)
	// 创建目录条目
	if _, err := w.Create("mydir/"); err != nil {
		t.Fatal(err)
	}
	f, err := w.Create("mydir/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("hi")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	written, err := ExtractZip(buf.Bytes(), root)
	if err != nil {
		t.Fatalf("ExtractZip: %v", err)
	}
	if len(written) != 1 {
		t.Errorf("expected 1 file, got %d", len(written))
	}
	if _, err := os.Stat(filepath.Join(root, "mydir", "file.txt")); err != nil {
		t.Errorf("file not found: %v", err)
	}
}

func TestListZip(t *testing.T) {
	data := makeZip(t, map[string]string{
		"a.txt":     "hello",
		"sub/b.txt": "world",
	})
	entries, err := ListZip(data)
	if err != nil {
		t.Fatalf("ListZip: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	// 验证条目内容
	found := map[string]bool{}
	for _, e := range entries {
		if e.Name == "a.txt" {
			found["a.txt"] = true
			if e.Size != 5 {
				t.Errorf("a.txt size: expected 5, got %d", e.Size)
			}
			if e.Target != "a.txt" {
				t.Errorf("a.txt target: expected a.txt, got %s", e.Target)
			}
		}
		if e.Name == "sub/b.txt" {
			found["sub/b.txt"] = true
		}
	}
	if !found["a.txt"] || !found["sub/b.txt"] {
		t.Errorf("missing entries: %v", found)
	}
}

func TestExtractZipWithMapping(t *testing.T) {
	root := t.TempDir()
	data := makeZip(t, map[string]string{
		"src/app.exe":   "binary",
		"src/config.ini": "config",
	})
	mapping := map[string]string{
		"src/app.exe":    "bin/app.exe",
		"src/config.ini": "config/config.ini",
	}
	written, err := ExtractZipWithMapping(data, root, mapping)
	if err != nil {
		t.Fatalf("ExtractZipWithMapping: %v", err)
	}
	if len(written) != 2 {
		t.Fatalf("expected 2 files, got %d", len(written))
	}
	// 验证映射后的路径
	if _, err := os.Stat(filepath.Join(root, "bin", "app.exe")); err != nil {
		t.Errorf("mapped file not found: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "config", "config.ini")); err != nil {
		t.Errorf("mapped file not found: %v", err)
	}
	// 验证内容
	content, _ := os.ReadFile(filepath.Join(root, "bin", "app.exe"))
	if string(content) != "binary" {
		t.Errorf("content mismatch: got %s", content)
	}
}

func TestExtractZipWithMappingEscape(t *testing.T) {
	root := t.TempDir()
	data := makeZip(t, map[string]string{"a.txt": "x"})
	mapping := map[string]string{
		"a.txt": "../../escape.txt",
	}
	_, err := ExtractZipWithMapping(data, root, mapping)
	if err == nil {
		t.Errorf("expected error for path escape in mapping")
	}
}

func TestExtractZipWithMappingPartial(t *testing.T) {
	root := t.TempDir()
	data := makeZip(t, map[string]string{
		"a.txt": "content-a",
		"b.txt": "content-b",
	})
	// 只映射 a.txt，b.txt 使用原始路径
	mapping := map[string]string{
		"a.txt": "renamed.txt",
	}
	written, err := ExtractZipWithMapping(data, root, mapping)
	if err != nil {
		t.Fatalf("ExtractZipWithMapping: %v", err)
	}
	if len(written) != 2 {
		t.Fatalf("expected 2 files, got %d", len(written))
	}
	if _, err := os.Stat(filepath.Join(root, "renamed.txt")); err != nil {
		t.Errorf("renamed file not found: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "b.txt")); err != nil {
		t.Errorf("original file not found: %v", err)
	}
}

func TestDeployFilesBasic(t *testing.T) {
	root := t.TempDir()
	items := []FileItem{
		{FileName: "a.dll", Data: []byte("dll-content")},
		{FileName: "b.json", Data: []byte("{}")},
	}
	written, err := DeployFiles(items, root, "deploy/sub")
	if err != nil {
		t.Fatalf("DeployFiles: %v", err)
	}
	if len(written) != 2 {
		t.Fatalf("expected 2, got %d", len(written))
	}
	if content, _ := os.ReadFile(filepath.Join(root, "deploy", "sub", "a.dll")); string(content) != "dll-content" {
		t.Errorf("a.dll content mismatch")
	}
	if content, _ := os.ReadFile(filepath.Join(root, "deploy", "sub", "b.json")); string(content) != "{}" {
		t.Errorf("b.json content mismatch")
	}
}

func TestDeployFilesOverwrite(t *testing.T) {
	root := t.TempDir()
	targetPath := filepath.Join(root, "bin", "a.dll")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	items := []FileItem{{FileName: "a.dll", Data: []byte("new")}}
	if _, err := DeployFiles(items, root, "bin"); err != nil {
		t.Fatalf("DeployFiles: %v", err)
	}
	content, _ := os.ReadFile(targetPath)
	if string(content) != "new" {
		t.Errorf("expected new, got %s", content)
	}
}

func TestDeployFilesEscape(t *testing.T) {
	root := t.TempDir()
	items := []FileItem{{FileName: "../../escape.txt", Data: []byte("bad")}}
	_, err := DeployFiles(items, root, "bin")
	if err == nil {
		t.Errorf("expected error for path escape")
	}
}

func TestExtractZipToDirBasic(t *testing.T) {
	root := t.TempDir()
	data := makeZip(t, map[string]string{
		"a.txt":     "content-a",
		"sub/b.txt": "content-b",
	})
	written, err := ExtractZipToDir(data, root, "target/dir")
	if err != nil {
		t.Fatalf("ExtractZipToDir: %v", err)
	}
	if len(written) != 2 {
		t.Fatalf("expected 2, got %d", len(written))
	}
	if content, _ := os.ReadFile(filepath.Join(root, "target", "dir", "a.txt")); string(content) != "content-a" {
		t.Errorf("a.txt content mismatch")
	}
	if content, _ := os.ReadFile(filepath.Join(root, "target", "dir", "sub", "b.txt")); string(content) != "content-b" {
		t.Errorf("b.txt content mismatch")
	}
}

func TestExtractZipToDirEscape(t *testing.T) {
	root := t.TempDir()
	data := makeZip(t, map[string]string{"../../escape.txt": "bad"})
	_, err := ExtractZipToDir(data, root, "target")
	if err == nil {
		t.Errorf("expected error for path escape")
	}
}
