// Package targets 负责按服务持久化部署目标路径列表（JSON 文件）。
package targets

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// Target 单个部署目标路径条目。
type Target struct {
	Path         string `json:"path"`          // 相对 rootDir 的目录路径
	Desc         string `json:"desc"`          // 可选描述
	SourceDir    string `json:"source_dir"`    // 可选：服务器本机更新文件目录
	DeleteSource bool   `json:"delete_source"` // 可选：更新成功后删除源目录本次读取的文件
}

// Store 按服务存储目标路径列表，基于本地 JSON 文件。
type Store struct {
	mu       sync.Mutex
	filePath string
	data     map[string][]Target // service -> targets
}

// New 创建目标路径存储。filePath 为 JSON 文件路径。
func New(filePath string) (*Store, error) {
	s := &Store{filePath: filePath, data: make(map[string][]Target)}
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return nil, fmt.Errorf("创建目标路径目录失败: %w", err)
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("读取目标路径文件失败: %w", err)
	}
	if len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, &s.data); err != nil {
		return fmt.Errorf("解析目标路径文件失败: %w", err)
	}
	if s.data == nil {
		s.data = make(map[string][]Target)
	}
	return nil
}

func (s *Store) save() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("编码目标路径失败: %w", err)
	}
	if err := os.WriteFile(s.filePath, data, 0o644); err != nil {
		return fmt.Errorf("写入目标路径文件失败: %w", err)
	}
	return nil
}

// All 返回所有服务的目标路径映射（副本）。
func (s *Store) All() map[string][]Target {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string][]Target, len(s.data))
	for k, v := range s.data {
		cp := make([]Target, len(v))
		copy(cp, v)
		result[k] = cp
	}
	return result
}

// Get 返回指定服务的目标路径列表（副本）。
func (s *Store) Get(service string) []Target {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.data[service]
	cp := make([]Target, len(v))
	copy(cp, v)
	return cp
}

// Save 覆盖保存指定服务的目标路径列表。
func (s *Store) Save(service string, targets []Target) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 去重 & 去空
	seen := make(map[string]bool)
	cleaned := make([]Target, 0, len(targets))
	for _, t := range targets {
		if t.Path == "" || seen[t.Path] {
			continue
		}
		seen[t.Path] = true
		cleaned = append(cleaned, t)
	}
	sort.SliceStable(cleaned, func(i, j int) bool {
		return cleaned[i].Path < cleaned[j].Path
	})
	s.data[service] = cleaned
	return s.save()
}
