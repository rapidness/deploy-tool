package handler

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"update-app/internal/config"
	"update-app/internal/middleware"
	"update-app/internal/store"
)

func TestParseServiceNames(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"A", []string{"A"}},
		{"A,B,C", []string{"A", "B", "C"}},
		{" A , B , C ", []string{"A", "B", "C"}},
	}
	for _, c := range cases {
		got := ParseServiceNames(c.in)
		if len(got) != len(c.want) {
			t.Errorf("ParseServiceNames(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("ParseServiceNames(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}

func TestLogsProxyRemoteNode(t *testing.T) {
	var gotAuth string
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		switch r.URL.Path {
		case "/api/logs":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Write([]byte(`[{"name":"app.log","size":3}]`))
		case "/api/logs/view":
			if name := r.URL.Query().Get("name"); name != "app.log" {
				t.Errorf("name = %q, want app.log", name)
			}
			w.Write([]byte("log content"))
		case "/api/logs/download":
			w.Write([]byte("log content"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()

	cfg := &config.Config{
		RootDir:  t.TempDir(),
		NodeName: "node-1",
		Cluster: config.ClusterConfig{Nodes: []config.ClusterNode{
			{Name: "node-1", URL: "http://127.0.0.1:1"},
			{Name: "node-2", URL: remote.URL, APIToken: "tok"},
		}},
	}
	h := New(cfg, nil, middleware.NewAuth("admin", "hash", "", ""), nil, nil)

	// 列表代理
	rec := httptest.NewRecorder()
	h.logs(rec, httptest.NewRequest(http.MethodGet, "/api/logs?node=node-2", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "app.log") {
		t.Errorf("列表代理失败: code=%d body=%s", rec.Code, rec.Body.String())
	}
	if gotAuth != "Bearer tok" {
		t.Errorf("Authorization = %q", gotAuth)
	}

	// 查看代理
	rec = httptest.NewRecorder()
	h.logView(rec, httptest.NewRequest(http.MethodGet, "/api/logs/view?node=node-2&name=app.log", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "log content" {
		t.Errorf("查看代理失败: code=%d body=%s", rec.Code, rec.Body.String())
	}

	// 下载代理
	rec = httptest.NewRecorder()
	h.logDownload(rec, httptest.NewRequest(http.MethodGet, "/api/logs/download?node=node-2&name=app.log", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "log content" {
		t.Errorf("下载代理失败: code=%d body=%s", rec.Code, rec.Body.String())
	}

	// 非法文件名
	rec = httptest.NewRecorder()
	h.logDownload(rec, httptest.NewRequest(http.MethodGet, "/api/logs/download?node=node-2&name=..%2Fsecret", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("非法文件名应返回 400, got %d", rec.Code)
	}

	// 未知节点
	rec = httptest.NewRecorder()
	h.logs(rec, httptest.NewRequest(http.MethodGet, "/api/logs?node=node-x", nil))
	if !strings.Contains(rec.Body.String(), "未知节点") {
		t.Errorf("未知节点应返回错误, got %s", rec.Body.String())
	}
}

func writeTestZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("创建 zip 条目失败: %v", err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("写入 zip 条目失败: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("关闭 zip 失败: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("写入 zip 文件失败: %v", err)
	}
}

func TestReadDirUpload(t *testing.T) {
	dir := t.TempDir()
	writeTestZip(t, filepath.Join(dir, "a.zip"), map[string]string{"app/x.txt": "x"})
	if err := os.WriteFile(filepath.Join(dir, "b.dll"), []byte("dll"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "ignored.txt"), []byte("no"), 0o644); err != nil {
		t.Fatal(err)
	}

	entry, err := readDirUpload(dir)
	if err != nil {
		t.Fatalf("readDirUpload: %v", err)
	}
	if len(entry.Files) != 2 {
		t.Fatalf("文件数 = %d, want 2", len(entry.Files))
	}
	byName := make(map[string]uploadedFile, len(entry.Files))
	for _, f := range entry.Files {
		byName[f.FileName] = f
	}
	if !byName["a.zip"].IsZip {
		t.Error("a.zip 应识别为 zip")
	}
	if byName["b.dll"].IsZip {
		t.Error("b.dll 不应识别为 zip")
	}
}

func TestReadDirUploadErrors(t *testing.T) {
	if _, err := readDirUpload("relative/dir"); err == nil {
		t.Error("相对路径应返回错误")
	}
	if _, err := readDirUpload(t.TempDir()); err == nil {
		t.Error("空目录应返回错误")
	}
	if _, err := readDirUpload(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("不存在的目录应返回错误")
	}
	bad := t.TempDir()
	if err := os.WriteFile(filepath.Join(bad, "c.zip"), []byte("not a zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readDirUpload(bad); err == nil {
		t.Error("非法 zip 应返回错误")
	}
}

func TestClusterDeployDeleteSource(t *testing.T) {
	root := t.TempDir()
	src := t.TempDir()
	srcFile := filepath.Join(src, "a.txt")
	if err := os.WriteFile(srcFile, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	st, err := store.New(filepath.Join(t.TempDir(), "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		RootDir:  root,
		Services: []config.ServiceEntry{{Name: "Svc"}},
	}
	h := New(cfg, st, middleware.NewAuth("admin", "hash", "", ""), nil, nil)

	body := fmt.Sprintf(`{"source_dir":%q,"service":"Svc","target_dir":"app","files_only":true,"delete_source":true}`, src)
	rec := httptest.NewRecorder()
	h.clusterDeploy(rec, httptest.NewRequest(http.MethodPost, "/api/cluster/deploy", strings.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("clusterDeploy: code=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		BatchID string `json:"batch_id"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("clusterDeploy 返回错误: %s", resp.Error)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		v, ok := h.batches.Load(resp.BatchID)
		if ok {
			status := v.(*batchState).snapshot().Status
			if status == "success" {
				break
			}
			if status == "failed" || status == "partial" {
				t.Fatalf("批量部署失败: %s", status)
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("等待批量部署完成超时")
		}
		time.Sleep(50 * time.Millisecond)
	}

	if _, err := os.Stat(filepath.Join(root, "app", "a.txt")); err != nil {
		t.Errorf("目标文件未部署: %v", err)
	}
	if _, err := os.Stat(srcFile); !os.IsNotExist(err) {
		t.Errorf("更新成功后源文件应已删除, err=%v", err)
	}
}
