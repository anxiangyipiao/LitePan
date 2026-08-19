package driver

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"litepan/internal/domain"
)

// Manager 管理并复用静态配置未变化的驱动实例。
type Manager struct {
	repo          domain.AccountRepository
	authStates    domain.AuthStateRepository
	settings      domain.ConfigRepository
	log           *slog.Logger
	delays        *DelayController
	onAuthPersist func(ctx context.Context, accountID int64)

	mu        sync.Mutex
	instances map[int64]*managedInstance
}

type managedInstance struct {
	drv        Driver
	configHash string
	rawConfig  string
	lastUsed   time.Time
}

func NewManager(repo domain.AccountRepository, authStates domain.AuthStateRepository, settings domain.ConfigRepository, log *slog.Logger) *Manager {
	if log == nil {
		log = slog.Default()
	}
	return &Manager{
		repo:       repo,
		authStates: authStates,
		settings:   settings,
		log:        log,
		delays:     NewDelayController(),
		instances:  make(map[int64]*managedInstance),
	}
}

// SetAuthPersistHook 注册认证凭证写回后的回调（认证子系统用于同步状态机）。
func (m *Manager) SetAuthPersistHook(fn func(ctx context.Context, accountID int64)) {
	m.onAuthPersist = fn
}

// Get 返回账号对应的驱动实例：配置未变则复用，变了则重建。
func (m *Manager) Get(ctx context.Context, accountID int64) (Driver, error) {
	acc, err := m.repo.Get(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if !acc.IsActive {
		return nil, domain.Errorf(domain.CodeValidation, "账号已停用")
	}

	m.mu.Lock()
	if inst, ok := m.instances[accountID]; ok && inst.rawConfig == acc.Config {
		inst.lastUsed = time.Now()
		drv := inst.drv
		m.mu.Unlock()
		return drv, nil
	}
	m.mu.Unlock()

	hash := configHash(acc.Config)

	m.mu.Lock()
	if inst, ok := m.instances[accountID]; ok && inst.configHash == hash {
		inst.lastUsed = time.Now()
		drv := inst.drv
		m.mu.Unlock()
		return drv, nil
	}
	var stale Driver
	if inst, ok := m.instances[accountID]; ok {
		stale = inst.drv
		delete(m.instances, accountID)
	}
	m.mu.Unlock()
	if stale != nil {
		_ = stale.Drop(ctx)
	}

	drv, err := m.buildDriver(ctx, accountID, acc, hash)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if inst, ok := m.instances[accountID]; ok && inst.configHash == hash {
		_ = drv.Drop(ctx)
		inst.lastUsed = time.Now()
		inst.rawConfig = acc.Config
		return inst.drv, nil
	}
	m.instances[accountID] = &managedInstance{drv: drv, configHash: hash, rawConfig: acc.Config, lastUsed: time.Now()}
	return drv, nil
}

func (m *Manager) buildDriver(ctx context.Context, accountID int64, acc *domain.Account, hash string) (Driver, error) {
	drv, ok := New(acc.DriverType)
	if !ok {
		return nil, domain.Errorf(domain.CodeValidation, "未知驱动类型：%s", acc.DriverType)
	}
	if err := applyConfigJSON(drv, acc.Config); err != nil {
		return nil, err
	}
	applyOAuth(ctx, drv, m.oauthServerURL)
	if c, ok := drv.(RequestIntervalConsumer); ok {
		c.SetRequestIntervalGate(m.delays.Gate(accountID))
	}
	if c, ok := drv.(AccountResolverConsumer); ok {
		c.SetAccountResolver(AccountRefResolver{
			ByName: m.resolveAccountByName,
			ByID:   m.resolveAccountByID,
		})
	}
	m.injectAuth(ctx, accountID, drv)
	if err := drv.Init(ctx); err != nil {
		_ = drv.Drop(ctx)
		return nil, err
	}
	return drv, nil
}

// ResetTransport 关闭账号当前驱动实例的 idle 连接，便于断网/睡眠后恢复；不丢弃实例。
func (m *Manager) ResetTransport(ctx context.Context, accountID int64) {
	m.mu.Lock()
	inst, ok := m.instances[accountID]
	m.mu.Unlock()
	if !ok {
		return
	}
	_ = inst.drv.Drop(ctx)
	m.log.Debug("驱动传输层已重置", "account", accountID)
}

// oauthServerURL 取全局 OAuth 代理地址：系统设置优先，无效值回落默认值。
func (m *Manager) oauthServerURL(ctx context.Context) string {
	if m.settings != nil {
		if v, ok, _ := m.settings.Get(ctx, domain.SettingOAuthServerURL); ok {
			return domain.NormalizeOAuthServerURL(v)
		}
	}
	return domain.NormalizeOAuthServerURL("")
}

// resolveAccountByID 按账号 ID 获取驱动实例。
func (m *Manager) resolveAccountByID(ctx context.Context, accountID int64) (Driver, error) {
	return m.Get(ctx, accountID)
}

// resolveAccountByName 按账号名（大小写不敏感）解析另一个账号的驱动实例，供 alias 等聚合驱动使用。
func (m *Manager) resolveAccountByName(ctx context.Context, name string) (int64, Driver, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, nil, domain.Errorf(domain.CodeValidation, "别名目标账号名为空")
	}
	accounts, err := m.repo.List(ctx)
	if err != nil {
		return 0, nil, err
	}
	lower := strings.ToLower(name)
	for _, a := range accounts {
		if !a.IsActive || strings.ToLower(strings.TrimSpace(a.Name)) != lower {
			continue
		}
		drv, err := m.Get(ctx, a.ID)
		if err != nil {
			return 0, nil, err
		}
		return a.ID, drv, nil
	}
	return 0, nil, domain.Errorf(domain.CodeNotFound, "找不到别名目标账号：%s", name)
}

func (m *Manager) injectAuth(ctx context.Context, accountID int64, drv Driver) {
	var st *domain.AuthState
	if m.authStates != nil {
		got, err := m.authStates.Get(ctx, accountID)
		if err == nil {
			st = got
		} else if ae, ok := domain.AsAppError(err); !ok || ae.Code != domain.CodeNotFound {
			m.log.Warn("加载认证状态失败", "account", accountID, "err", err)
		}
	}
	if c, ok := drv.(AuthCredentialConsumer); ok {
		c.SetAuthCredentials(domain.CredentialsFromState(st))
	}
	if c, ok := drv.(AuthPersistConsumer); ok {
		c.SetAuthPersister(func(ctx context.Context, creds domain.AuthCredentials) error {
			return m.persistAuth(ctx, accountID, creds)
		})
	}
}

func (m *Manager) persistAuth(ctx context.Context, accountID int64, creds domain.AuthCredentials) error {
	if m.authStates == nil {
		return nil
	}
	var existing *domain.AuthState
	got, err := m.authStates.Get(ctx, accountID)
	if err == nil {
		existing = got
	} else if ae, ok := domain.AsAppError(err); !ok || ae.Code != domain.CodeNotFound {
		return err
	}
	st := domain.MergeAuthCredentials(existing, creds)
	st.AccountID = accountID
	if err := m.authStates.Upsert(ctx, st); err != nil {
		return err
	}
	if m.onAuthPersist != nil {
		m.onAuthPersist(ctx, accountID)
	}
	return nil
}

// Drop 丢弃某账号的缓存实例（账号配置变更/删除时调用）。
func (m *Manager) Drop(ctx context.Context, accountID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if inst, ok := m.instances[accountID]; ok {
		_ = inst.drv.Drop(ctx)
		delete(m.instances, accountID)
	}
	m.delays.DropAccount(accountID)
}

// Close 丢弃所有实例，用于优雅关闭。
func (m *Manager) Close(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, inst := range m.instances {
		_ = inst.drv.Drop(ctx)
		delete(m.instances, id)
	}
}

// configHash 对静态配置生成不受 map 键顺序影响的稳定哈希。
func configHash(cfg string) string {
	s := strings.TrimSpace(cfg)
	if s == "" || s == "{}" {
		return "empty"
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return fmt.Sprintf("%x", md5.Sum([]byte(s)))
	}
	b, _ := json.Marshal(m)
	return fmt.Sprintf("%x", md5.Sum(b))
}
