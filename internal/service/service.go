// Package service 实现业务逻辑：Windows 服务启停、zip 解压覆盖、部署流程编排。
package service

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ServiceState 服务状态。
type ServiceState string

const (
	StateRunning ServiceState = "running"
	StateStopped ServiceState = "stopped"
	StateUnknown ServiceState = "unknown"
)

// Manager 管理一个 Windows 服务（通过 sc.exe）。
type Manager struct {
	Name string
}

// NewManager 创建服务管理器。
func NewManager(name string) *Manager {
	return &Manager{Name: name}
}

// Status 查询服务当前状态。
func (m *Manager) Status() (ServiceState, error) {
	out, err := exec.Command("sc", "query", m.Name).Output()
	if err != nil {
		return StateUnknown, fmt.Errorf("查询服务状态失败: %w", err)
	}
	s := string(out)
	switch {
	case strings.Contains(s, "RUNNING"):
		return StateRunning, nil
	case strings.Contains(s, "STOPPED"):
		return StateStopped, nil
	default:
		return StateUnknown, nil
	}
}

// Start 启动服务。
func (m *Manager) Start() error {
	if err := exec.Command("sc", "start", m.Name).Run(); err != nil {
		return fmt.Errorf("启动服务失败: %w", err)
	}
	return nil
}

// Stop 停止服务。
func (m *Manager) Stop() error {
	if err := exec.Command("sc", "stop", m.Name).Run(); err != nil {
		return fmt.Errorf("停止服务失败: %w", err)
	}
	return nil
}

// Restart 重启服务。
func (m *Manager) Restart() error {
	if err := m.Stop(); err != nil {
		return err
	}
	return m.Start()
}

// VerifyZip 校验 zip 数据完整性，返回条目数量。
func VerifyZip(data []byte) (int, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return 0, fmt.Errorf("zip 格式非法: %w", err)
	}
	if len(r.File) == 0 {
		return 0, fmt.Errorf("zip 内无文件")
	}
	return len(r.File), nil
}

// SafeJoin 在 rootDir 安全拼接路径，防止越界。
func SafeJoin(rootDir, rel string) (string, error) {
	cleaned := filepath.Clean(rel)
	if filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("不允许绝对路径: %s", rel)
	}
	joined := filepath.Join(rootDir, cleaned)
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return "", err
	}
	absJoined, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}
	rel2, err := filepath.Rel(absRoot, absJoined)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(rel2, "..") {
		return "", fmt.Errorf("路径越界: %s", rel)
	}
	return absJoined, nil
}

// ZipEntry 表示 zip 内一个条目的信息。
type ZipEntry struct {
	Name   string `json:"name"`    // zip 内路径
	IsDir  bool   `json:"is_dir"`
	Size   int64  `json:"size"`
	Target string `json:"target"`  // 默认映射目标（相对 rootDir 的路径，等同 name）
}

// ListZip 列出 zip 内所有条目，返回条目列表。
func ListZip(data []byte) ([]ZipEntry, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("zip 格式非法: %w", err)
	}
	entries := make([]ZipEntry, 0, len(r.File))
	for _, f := range r.File {
		entries = append(entries, ZipEntry{
			Name:   f.Name,
			IsDir:  f.FileInfo().IsDir(),
			Size:   int64(f.UncompressedSize64),
			Target: f.Name, // 默认映射 = zip 内路径
		})
	}
	return entries, nil
}

// ExtractZipWithMapping 按映射关系解压 zip 到 rootDir。
// mapping: zip 内路径 -> 目标相对路径（相对 rootDir）。若映射中无某条目，使用其原始路径。
func ExtractZipWithMapping(data []byte, rootDir string, mapping map[string]string) ([]string, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("zip 格式非法: %w", err)
	}
	written := make([]string, 0, len(r.File))
	for _, f := range r.File {
		// 确定目标相对路径：优先使用映射，否则用原始路径
		rel := f.Name
		if mapped, ok := mapping[f.Name]; ok && mapped != "" {
			rel = mapped
		}
		target, err := SafeJoin(rootDir, rel)
		if err != nil {
			return written, fmt.Errorf("路径校验失败(%s → %s): %w", f.Name, rel, err)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return written, fmt.Errorf("创建目录失败(%s): %w", target, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return written, fmt.Errorf("创建父目录失败(%s): %w", filepath.Dir(target), err)
		}
		if err := copyZipFile(f, target); err != nil {
			return written, fmt.Errorf("写入文件失败(%s): %w", target, err)
		}
		written = append(written, target)
	}
	return written, nil
}

// ExtractZip 将 zip 数据解压覆盖到 rootDir 下对应路径。
// 仅限 rootDir 内，遇到越界路径直接返回错误中止。
func ExtractZip(data []byte, rootDir string) ([]string, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("zip 格式非法: %w", err)
	}
	written := make([]string, 0, len(r.File))
	for _, f := range r.File {
		target, err := SafeJoin(rootDir, f.Name)
		if err != nil {
			return written, fmt.Errorf("路径校验失败(%s): %w", f.Name, err)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return written, fmt.Errorf("创建目录失败(%s): %w", target, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return written, fmt.Errorf("创建父目录失败(%s): %w", filepath.Dir(target), err)
		}
		if err := copyZipFile(f, target); err != nil {
			return written, fmt.Errorf("写入文件失败(%s): %w", target, err)
		}
		written = append(written, target)
	}
	return written, nil
}

func copyZipFile(f *zip.File, target string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}

// StepFunc 单个部署步骤函数，返回结果描述与错误。
type StepFunc func() (string, error)

// Step 执行一个步骤并计时。
func Step(name string, fn StepFunc) StepResult {
	start := time.Now()
	msg, err := fn()
	res := StepResult{Name: name, Duration: time.Since(start).Milliseconds()}
	if err != nil {
		res.Status = "failed"
		res.Message = err.Error()
	} else {
		res.Status = "success"
		res.Message = msg
	}
	return res
}

// StepResult 步骤执行结果。
type StepResult struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Message  string `json:"message"`
	Duration int64  `json:"duration"`
}

// FileItem 表示一个待部署的文件（非 zip）。
type FileItem struct {
	FileName string // 原始文件名
	Data     []byte // 文件内容
}

// DeployFiles 将多个非 zip 文件覆盖到 rootDir 下的指定目录。
// targetDir 为相对 rootDir 的目录路径，所有文件直接放入该目录。
func DeployFiles(items []FileItem, rootDir, targetDir string) ([]string, error) {
	written := make([]string, 0, len(items))
	for _, item := range items {
		rel := filepath.Join(targetDir, item.FileName)
		target, err := SafeJoin(rootDir, rel)
		if err != nil {
			return written, fmt.Errorf("路径校验失败(%s): %w", rel, err)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return written, fmt.Errorf("创建父目录失败(%s): %w", filepath.Dir(target), err)
		}
		if err := os.WriteFile(target, item.Data, 0o644); err != nil {
			return written, fmt.Errorf("写入文件失败(%s): %w", target, err)
		}
		written = append(written, target)
	}
	return written, nil
}

// ExtractZipToDir 将 zip 解压到 rootDir 下的指定目录。
// targetDir 为相对 rootDir 的目录路径，zip 内所有文件解压到该目录下（保留 zip 内相对结构）。
func ExtractZipToDir(data []byte, rootDir, targetDir string) ([]string, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("zip 格式非法: %w", err)
	}
	written := make([]string, 0, len(r.File))
	for _, f := range r.File {
		rel := filepath.Join(targetDir, f.Name)
		target, err := SafeJoin(rootDir, rel)
		if err != nil {
			return written, fmt.Errorf("路径校验失败(%s): %w", rel, err)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return written, fmt.Errorf("创建目录失败(%s): %w", target, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return written, fmt.Errorf("创建父目录失败(%s): %w", filepath.Dir(target), err)
		}
		if err := copyZipFile(f, target); err != nil {
			return written, fmt.Errorf("写入文件失败(%s): %w", target, err)
		}
		written = append(written, target)
	}
	return written, nil
}

// DeployInput 部署流程输入。
type DeployInput struct {
	ServiceName string
	ZipData     []byte
	RootDir     string
	Mapping     map[string]string // 可选：zip 内路径 -> 目标相对路径
	IsZip       bool              // true=zip 压缩包, false=单个文件
	FileName    string            // 单个文件模式下的文件名
}

// DeployResult 部署流程结果。
type DeployResult struct {
	Steps      []StepResult
	FinalState string // success / failed
}

// DeployFile 将单个文件写入 rootDir 下的目标路径。
func DeployFile(data []byte, rootDir, targetRel string) (string, error) {
	target, err := SafeJoin(rootDir, targetRel)
	if err != nil {
		return "", fmt.Errorf("路径校验失败(%s): %w", targetRel, err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", fmt.Errorf("创建父目录失败(%s): %w", filepath.Dir(target), err)
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		return "", fmt.Errorf("写入文件失败(%s): %w", target, err)
	}
	return target, nil
}

// Deploy 执行核心部署流程：停服务 → 解压覆盖 → 起服务。
// 任一步骤失败立即中止。若 Mapping 非空则按映射解压，否则按原始路径解压。
func Deploy(in DeployInput) DeployResult {
	mgr := NewManager(in.ServiceName)
	var steps []StepResult

	// 1. 停止服务（已停止则跳过）
	steps = append(steps, Step("stop", func() (string, error) {
		st, err := mgr.Status()
		if err != nil {
			return "", err
		}
		if st == StateStopped {
			return "服务已停止，跳过", nil
		}
		if err := mgr.Stop(); err != nil {
			return "", err
		}
		return "服务已停止", nil
	}))
	if steps[len(steps)-1].Status == "failed" {
		return DeployResult{Steps: steps, FinalState: "failed"}
	}

	// 2. 部署文件（zip 按映射或原始路径解压；单个文件直接写入）
	steps = append(steps, Step("deploy", func() (string, error) {
		if in.IsZip {
			var written []string
			var err error
			if len(in.Mapping) > 0 {
				written, err = ExtractZipWithMapping(in.ZipData, in.RootDir, in.Mapping)
			} else {
				written, err = ExtractZip(in.ZipData, in.RootDir)
			}
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("已覆盖 %d 个文件", len(written)), nil
		}
		// 单个文件模式：映射中 key=文件名, value=目标路径；若无映射则用文件名本身
		targetRel := in.FileName
		if m, ok := in.Mapping[in.FileName]; ok && m != "" {
			targetRel = m
		}
		_, err := DeployFile(in.ZipData, in.RootDir, targetRel)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("已覆盖 1 个文件: %s", targetRel), nil
	}))
	if steps[len(steps)-1].Status == "failed" {
		return DeployResult{Steps: steps, FinalState: "failed"}
	}

	// 3. 启动服务
	steps = append(steps, Step("start", func() (string, error) {
		if err := mgr.Start(); err != nil {
			return "", err
		}
		return "服务已启动", nil
	}))
	if steps[len(steps)-1].Status == "failed" {
		return DeployResult{Steps: steps, FinalState: "failed"}
	}

	return DeployResult{Steps: steps, FinalState: "success"}
}
