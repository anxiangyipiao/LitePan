package qb

import (
	"context"
	"log/slog"
	"strings"

	"litepan/internal/domain"
	"litepan/internal/settings"
)

// Settings 是 qBittorrent 下载工具的连接配置。
type Settings struct {
	Enabled  bool   `json:"enabled"`
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
	SavePath string `json:"save_path"`
}

type Service struct {
	settings *settings.Service
	log      *slog.Logger
}

func NewService(st *settings.Service, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{settings: st, log: log}
}

func (s *Service) GetSettings() Settings {
	if s.settings == nil {
		return Settings{}
	}
	return Settings{
		Enabled:  s.settings.Bool(settings.KeyQBEnabled),
		URL:      strings.TrimSpace(s.settings.String(settings.KeyQBURL)),
		Username: strings.TrimSpace(s.settings.String(settings.KeyQBUsername)),
		Password: s.settings.String(settings.KeyQBPassword),
		SavePath: strings.TrimSpace(s.settings.String(settings.KeyQBSavePath)),
	}
}

func (s *Service) UpdateSettings(ctx context.Context, in Settings) error {
	if s.settings == nil {
		return domain.Errorf(domain.CodeInternal, "设置服务未装配")
	}
	payload := map[string]string{
		settings.KeyQBEnabled:  boolStr(in.Enabled),
		settings.KeyQBURL:      strings.TrimSpace(in.URL),
		settings.KeyQBUsername: strings.TrimSpace(in.Username),
		settings.KeyQBSavePath: strings.TrimSpace(in.SavePath),
	}
	// 密码留空表示不修改（Get 返回时已屏蔽）
	if pwd := in.Password; pwd != "" {
		payload[settings.KeyQBPassword] = pwd
	}
	return s.settings.Update(ctx, payload)
}

// Test 测试 qB WebUI 连通性，返回版本号。
func (s *Service) Test(ctx context.Context) (map[string]any, error) {
	cfg := s.GetSettings()
	client, err := s.client(cfg)
	if err != nil {
		return nil, err
	}
	version, err := client.Test(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "version": version, "url": cfg.URL}, nil
}

// Add 添加磁链/链接到 qB。savePath 为空时使用 qB 设置里的默认保存目录。
func (s *Service) Add(ctx context.Context, urls []string, savePath string) error {
	cfg := s.GetSettings()
	client, err := s.client(cfg)
	if err != nil {
		return err
	}
	if strings.TrimSpace(savePath) == "" {
		savePath = cfg.SavePath
	}
	return client.Add(ctx, urls, savePath)
}

func (s *Service) List(ctx context.Context) ([]Task, error) {
	cfg := s.GetSettings()
	client, err := s.client(cfg)
	if err != nil {
		return nil, err
	}
	return client.List(ctx)
}

func (s *Service) Delete(ctx context.Context, hashes []string, deleteFiles bool) error {
	cfg := s.GetSettings()
	client, err := s.client(cfg)
	if err != nil {
		return err
	}
	return client.Delete(ctx, hashes, deleteFiles)
}

func (s *Service) client(cfg Settings) (*Client, error) {
	if !cfg.Enabled {
		return nil, domain.Errorf(domain.CodeValidation, "未启用 qBittorrent 下载，请先在 qb下载 面板开启并配置")
	}
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, domain.Errorf(domain.CodeValidation, "未配置 qBittorrent WebUI 地址")
	}
	return NewClient(Options{
		BaseURL:  cfg.URL,
		Username: cfg.Username,
		Password: cfg.Password,
	}), nil
}

func boolStr(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
