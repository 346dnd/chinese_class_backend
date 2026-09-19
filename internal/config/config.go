// Package config 负责加载应用配置。
// 配置来源优先级：环境变量 > config.yaml。
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// Config 应用配置根结构
type Config struct {
	Server   ServerConfig   `yaml:"server" koanf:"server"`
	Database DatabaseConfig `yaml:"database" koanf:"database"`
	JWT      JWTConfig      `yaml:"jwt" koanf:"jwt"`
	AI       AIConfig       `yaml:"ai" koanf:"ai"`
	OCR      OCRConfig      `yaml:"ocr" koanf:"ocr"`
	STT      STTConfig      `yaml:"stt" koanf:"stt"`
	TTS      TTSConfig      `yaml:"tts" koanf:"tts"`
	Prompts  PromptsConfig  `yaml:"prompts" koanf:"prompts"`
	Log      LogConfig      `yaml:"log" koanf:"log"`
}

type ServerConfig struct {
	Host string `yaml:"host" koanf:"host"`
	Port int    `yaml:"port" koanf:"port"`
	// UploadDir 文件上传保存目录（相对工作目录），可被环境变量 SERVER__UPLOAD_DIR 覆盖
	UploadDir string `yaml:"upload_dir" koanf:"upload_dir"`
	// StorageURL 上传资源对外访问的 URL 前缀（静态服务映射，如 Nginx location /storage/ 指向 upload_dir），可被环境变量 SERVER__STORAGE_URL 覆盖
	StorageURL string `yaml:"storage_url" koanf:"storage_url"`
}

type DatabaseConfig struct {
	Host     string `yaml:"host" koanf:"host"`
	Port     int    `yaml:"port" koanf:"port"`
	User     string `yaml:"user" koanf:"user"`
	Password string `yaml:"password" koanf:"password"`
	Name     string `yaml:"name" koanf:"name"`
	SSLMode  string `yaml:"ssl_mode" koanf:"ssl_mode"`
	MaxConns int    `yaml:"max_conns" koanf:"max_conns"`
}

type JWTConfig struct {
	Secret      string `yaml:"secret" koanf:"secret"`
	ExpireHours int    `yaml:"expire_hours" koanf:"expire_hours"`
}

// AIConfig LLM 服务配置，支持 OpenAI 兼容接口。
type AIConfig struct {
	Provider string `yaml:"provider" koanf:"provider"` // openai / qwen / custom
	APIKey   string `yaml:"api_key" koanf:"api_key"`
	BaseURL  string `yaml:"base_url" koanf:"base_url"` // 自定义端点（可选）
	Model    string `yaml:"model" koanf:"model"`       // 模型名称
}

type LogConfig struct {
	Level    string `yaml:"level" koanf:"level"`
	Encoding string `yaml:"encoding" koanf:"encoding"`
}

// OCRConfig 百度智能云 OCR 配置。
type OCRConfig struct {
	APIKey    string `yaml:"api_key" koanf:"api_key"`     // API Key（建议通过环境变量 OCR__API_KEY 设置）
	SecretKey string `yaml:"secret_key" koanf:"secret_key"` // Secret Key（建议通过环境变量 OCR__SECRET_KEY 设置）
}

// STTConfig 百度智能云语音识别（短语音识别 server_api）配置。
type STTConfig struct {
	APIKey    string `yaml:"api_key" koanf:"api_key"`     // API Key（建议通过环境变量 STT__API_KEY 设置）
	SecretKey string `yaml:"secret_key" koanf:"secret_key"` // Secret Key（建议通过环境变量 STT__SECRET_KEY 设置）
}

// TTSConfig 百度智能云语音合成（text2audio）配置。
type TTSConfig struct {
	APIKey    string `yaml:"api_key" koanf:"api_key"`     // API Key（建议通过环境变量 TTS__API_KEY 设置）
	SecretKey string `yaml:"secret_key" koanf:"secret_key"` // Secret Key（建议通过环境变量 TTS__SECRET_KEY 设置）
}

// PromptsConfig AI 提示词配置。
// 提示词优先从 prompts.dir 目录加载，内联配置作为兜底。
type PromptsConfig struct {
	Default string            `yaml:"default" koanf:"default"` // 通用兜底提示词
	Dir     string            `yaml:"dir" koanf:"dir"`         // 提示词文件目录（可选）
	Types   map[string]string `yaml:"types" koanf:"types"`     // 内联提示词（兜底，优先级低于文件）
}

// Load 从指定路径加载 YAML 配置，再用环境变量覆盖。
// 环境变量以双下划线 `__` 作为分隔符，前缀为空，例如 `DATABASE__HOST` 覆盖 database.host。
func Load(path string) (*Config, error) {
	k := koanf.New(".")

	if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
		return nil, fmt.Errorf("load yaml config: %w", err)
	}

	if err := k.Load(env.Provider("", "__", func(s string) string {
		return strings.ToLower(s)
	}), nil); err != nil {
		return nil, fmt.Errorf("load env config: %w", err)
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &cfg, nil
}

// LoadPromptsFromDir 从 prompts.dir 目录加载提示词文件，覆盖到 Types 中。
// 文件名（不含扩展名）作为 Types 的 key，文件内容作为 value。
// 目录不存在或为空时静默跳过（不报错）。
func (p *PromptsConfig) LoadPromptsFromDir() {
	if p.Dir == "" {
		return
	}
	entries, err := os.ReadDir(p.Dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return
		}
		// 其他错误（如权限）静默跳过
		return
	}
	if p.Types == nil {
		p.Types = make(map[string]string, len(entries))
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := filepath.Ext(name)
		key := strings.TrimSuffix(name, ext)
		if key == "" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(p.Dir, name))
		if err != nil {
			continue
		}
		p.Types[key] = string(data)
	}
}
