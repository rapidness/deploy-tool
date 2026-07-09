// Package config 负责加载与管理服务端配置文件。
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config 服务端配置结构。
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Auth     AuthConfig     `yaml:"auth"`
	RootDir  string         `yaml:"root_dir"`
	Services []ServiceEntry `yaml:"services"`
	Log      LogConfig      `yaml:"log"`
}

// ServerConfig HTTP 服务监听配置。
type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

// AuthConfig 登录认证配置。
type AuthConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"` // 哈希后的密码
}

// ServiceEntry 被管理的 Windows 服务条目。
type ServiceEntry struct {
	Name        string `yaml:"name"`
	DisplayName string `yaml:"display_name"`
}

// LogConfig 日志配置。
type LogConfig struct {
	Path        string `yaml:"path"`
	MaxSizeMB   int    `yaml:"max_size_mb"`
	MaxBackups  int    `yaml:"max_backups"`
	MaxAgeDays  int    `yaml:"max_age_days"`
}

// Load 从指定路径加载配置文件并校验。
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// Validate 校验配置合法性。
func (c *Config) Validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("监听端口非法: %d", c.Server.Port)
	}
	if c.Auth.Username == "" || c.Auth.Password == "" {
		return fmt.Errorf("认证信息不能为空")
	}
	if c.RootDir == "" {
		return fmt.Errorf("root_dir 不能为空")
	}
	abs, err := filepath.Abs(c.RootDir)
	if err != nil {
		return fmt.Errorf("root_dir 路径解析失败: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("root_dir 不存在: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("root_dir 不是目录: %s", abs)
	}
	c.RootDir = abs
	if len(c.Services) == 0 {
		return fmt.Errorf("services 不能为空")
	}
	for i, s := range c.Services {
		if s.Name == "" {
			return fmt.Errorf("services[%d].name 不能为空", i)
		}
	}
	if c.Log.Path == "" {
		c.Log.Path = "logs"
	}
	if c.Log.MaxSizeMB <= 0 {
		c.Log.MaxSizeMB = 10
	}
	if c.Log.MaxBackups < 0 {
		c.Log.MaxBackups = 5
	}
	if c.Log.MaxAgeDays < 0 {
		c.Log.MaxAgeDays = 30
	}
	return nil
}

// ServiceNames 返回所有被管理的服务名称。
func (c *Config) ServiceNames() []string {
	names := make([]string, 0, len(c.Services))
	for _, s := range c.Services {
		names = append(names, s.Name)
	}
	return names
}

// FindService 根据名称查找服务条目，找不到返回 false。
func (c *Config) FindService(name string) (ServiceEntry, bool) {
	for _, s := range c.Services {
		if s.Name == name {
			return s, true
		}
	}
	return ServiceEntry{}, false
}
