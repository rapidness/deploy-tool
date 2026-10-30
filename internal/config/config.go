// Package config 负责加载与管理服务端配置文件。
package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config 服务端配置结构。
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Auth      AuthConfig      `yaml:"auth"`
	RootDir   string          `yaml:"root_dir"`
	BasePath  string          `yaml:"base_path"` // URL 前缀，如 /update-module
	LogDir    string          `yaml:"log_dir"`   // 日志文件目录（供前端浏览/下载）
	Services  []ServiceEntry  `yaml:"services"`
	NodeName  string          `yaml:"node_name"` // 本节点名称，需与 cluster.nodes 中某项一致
	Cluster   ClusterConfig   `yaml:"cluster"`
	Log       LogConfig       `yaml:"log"`
	RateLimit RateLimitConfig `yaml:"rate_limit"`
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
	APIToken string `yaml:"api_token"`
}

// ServiceEntry 被管理的 Windows 服务条目。
type ServiceEntry struct {
	Name        string `yaml:"name"`
	DisplayName string `yaml:"display_name"`
}

// ClusterConfig 多节点部署配置。
type ClusterConfig struct {
	Nodes []ClusterNode `yaml:"nodes"`
}

// ClusterNode 一个参与批量更新的节点。
type ClusterNode struct {
	Name     string `yaml:"name"`
	URL      string `yaml:"url"`       // 节点内网访问地址，可含 base_path，如 http://10.0.0.12:8080/update
	APIToken string `yaml:"api_token"` // 对方节点 auth.api_token，用于节点间调用
}

// LogConfig 日志配置。
type LogConfig struct {
	Path        string `yaml:"path"`
	MaxSizeMB   int    `yaml:"max_size_mb"`
	MaxBackups  int    `yaml:"max_backups"`
	MaxAgeDays  int    `yaml:"max_age_days"`
}

// RateLimitConfig 请求速率限制配置。
type RateLimitConfig struct {
	Enabled     bool `yaml:"enabled"`
	MaxRequests int  `yaml:"max_requests"`
	PerIP       bool `yaml:"per_ip"`
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
	// 规范化 BasePath：确保以 / 开头，去除尾部 /
	bp := strings.TrimRight(c.BasePath, "/")
	if bp != "" && !strings.HasPrefix(bp, "/") {
		bp = "/" + bp
	}
	c.BasePath = bp
	if len(c.Services) == 0 {
		return fmt.Errorf("services 不能为空")
	}
	for i, s := range c.Services {
		if s.Name == "" {
			return fmt.Errorf("services[%d].name 不能为空", i)
		}
	}
	// 校验并规范化集群节点配置
	c.NodeName = strings.TrimSpace(c.NodeName)
	names := make(map[string]bool, len(c.Cluster.Nodes))
	for i := range c.Cluster.Nodes {
		n := &c.Cluster.Nodes[i]
		n.Name = strings.TrimSpace(n.Name)
		n.URL = strings.TrimRight(strings.TrimSpace(n.URL), "/")
		if n.Name == "" {
			return fmt.Errorf("cluster.nodes[%d].name 不能为空", i)
		}
		if names[n.Name] {
			return fmt.Errorf("cluster.nodes 中节点名称重复: %s", n.Name)
		}
		names[n.Name] = true
		if n.URL == "" {
			return fmt.Errorf("cluster.nodes[%s].url 不能为空", n.Name)
		}
		u, err := url.Parse(n.URL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("cluster.nodes[%s].url 非法: %s", n.Name, n.URL)
		}
	}
	if c.NodeName != "" && len(c.Cluster.Nodes) > 0 && !names[c.NodeName] {
		return fmt.Errorf("node_name %q 未在 cluster.nodes 中配置", c.NodeName)
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
	// 校验并规范化 LogDir（日志文件浏览目录）
	if c.LogDir != "" {
		absLog, err := filepath.Abs(c.LogDir)
		if err != nil {
			return fmt.Errorf("log_dir 路径解析失败: %w", err)
		}
		info, err := os.Stat(absLog)
		if err != nil {
			return fmt.Errorf("log_dir 不存在: %w", err)
		}
		if !info.IsDir() {
			return fmt.Errorf("log_dir 不是目录: %s", absLog)
		}
		c.LogDir = absLog
	}
	if c.RateLimit.Enabled && c.RateLimit.MaxRequests <= 0 {
		c.RateLimit.MaxRequests = 60
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

// SelfName 返回本节点名称，未配置 node_name 时返回"本机"。
func (c *Config) SelfName() string {
	if c.NodeName != "" {
		return c.NodeName
	}
	return "本机"
}

// IsSelf 判断节点名称是否为本机。
func (c *Config) IsSelf(name string) bool {
	return name == c.SelfName()
}

// FindNode 根据名称查找集群节点，找不到返回 false。
func (c *Config) FindNode(name string) (ClusterNode, bool) {
	for _, n := range c.Cluster.Nodes {
		if n.Name == name {
			return n, true
		}
	}
	return ClusterNode{}, false
}

// NodeNames 返回所有节点名称（按配置顺序）；未配置集群时返回本机。
func (c *Config) NodeNames() []string {
	if len(c.Cluster.Nodes) == 0 {
		return []string{c.SelfName()}
	}
	names := make([]string, 0, len(c.Cluster.Nodes))
	for _, n := range c.Cluster.Nodes {
		names = append(names, n.Name)
	}
	return names
}
