package favorites

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const fileName = "litepan_favorites.json"

type Crumb struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Item 一条收藏。收藏夹全局共享（跨账号一份列表），AccountID 记录该收藏所属的存储盘。
type Item struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	AccountID int64   `json:"account_id"`
	Crumbs    []Crumb `json:"crumbs"`
}

// State 全局收藏夹状态。
type State struct {
	Open  bool   `json:"open"`
	Items []Item `json:"items"`
}

// snapshot 落盘结构。Version 2 为全局收藏；旧版（Version 1）为按账号结构，读取时迁移。
type snapshot struct {
	Version int    `json:"version"`
	Open    bool   `json:"open"`
	Items   []Item `json:"items"`
}

// rawSnapshot 兼容新老两种格式解析。
type rawSnapshot struct {
	Version  int                     `json:"version"`
	Open     bool                    `json:"open"`
	Items    []Item                  `json:"items"`
	Accounts map[string]accountState `json:"accounts"`
}

type accountState struct {
	Open  bool   `json:"open"`
	Items []Item `json:"items"`
}

type Service struct {
	path string
	log  *slog.Logger
	mu   sync.Mutex
}

func NewService(dbPath string, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		path: filepath.Join(filepath.Dir(dbPath), fileName),
		log:  log,
	}
}

func (s *Service) Get(ctx context.Context) (State, error) {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.readUnlocked()
	if err != nil {
		return State{}, err
	}
	return cloneState(stateOf(data)), nil
}

func (s *Service) Put(ctx context.Context, state State) (State, error) {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.readUnlocked()
	if err != nil {
		return State{}, err
	}
	clean := sanitizeState(state)
	data.Open = clean.Open
	data.Items = clean.Items
	if err := s.writeUnlocked(data); err != nil {
		return State{}, err
	}
	return cloneState(clean), nil
}

// Delete 移除指定账号的全部收藏（账号删除时清理）；该账号无收藏时不改写文件。
func (s *Service) Delete(ctx context.Context, accountID int64) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.readUnlocked()
	if err != nil {
		return err
	}
	filtered := make([]Item, 0, len(data.Items))
	for _, item := range data.Items {
		if item.AccountID != accountID {
			filtered = append(filtered, item)
		}
	}
	if len(filtered) == len(data.Items) {
		return nil
	}
	data.Items = filtered
	return s.writeUnlocked(data)
}

func stateOf(data snapshot) State {
	return State{Open: data.Open, Items: data.Items}
}

func (s *Service) readUnlocked() (snapshot, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return snapshot{Version: 2, Open: true}, nil
		}
		return snapshot{}, fmt.Errorf("read favorites file: %w", err)
	}

	var rawData rawSnapshot
	if err := json.Unmarshal(raw, &rawData); err != nil {
		return s.handleCorruptedUnlocked()
	}

	data := snapshot{Version: 2, Open: rawData.Open, Items: rawData.Items}
	if len(rawData.Accounts) > 0 {
		// 旧版按账号结构 → 迁移为全局收藏（每条带所属账号）
		data = migrateLegacy(rawData.Accounts)
		if writeErr := s.writeUnlocked(data); writeErr != nil {
			s.log.Warn("收藏夹旧格式迁移落盘失败", "err", writeErr)
		}
	}
	data.Items = sanitizeItems(data.Items)
	return data, nil
}

func (s *Service) handleCorruptedUnlocked() (snapshot, error) {
	backupPath, moveErr := s.moveCorruptedFileUnlocked()
	if moveErr != nil {
		s.log.Error("收藏夹文件解析失败，转移损坏文件失败", "path", s.path, "move_err", moveErr)
		return snapshot{}, fmt.Errorf("favorites file corrupted")
	}
	s.log.Error("收藏夹文件解析失败，已转移损坏文件", "path", s.path, "backup", backupPath)
	return snapshot{}, fmt.Errorf("收藏夹文件已损坏，已转移到 %s，请检查或手动恢复", backupPath)
}

// migrateLegacy 把旧版按账号收藏合并为全局收藏：每条记录所属账号，open 取任一账号展开。
func migrateLegacy(accounts map[string]accountState) snapshot {
	out := snapshot{Version: 2, Open: true}
	var items []Item
	for key, state := range accounts {
		if state.Open {
			out.Open = true
		}
		accountID, _ := strconv.ParseInt(key, 10, 64)
		for _, item := range state.Items {
			if item.AccountID == 0 {
				item.AccountID = accountID
			}
			items = append(items, item)
		}
	}
	out.Items = sanitizeItems(items)
	return out
}

func (s *Service) moveCorruptedFileUnlocked() (string, error) {
	base := s.path + ".corrupt-" + time.Now().Format("20060102-150405")
	target := base
	for i := 1; ; i++ {
		if _, err := os.Stat(target); os.IsNotExist(err) {
			break
		}
		target = fmt.Sprintf("%s-%d", base, i)
	}
	if err := os.Rename(s.path, target); err != nil {
		return "", fmt.Errorf("rename corrupted favorites file: %w", err)
	}
	return target, nil
}

func (s *Service) writeUnlocked(data snapshot) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create favorites dir: %w", err)
	}
	data.Version = 2
	body, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal favorites: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(body, '\n'), 0o644); err != nil {
		return fmt.Errorf("write favorites tmp: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("rename favorites file: %w", err)
	}
	return nil
}

// sanitizeState 校验并规范化收藏夹状态，返回新的 State。
func sanitizeState(state State) State {
	return State{Open: state.Open, Items: sanitizeItems(state.Items)}
}

// sanitizeItems 校验单条收藏：ID/名称/所属账号必须有效，crumbs 非空，按 账号+ID 去重。
func sanitizeItems(items []Item) []Item {
	out := make([]Item, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item.ID)
		name := strings.TrimSpace(item.Name)
		if id == "" || name == "" || item.AccountID <= 0 {
			continue
		}
		key := strconv.FormatInt(item.AccountID, 10) + ":" + id
		if _, ok := seen[key]; ok {
			continue
		}
		crumbs := make([]Crumb, 0, len(item.Crumbs))
		for _, crumb := range item.Crumbs {
			crumbName := strings.TrimSpace(crumb.Name)
			if crumbName == "" {
				continue
			}
			crumbs = append(crumbs, Crumb{
				ID:   strings.TrimSpace(crumb.ID),
				Name: crumbName,
			})
		}
		if len(crumbs) == 0 {
			continue
		}
		out = append(out, Item{
			ID:        id,
			Name:      name,
			AccountID: item.AccountID,
			Crumbs:    crumbs,
		})
		seen[key] = struct{}{}
	}
	return out
}

func cloneState(state State) State {
	out := State{
		Open:  state.Open,
		Items: make([]Item, 0, len(state.Items)),
	}
	for _, item := range state.Items {
		cloned := Item{
			ID:        item.ID,
			Name:      item.Name,
			AccountID: item.AccountID,
			Crumbs:    make([]Crumb, len(item.Crumbs)),
		}
		copy(cloned.Crumbs, item.Crumbs)
		out.Items = append(out.Items, cloned)
	}
	return out
}
