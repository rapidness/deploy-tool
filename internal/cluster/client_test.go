package cluster

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"update-app/internal/config"
)

func TestUploadAndDeploy(t *testing.T) {
	var token string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token = r.Header.Get("Authorization")
		switch r.URL.Path {
		case "/api/upload":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("解析上传请求失败: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			files := r.MultipartForm.File["files"]
			if len(files) != 1 || files[0].Filename != "a.zip" {
				t.Errorf("上传文件不符: %+v", files)
			}
			json.NewEncoder(w).Encode(map[string]any{"upload_id": "u1"})
		case "/api/deploy":
			var req struct {
				UploadID  string `json:"upload_id"`
				Service   string `json:"service"`
				TargetDir string `json:"target_dir"`
				FilesOnly bool   `json:"files_only"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("解析部署请求失败: %v", err)
			}
			if req.UploadID != "u1" || req.Service != "Svc" || req.TargetDir != "app" || req.FilesOnly {
				t.Errorf("部署参数不符: %+v", req)
			}
			json.NewEncoder(w).Encode(map[string]any{
				"steps":       []map[string]any{{"name": "stop", "status": "success", "message": "服务已停止"}},
				"final_state": "success",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewClient(config.ClusterNode{Name: "n1", URL: srv.URL + "/", APIToken: "tok"})

	uploadID, err := c.Upload([]File{{Name: "a.zip", Data: []byte("abc")}})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if uploadID != "u1" {
		t.Errorf("upload_id = %q, want u1", uploadID)
	}
	if token != "Bearer tok" {
		t.Errorf("Authorization = %q", token)
	}

	res, err := c.Deploy(uploadID, "Svc", "app", false)
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if res.FinalState != "success" || len(res.Steps) != 1 || res.Steps[0].Name != "stop" {
		t.Errorf("部署结果不符: %+v", res)
	}
}

func TestDeployHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewClient(config.ClusterNode{Name: "n1", URL: srv.URL, APIToken: "bad"})
	if _, err := c.Deploy("u1", "Svc", "app", false); err == nil {
		t.Error("期望返回错误")
	}
}

func TestGet(t *testing.T) {
	var gotAuth, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.RawQuery
		if r.URL.Path != "/api/logs/view" {
			t.Errorf("请求路径 = %q", r.URL.Path)
		}
		w.Write([]byte("log content"))
	}))
	defer srv.Close()

	c := NewClient(config.ClusterNode{Name: "n1", URL: srv.URL + "/", APIToken: "tok"})
	body, err := c.Get("/api/logs/view?name=a.log", "查看远端日志")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(body) != "log content" {
		t.Errorf("响应体 = %q", string(body))
	}
	if gotAuth != "Bearer tok" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotQuery != "name=a.log" {
		t.Errorf("查询参数 = %q", gotQuery)
	}
}
