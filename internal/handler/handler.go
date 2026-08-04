// Package handler 实现 HTTP 请求处理与路由。
package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"update-app/internal/config"
	"update-app/internal/middleware"
	"update-app/internal/service"
	"update-app/internal/store"
	"update-app/internal/targets"
	"update-app/web"
)

// Handler 汇总所有 HTTP 处理器与依赖。
type Handler struct {
	cfg     *config.Config
	store   *store.Store
	auth    *middleware.Auth
	targets *targets.Store
	rl      *middleware.RateLimiter

	mu             sync.Mutex
	uploads        map[string]uploadEntry // uploadID -> 上传内容
	deployStatuses sync.Map
}

// uploadEntry 保存已上传的文件数据（多文件）。
type uploadEntry struct {
	Files      []uploadedFile
	UploadTime time.Time
}

// uploadedFile 单个已上传文件。
type uploadedFile struct {
	FileName string
	Data     []byte
	IsZip    bool
}

// DeployStatusEntry 异步部署状态条目。
type DeployStatusEntry struct {
	DeployID   string               `json:"deploy_id"`
	Status     string               `json:"status"`
	Steps      []service.StepResult `json:"steps"`
	FinalState string               `json:"final_state"`
	Service    string               `json:"service"`
	TargetDir  string               `json:"target_dir"`
	FileName   string               `json:"file_name"`
	CreatedAt  time.Time            `json:"created_at"`
}

// New 创建 Handler。
func New(cfg *config.Config, st *store.Store, auth *middleware.Auth, ts *targets.Store, rl *middleware.RateLimiter) *Handler {
	return &Handler{cfg: cfg, store: st, auth: auth, targets: ts, rl: rl, uploads: make(map[string]uploadEntry)}
}

// Routes 注册所有路由。
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	bp := h.cfg.BasePath

	staticFS, err := fs.Sub(web.Static, "static")
	if err != nil {
		panic(fmt.Sprintf("嵌入静态资源子目录失败: %v", err))
	}
	mux.Handle(bp+"/static/", http.StripPrefix(bp+"/static/", http.FileServerFS(staticFS)))

	mux.HandleFunc(bp+"/login", h.login)
	mux.HandleFunc(bp+"/logout", h.logout)

	mux.HandleFunc(bp+"/", h.auth.RequireAuth(h.index))
	mux.HandleFunc(bp+"/logs", h.auth.RequireAuth(h.logsPage))

	// API 路由：Token 认证 + 限流
	apiAuth := h.auth.RequireAPIAuth
	registerAPI := func(pattern string, handler http.HandlerFunc) {
		if h.rl != nil {
			handler = h.rl.Middleware(handler)
		}
		mux.HandleFunc(pattern, apiAuth(handler))
	}

	registerAPI(bp+"/api/services", h.services)
	registerAPI(bp+"/api/upload", h.upload)
	registerAPI(bp+"/api/deploy", h.deploy)
	registerAPI(bp+"/api/deploy/upload", h.deployUpload)
	registerAPI(bp+"/api/deploy/status", h.deployStatus)
	registerAPI(bp+"/api/history", h.history)
	registerAPI(bp+"/api/service/control", h.control)
	registerAPI(bp+"/api/targets", h.handleTargets)
	registerAPI(bp+"/api/logs", h.logs)
	registerAPI(bp+"/api/logs/download", h.logDownload)
	registerAPI(bp+"/api/logs/view", h.logView)

	return mux
}

func (h *Handler) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != h.cfg.BasePath+"/" {
		http.NotFound(w, r)
		return
	}
	data, err := web.Static.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "内部错误", http.StatusInternalServerError)
		return
	}
	// 注入 base_path
	html := strings.Replace(string(data), "__BASE_PATH__", h.cfg.BasePath, -1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// logsPage 返回独立的日志文件页面。
func (h *Handler) logsPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != h.cfg.BasePath+"/logs" {
		http.NotFound(w, r)
		return
	}
	data, err := web.Static.ReadFile("static/logs.html")
	if err != nil {
		http.Error(w, "内部错误", http.StatusInternalServerError)
		return
	}
	html := strings.Replace(string(data), "__BASE_PATH__", h.cfg.BasePath, -1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		data, _ := web.Static.ReadFile("static/login.html")
		// 注入 base_path
		html := strings.Replace(string(data), "__BASE_PATH__", h.cfg.BasePath, -1)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(html))
		return
	}
	if h.auth.Login(w, r) {
		http.Redirect(w, r, h.cfg.BasePath+"/", http.StatusSeeOther)
	}
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	h.auth.Logout(w, r)
	http.Redirect(w, r, h.cfg.BasePath+"/login", http.StatusSeeOther)
}

// servicesResp 服务状态响应。
type servicesResp struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Status      string `json:"status"`
}

func (h *Handler) services(w http.ResponseWriter, r *http.Request) {
	var resp []servicesResp
	for _, s := range h.cfg.Services {
		mgr := service.NewManager(s.Name)
		st, err := mgr.Status()
		status := string(st)
		if err != nil {
			status = string(service.StateUnknown)
		}
		resp = append(resp, servicesResp{
			Name:        s.Name,
			DisplayName: s.DisplayName,
			Status:      status,
		})
	}
	writeJSON(w, resp)
}

// uploadResp 上传接口响应。
type uploadResp struct {
	UploadID string         `json:"upload_id"`
	Files    []uploadFile   `json:"files"`
}

// uploadFile 单个文件信息。
type uploadFile struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	IsZip  bool   `json:"is_zip"`
}

// upload 接收多文件上传，校验后保存到内存，返回 uploadID。
func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
		return
	}
	maxSize := int64(500 << 20) // 500MB
	r.Body = http.MaxBytesReader(w, r.Body, maxSize)
	if err := r.ParseMultipartForm(maxSize); err != nil {
		writeJSON(w, map[string]any{"error": "文件过大或解析失败: " + err.Error()})
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		// 兼容单字段 "file"
		if f := r.MultipartForm.File["file"]; len(f) > 0 {
			files = f
		}
	}
	if len(files) == 0 {
		writeJSON(w, map[string]any{"error": "未提供文件"})
		return
	}

	var uploaded []uploadedFile
	var fileInfos []uploadFile
	for _, header := range files {
		f, err := header.Open()
		if err != nil {
			writeJSON(w, map[string]any{"error": "打开文件失败: " + header.Filename})
			return
		}
		data, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			writeJSON(w, map[string]any{"error": "读取文件失败: " + header.Filename})
			return
		}
		isZip := strings.HasSuffix(strings.ToLower(header.Filename), ".zip")
		if isZip {
			if _, err := service.VerifyZip(data); err != nil {
				writeJSON(w, map[string]any{"error": "zip 校验失败(" + header.Filename + "): " + err.Error()})
				return
			}
		}
		uploaded = append(uploaded, uploadedFile{FileName: header.Filename, Data: data, IsZip: isZip})
		fileInfos = append(fileInfos, uploadFile{Name: header.Filename, Size: int64(len(data)), IsZip: isZip})
	}

	uploadID := generateUploadID()
	h.mu.Lock()
	h.uploads[uploadID] = uploadEntry{Files: uploaded, UploadTime: time.Now()}
	h.mu.Unlock()
	writeJSON(w, uploadResp{UploadID: uploadID, Files: fileInfos})
}

// deployReq 部署请求体。
type deployReq struct {
	UploadID  string `json:"upload_id"`
	Service   string `json:"service"`
	TargetDir string `json:"target_dir"` // 相对 rootDir 的目标目录
	FilesOnly bool   `json:"files_only"` // true=仅替换文件，不启停服务
}

func (h *Handler) deploy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
		return
	}
	var req deployReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"error": "解析请求失败: " + err.Error()})
		return
	}

	if _, ok := h.cfg.FindService(req.Service); !ok {
		writeJSON(w, map[string]any{"error": "未知服务: " + req.Service})
		return
	}
	if req.TargetDir == "" {
		writeJSON(w, map[string]any{"error": "目标路径不能为空"})
		return
	}
	h.mu.Lock()
	entry, ok := h.uploads[req.UploadID]
	h.mu.Unlock()
	if !ok {
		writeJSON(w, map[string]any{"error": "上传文件不存在或已过期，请重新上传"})
		return
	}

	result := h.deployMulti(req.Service, req.TargetDir, entry, req.FilesOnly)

	// 记算文件名摘要
	fileNames := make([]string, 0, len(entry.Files))
	for _, f := range entry.Files {
		fileNames = append(fileNames, f.FileName)
	}
	hist := store.History{
		Operator:   h.auth.Username,
		Service:    req.Service,
		FileName:   strings.Join(fileNames, ", "),
		Steps:      toStoreSteps(result.Steps),
		FinalState: result.FinalState,
	}
	h.store.Add(hist)
	h.mu.Lock()
	delete(h.uploads, req.UploadID)
	h.mu.Unlock()
	writeJSON(w, result)
}

// deployMulti 执行部署：filesOnly=true 时仅部署文件；否则停服务 → 部署文件 → 启服务。
func (h *Handler) deployMulti(serviceName, targetDir string, entry uploadEntry, filesOnly bool) service.DeployResult {
	var steps []service.StepResult

	if !filesOnly {
		mgr := service.NewManager(serviceName)

		// 1. 停止服务
		steps = append(steps, service.Step("stop", func() (string, error) {
			st, err := mgr.Status()
			if err != nil {
				return "", err
			}
			if st == service.StateStopped {
				return "服务已停止，跳过", nil
			}
			if err := mgr.Stop(); err != nil {
				return "", err
			}
			return "服务已停止", nil
		}))
		if steps[len(steps)-1].Status == "failed" {
			return service.DeployResult{Steps: steps, FinalState: "failed"}
		}
	}

	// 2. 部署文件
	steps = append(steps, service.Step("deploy", func() (string, error) {
		totalWritten := 0
		for _, f := range entry.Files {
			if f.IsZip {
				written, err := service.ExtractZipToDir(f.Data, h.cfg.RootDir, targetDir)
				if err != nil {
					return "", fmt.Errorf("解压 %s 失败: %w", f.FileName, err)
				}
				totalWritten += len(written)
			} else {
				written, err := service.DeployFiles([]service.FileItem{{FileName: f.FileName, Data: f.Data}}, h.cfg.RootDir, targetDir)
				if err != nil {
					return "", fmt.Errorf("部署 %s 失败: %w", f.FileName, err)
				}
				totalWritten += len(written)
			}
		}
		return fmt.Sprintf("已覆盖 %d 个文件到 %s", totalWritten, targetDir), nil
	}))
	if steps[len(steps)-1].Status == "failed" {
		return service.DeployResult{Steps: steps, FinalState: "failed"}
	}

	if !filesOnly {
		mgr := service.NewManager(serviceName)
		// 3. 启动服务
		steps = append(steps, service.Step("start", func() (string, error) {
			if err := mgr.Start(); err != nil {
				return "", err
			}
			return "服务已启动", nil
		}))
		if steps[len(steps)-1].Status == "failed" {
			return service.DeployResult{Steps: steps, FinalState: "failed"}
		}
	}

	return service.DeployResult{Steps: steps, FinalState: "success"}
}

// handleTargets 管理目标路径：GET 返回指定服务的目标路径，POST 保存。
func (h *Handler) handleTargets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		service := r.URL.Query().Get("service")
		if service == "" {
			// 返回所有
			writeJSON(w, h.targets.All())
			return
		}
		writeJSON(w, h.targets.Get(service))
	case http.MethodPost:
		var req struct {
			Service string          `json:"service"`
			Targets []targets.Target `json:"targets"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"error": "解析请求失败: " + err.Error()})
			return
		}
		if _, ok := h.cfg.FindService(req.Service); !ok {
			writeJSON(w, map[string]any{"error": "未知服务: " + req.Service})
			return
		}
		if err := h.targets.Save(req.Service, req.Targets); err != nil {
			writeJSON(w, map[string]any{"error": "保存失败: " + err.Error()})
			return
		}
		writeJSON(w, map[string]any{"status": "ok"})
	default:
		http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) history(w http.ResponseWriter, r *http.Request) {
	list := h.store.List(50)
	writeJSON(w, list)
}

func (h *Handler) control(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
		return
	}
	svcName := r.FormValue("service")
	action := r.FormValue("action")
	if _, ok := h.cfg.FindService(svcName); !ok {
		writeJSON(w, map[string]any{"error": "未知服务"})
		return
	}
	mgr := service.NewManager(svcName)
	var err error
	switch action {
	case "start":
		err = mgr.Start()
	case "stop":
		err = mgr.Stop()
	case "restart":
		err = mgr.Restart()
	default:
		writeJSON(w, map[string]any{"error": "未知操作: " + action})
		return
	}
	if err != nil {
		writeJSON(w, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}

// logEntry 日志文件信息。
type logEntry struct {
	Name       string `json:"name"`        // 文件名
	Size       int64  `json:"size"`        // 字节数
	ModTime    string `json:"mod_time"`    // 最后更新时间 RFC3339
	ModTimeMs  int64  `json:"mod_time_ms"` // 最后更新时间毫秒时间戳
	IsDir      bool   `json:"is_dir"`
}

// logs 返回 log_dir 下的日志文件列表（仅文件，不递归）。
func (h *Handler) logs(w http.ResponseWriter, r *http.Request) {
	if h.cfg.LogDir == "" {
		writeJSON(w, map[string]any{"error": "log_dir 未配置"})
		return
	}
	entries, err := os.ReadDir(h.cfg.LogDir)
	if err != nil {
		writeJSON(w, map[string]any{"error": "读取日志目录失败: " + err.Error()})
		return
	}
	var list []logEntry
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		list = append(list, logEntry{
			Name:      e.Name(),
			Size:      info.Size(),
			ModTime:   info.ModTime().Format(time.RFC3339),
			ModTimeMs: info.ModTime().UnixMilli(),
			IsDir:     false,
		})
	}
	// 按最后更新时间倒序
	sort.Slice(list, func(i, j int) bool {
		return list[i].ModTimeMs > list[j].ModTimeMs
	})
	writeJSON(w, list)
}

// safeLogPath 校验请求的文件名不会越界 log_dir。
func (h *Handler) safeLogPath(name string) (string, error) {
	if h.cfg.LogDir == "" {
		return "", fmt.Errorf("log_dir 未配置")
	}
	// 拒绝路径分隔符与绝对路径，仅允许单层文件名
	cleaned := filepath.Clean(name)
	if cleaned != name || filepath.IsAbs(name) || strings.ContainsAny(name, `/\`) || cleaned == "." || cleaned == ".." {
		return "", fmt.Errorf("非法文件名: %s", name)
	}
	abs := filepath.Join(h.cfg.LogDir, cleaned)
	absLogDir, err := filepath.Abs(h.cfg.LogDir)
	if err != nil {
		return "", err
	}
	absResolved, err := filepath.Abs(abs)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absLogDir, absResolved)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("路径越界: %s", name)
	}
	return absResolved, nil
}

// logDownload 下载指定日志文件。
func (h *Handler) logDownload(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "缺少 name 参数", http.StatusBadRequest)
		return
	}
	abs, err := h.safeLogPath(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+name+"\"")
	http.ServeFile(w, r, abs)
}

// logView 在线查看指定日志文件（text/plain）。
func (h *Handler) logView(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "缺少 name 参数", http.StatusBadRequest)
		return
	}
	abs, err := h.safeLogPath(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	http.ServeFile(w, r, abs)
}

// deployUpload 异步部署接口：接收 zip + 参数，异步执行部署，立即返回 deploy_id。
func (h *Handler) deployUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
		return
	}

	maxSize := int64(500 << 20)
	r.Body = http.MaxBytesReader(w, r.Body, maxSize)
	if err := r.ParseMultipartForm(maxSize); err != nil {
		writeJSON(w, map[string]any{"error": "文件过大或解析失败: " + err.Error()})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, map[string]any{"error": "未提供文件"})
		return
	}
	data, err := io.ReadAll(file)
	file.Close()
	if err != nil {
		writeJSON(w, map[string]any{"error": "读取文件失败"})
		return
	}

	serviceName := r.FormValue("service")
	targetDir := r.FormValue("target_dir")
	filesOnly := r.FormValue("files_only") == "true"

	if _, ok := h.cfg.FindService(serviceName); !ok {
		writeJSON(w, map[string]any{"error": "未知服务: " + serviceName})
		return
	}
	if targetDir == "" {
		writeJSON(w, map[string]any{"error": "目标路径不能为空"})
		return
	}

	isZip := strings.HasSuffix(strings.ToLower(header.Filename), ".zip")
	if isZip {
		if _, err := service.VerifyZip(data); err != nil {
			writeJSON(w, map[string]any{"error": "zip 校验失败: " + err.Error()})
			return
		}
	}

	deployID := generateUploadID()
	entry := &DeployStatusEntry{
		DeployID:  deployID,
		Status:    "pending",
		Service:   serviceName,
		TargetDir: targetDir,
		FileName:  header.Filename,
		CreatedAt: time.Now(),
	}
	h.deployStatuses.Store(deployID, entry)
	h.evictOldDeploys()

	uploadEntry := uploadEntry{
		Files: []uploadedFile{{FileName: header.Filename, Data: data, IsZip: isZip}},
	}

	go func() {
		st, ok := h.deployStatuses.Load(deployID)
		if !ok {
			return
		}
		statusEntry := st.(*DeployStatusEntry)
		statusEntry.Status = "running"

		result := h.deployMulti(serviceName, targetDir, uploadEntry, filesOnly)

		statusEntry.Status = result.FinalState
		statusEntry.Steps = result.Steps
		statusEntry.FinalState = result.FinalState

		hist := store.History{
			Operator:   "api",
			Service:    serviceName,
			FileName:   header.Filename,
			Steps:      toStoreSteps(result.Steps),
			FinalState: result.FinalState,
		}
		h.store.Add(hist)
	}()

	writeJSON(w, map[string]any{"deploy_id": deployID, "status": "pending"})
}

// deployStatus 查询异步部署状态。
func (h *Handler) deployStatus(w http.ResponseWriter, r *http.Request) {
	deployID := r.URL.Query().Get("id")
	if deployID == "" {
		writeJSON(w, map[string]any{"error": "缺少 id 参数"})
		return
	}
	st, ok := h.deployStatuses.Load(deployID)
	if !ok {
		http.Error(w, "部署记录不存在", http.StatusNotFound)
		return
	}
	writeJSON(w, st.(*DeployStatusEntry))
}

// evictOldDeploys 淘汰旧的部署记录，保留最近 100 条。
func (h *Handler) evictOldDeploys() {
	count := 0
	h.deployStatuses.Range(func(key, value any) bool {
		count++
		return true
	})
	if count > 100 {
		type kv struct {
			key string
			t   time.Time
		}
		var entries []kv
		h.deployStatuses.Range(func(key, value any) bool {
			e := value.(*DeployStatusEntry)
			entries = append(entries, kv{key: key.(string), t: e.CreatedAt})
			return true
		})
		for i := 0; i < len(entries); i++ {
			for j := i + 1; j < len(entries); j++ {
				if entries[i].t.After(entries[j].t) {
					entries[i], entries[j] = entries[j], entries[i]
				}
			}
		}
		for i := 0; i < count-100; i++ {
			h.deployStatuses.Delete(entries[i].key)
		}
	}
}

func toStoreSteps(in []service.StepResult) []store.StepResult {
	out := make([]store.StepResult, len(in))
	for i, s := range in {
		out[i] = store.StepResult{
			Name:     s.Name,
			Status:   s.Status,
			Message:  s.Message,
			Duration: s.Duration,
		}
	}
	return out
}

func generateUploadID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// ParseServiceNames 用于测试辅助。
func ParseServiceNames(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}
