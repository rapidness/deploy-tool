// Package store 负责操作历史记录与审计日志的持久化。
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// StepResult 单个步骤执行结果。
type StepResult struct {
	Name     string `json:"name"`      // 步骤名称
	Status   string `json:"status"`    // success / failed / skipped
	Message  string `json:"message"`   // 结果描述
	Duration int64  `json:"duration"`  // 耗时(毫秒)
}

// History 单次更新操作的历史记录。
type History struct {
	ID         int64        `json:"id"`
	Time       time.Time    `json:"time"`
	Operator   string       `json:"operator"`
	Service    string       `json:"service"`
	Node       string       `json:"node"` // 执行节点名称
	FileName   string       `json:"file_name"`
	Steps      []StepResult `json:"steps"`
	FinalState string       `json:"final_state"` // success / failed
}

// Store 历史记录存储，基于本地 JSON 文件。
type Store struct {
	mu       sync.Mutex
	filePath string
	nextID   int64
	items    []History
}

// New 创建历史记录存储，filePath 为 JSON 文件路径。
func New(filePath string) (*Store, error) {
	s := &Store{filePath: filePath}
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return nil, fmt.Errorf("创建历史目录失败: %w", err)
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
		return fmt.Errorf("读取历史文件失败: %w", err)
	}
	if len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, &s.items); err != nil {
		return fmt.Errorf("解析历史文件失败: %w", err)
	}
	for _, h := range s.items {
		if h.ID > s.nextID {
			s.nextID = h.ID
		}
	}
	return nil
}

func (s *Store) save() error {
	data, err := json.MarshalIndent(s.items, "", "  ")
	if err != nil {
		return fmt.Errorf("编码历史失败: %w", err)
	}
	if err := os.WriteFile(s.filePath, data, 0o644); err != nil {
		return fmt.Errorf("写入历史文件失败: %w", err)
	}
	return nil
}

// Add 记录一次操作并返回其 ID。
func (s *Store) Add(h History) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	h.ID = s.nextID
	if h.Time.IsZero() {
		h.Time = time.Now()
	}
	s.items = append(s.items, h)
	if err := s.save(); err != nil {
		return 0, err
	}
	return h.ID, nil
}

// List 返回最近 limit 条历史记录（limit<=0 表示全部），按时间倒序。
func (s *Store) List(limit int) []History {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]History, len(s.items))
	copy(result, s.items)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Time.After(result[j].Time)
	})
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result
}

// Get 按 ID 查询历史详情。
func (s *Store) Get(id int64) (History, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, h := range s.items {
		if h.ID == id {
			return h, true
		}
	}
	return History{}, false
}
