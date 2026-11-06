package handler

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
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
	// 递归读取：顶层 2 个文件 + 子目录 1 个文件
	if len(entry.Files) != 3 {
		t.Fatalf("文件数 = %d, want 3: %+v", len(entry.Files), fileNamesOf(entry))
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
	if _, ok := byName["sub/ignored.txt"]; !ok {
		t.Errorf("子目录文件应带相对路径参与部署: %v", fileNamesOf(entry))
	}
}

func TestReadDirUploadErrors(t *testing.T) {
	if _, err := readDirUpload("relative/dir"); err == nil {
		t.Error("相对路径应返回错误")
	}
	if _, err := readDirUpload(t.TempDir()); err == nil {
		t.Error("空目录应返回错误")
	}
	onlyDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(onlyDir, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := readDirUpload(onlyDir); err == nil {
		t.Error("只有空子目录应返回错误")
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

func TestHistoryPagedAndLegacy(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "history.json"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	for i := 0; i < 12; i++ {
		h := store.History{Operator: "u", Service: "Svc", Time: time.Now().Add(time.Duration(i) * time.Second)}
		if _, err := st.Add(h); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	cfg := &config.Config{RootDir: t.TempDir(), Services: []config.ServiceEntry{{Name: "Svc"}}}
	h := New(cfg, st, middleware.NewAuth("admin", "hash", "", ""), nil, nil)

	// 不带参数：保持旧行为，返回最近 50 条数组
	rec := httptest.NewRecorder()
	h.history(rec, httptest.NewRequest(http.MethodGet, "/api/history", nil))
	var legacy []store.History
	if err := json.Unmarshal(rec.Body.Bytes(), &legacy); err != nil {
		t.Fatalf("旧接口返回不是数组: %v, body=%s", err, rec.Body.String())
	}
	if len(legacy) != 12 {
		t.Errorf("旧接口应返回 12 条，实际 %d", len(legacy))
	}

	// 带参数：返回分页对象
	rec2 := httptest.NewRecorder()
	h.history(rec2, httptest.NewRequest(http.MethodGet, "/api/history?page=2&page_size=10", nil))
	var page store.PageResult
	if err := json.Unmarshal(rec2.Body.Bytes(), &page); err != nil {
		t.Fatalf("分页响应解析失败: %v", err)
	}
	if page.Total != 12 || page.Page != 2 || page.PageSize != 10 || len(page.Items) != 2 {
		t.Errorf("分页结果不正确: %+v", page)
	}

	// page_size 上限
	rec3 := httptest.NewRecorder()
	h.history(rec3, httptest.NewRequest(http.MethodGet, "/api/history?page=1&page_size=1000", nil))
	var capped store.PageResult
	if err := json.Unmarshal(rec3.Body.Bytes(), &capped); err != nil {
		t.Fatalf("分页响应解析失败: %v", err)
	}
	if capped.PageSize != 100 {
		t.Errorf("page_size 应限制为 100，实际 %d", capped.PageSize)
	}
}

func TestSafeRelativePath(t *testing.T) {
	valid := []string{"a.dll", "sub/a.dll", `sub\a.dll`, "sub/deep/a.json"}
	for _, v := range valid {
		if _, err := safeRelativePath(v); err != nil {
			t.Errorf("合法路径 %q 被拒绝: %v", v, err)
		}
	}
	invalid := []string{"", ".", "..", "../a.dll", `..\a.dll`, "sub/../../a.dll", "/abs/a.dll", `C:\abs\a.dll`, "C:/abs/a.dll"}
	for _, v := range invalid {
		if _, err := safeRelativePath(v); err == nil {
			t.Errorf("非法路径 %q 未拒绝", v)
		}
	}
	got, err := safeRelativePath("sub/./a.dll")
	if err != nil || got != "sub/a.dll" {
		t.Errorf("规范化结果 = %q, err=%v", got, err)
	}
}

func TestReadDirUploadRecursiveDeploy(t *testing.T) {
	src := t.TempDir()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "app.dll"), []byte("top"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "plugins", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "plugins", "p.dll"), []byte("plugin"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "plugins", "deep", "d.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeTestZip(t, filepath.Join(src, "plugins", "bundle.zip"), map[string]string{"inner/x.txt": "x"})

	entry, err := readDirUpload(src)
	if err != nil {
		t.Fatalf("readDirUpload: %v", err)
	}
	wantNames := map[string]bool{
		"app.dll": true, "plugins/p.dll": true, "plugins/deep/d.json": true, "plugins/bundle.zip": true,
	}
	if len(entry.Files) != len(wantNames) {
		t.Fatalf("文件数 = %d: %v", len(entry.Files), fileNamesOf(entry))
	}
	for _, f := range entry.Files {
		if !wantNames[f.FileName] {
			t.Errorf("意外的相对路径: %q", f.FileName)
		}
	}

	st, err := store.New(filepath.Join(t.TempDir(), "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{RootDir: root, Services: []config.ServiceEntry{{Name: "Svc"}}}
	h := New(cfg, st, middleware.NewAuth("admin", "hash", "", ""), nil, nil)
	// filesOnly=true 避免测试环境启停 Windows 服务
	res := h.deployMulti("Svc", filepath.Join("app", "target"), entry, true)
	if res.FinalState != "success" {
		t.Fatalf("部署失败: %+v", res.Steps)
	}

	checks := []struct{ rel, want string }{
		{"app/target/app.dll", "top"},
		{"app/target/plugins/p.dll", "plugin"},
		{"app/target/plugins/deep/d.json", "{}"},
		{"app/target/plugins/inner/x.txt", "x"},
	}
	for _, c := range checks {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(c.rel)))
		if err != nil {
			t.Errorf("缺少 %s: %v", c.rel, err)
			continue
		}
		if string(data) != c.want {
			t.Errorf("%s = %q, want %q", c.rel, data, c.want)
		}
	}

	names := make([]string, 0, len(entry.Files))
	for _, f := range entry.Files {
		names = append(names, f.FileName)
	}
	deleteSourceFiles(src, names)
	for _, name := range names {
		p := filepath.Join(src, filepath.FromSlash(name))
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("源文件未删除: %s, err=%v", p, err)
		}
	}
}

func TestUploadPaths(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{RootDir: t.TempDir(), Services: []config.ServiceEntry{{Name: "Svc"}}}
	h := New(cfg, st, middleware.NewAuth("admin", "hash", "", ""), nil, nil)

	newReq := func(paths string) *http.Request {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		if paths != "" {
			if err := mw.WriteField("paths", paths); err != nil {
				t.Fatal(err)
			}
		}
		part, err := mw.CreateFormFile("files", "a.dll")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte("data")); err != nil {
			t.Fatal(err)
		}
		if err := mw.Close(); err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/upload", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		return req
	}

	rec := httptest.NewRecorder()
	h.upload(rec, newReq(`["sub/a.dll"]`))
	var out struct {
		UploadID string `json:"upload_id"`
		Error    string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if out.Error != "" || out.UploadID == "" {
		t.Fatalf("上传失败: %s", rec.Body.String())
	}
	h.mu.Lock()
	entry := h.uploads[out.UploadID]
	h.mu.Unlock()
	if len(entry.Files) != 1 || entry.Files[0].FileName != "sub/a.dll" {
		t.Errorf("相对路径未生效: %+v", entry.Files)
	}

	rec2 := httptest.NewRecorder()
	h.upload(rec2, newReq(`["../evil.dll"]`))
	var out2 struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec2.Body.Bytes(), &out2); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if out2.Error == "" {
		t.Error("非法相对路径应被拒绝")
	}

	// 不带 paths 的旧调用仍使用原始文件名
	rec3 := httptest.NewRecorder()
	h.upload(rec3, newReq(""))
	var out3 struct {
		UploadID string `json:"upload_id"`
	}
	if err := json.Unmarshal(rec3.Body.Bytes(), &out3); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	h.mu.Lock()
	entry3 := h.uploads[out3.UploadID]
	h.mu.Unlock()
	if len(entry3.Files) != 1 || entry3.Files[0].FileName != "a.dll" {
		t.Errorf("旧调用应保持纯文件名: %+v", entry3.Files)
	}
}
