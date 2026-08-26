package magnetfavorites

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const fileName = "litepan_magnet_favorites.json"

// Item 一条磁力收藏。用磁力 Hash 作为唯一标识。
type Item struct {
	Hash      string `json:"hash"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	Seeders   int    `json:"seeders"`
	Leechers  int    `json:"leechers"`
	Date      int64  `json:"date"`
	Category  string `json:"category,omitempty"`
	Magnet    string `json:"magnet"`
	ViewURL   string `json:"view_url,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

// State 收藏夹状态。
type State struct {
	Items []Item `json:"items"`
}

type snapshot struct {
	Version int    `json:"version"`
	Items   []Item `json:"items"`
}

// hashPattern 校验 btih 哈希为 40 位 hex。
var hashPattern = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

// magnetPrefix btih 磁力链标准前缀。
const magnetPrefix = "magnet:?xt=urn:btih:"

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

func (s *Service) Add(ctx context.Context, item Item) (State, error) {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()

	clean, ok := sanitizeItem(item)
	if !ok {
		return State{}, fmt.Errorf("收藏内容不合法：hash 必须为 40 位 hex，magnet 必须以 %s 开头，name 必填", magnetPrefix)
	}

	data, err := s.readUnlocked()
	if err != nil {
		return State{}, err
	}

	for _, existing := range data.Items {
		if existing.Hash == clean.Hash {
			return cloneState(stateOf(data)), nil
		}
	}

	clean.CreatedAt = time.Now().Unix()
	data.Items = append(data.Items, clean)
	sortByCreatedDesc(data.Items)

	if err := s.writeUnlocked(data); err != nil {
		return State{}, err
	}
	return cloneState(stateOf(data)), nil
}

func (s *Service) Remove(ctx context.Context, hash string) (State, error) {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()

	hash = strings.ToLower(strings.TrimSpace(hash))
	if hash == "" {
		return State{}, fmt.Errorf("hash 不能为空")
	}

	data, err := s.readUnlocked()
	if err != nil {
		return State{}, err
	}
	filtered := make([]Item, 0, len(data.Items))
	for _, item := range data.Items {
		if !strings.EqualFold(item.Hash, hash) {
			filtered = append(filtered, item)
		}
	}
	if len(filtered) == len(data.Items) {
		return cloneState(stateOf(data)), nil
	}
	data.Items = filtered
	if err := s.writeUnlocked(data); err != nil {
		return State{}, err
	}
	return cloneState(stateOf(data)), nil
}

func (s *Service) Clear(ctx context.Context) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeUnlocked(snapshot{Version: 1})
}

func stateOf(data snapshot) State {
	return State{Items: data.Items}
}

func (s *Service) readUnlocked() (snapshot, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return snapshot{Version: 1}, nil
		}
		return snapshot{}, fmt.Errorf("read magnet favorites file: %w", err)
	}

	var data snapshot
	if err := json.Unmarshal(raw, &data); err != nil {
		return s.handleCorruptedUnlocked()
	}
	data.Items = sanitizeItems(data.Items)
	return data, nil
}

func (s *Service) handleCorruptedUnlocked() (snapshot, error) {
	backupPath, moveErr := s.moveCorruptedFileUnlocked()
	if moveErr != nil {
		s.log.Error("磁力收藏文件解析失败，转移损坏文件失败", "path", s.path, "move_err", moveErr)
		return snapshot{}, fmt.Errorf("magnet favorites file corrupted")
	}
	s.log.Error("磁力收藏文件解析失败，已转移损坏文件", "path", s.path, "backup", backupPath)
	return snapshot{}, fmt.Errorf("磁力收藏文件已损坏，已转移到 %s，请检查或手动恢复", backupPath)
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
		return "", fmt.Errorf("rename corrupted magnet favorites file: %w", err)
	}
	return target, nil
}

func (s *Service) writeUnlocked(data snapshot) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create magnet favorites dir: %w", err)
	}
	data.Version = 1
	body, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal magnet favorites: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(body, '\n'), 0o644); err != nil {
		return fmt.Errorf("write magnet favorites tmp: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("rename magnet favorites file: %w", err)
	}
	return nil
}

// sanitizeItem 校验并规范化单条收藏。
func sanitizeItem(item Item) (Item, bool) {
	hash := strings.ToLower(strings.TrimSpace(item.Hash))
	if !hashPattern.MatchString(hash) {
		return Item{}, false
	}
	magnet := strings.TrimSpace(item.Magnet)
	if !strings.HasPrefix(strings.ToLower(magnet), magnetPrefix) {
		return Item{}, false
	}
	name := strings.TrimSpace(item.Name)
	if name == "" {
		return Item{}, false
	}
	return Item{
		Hash:     hash,
		Name:     name,
		Size:     item.Size,
		Seeders:  item.Seeders,
		Leechers: item.Leechers,
		Date:     item.Date,
		Category: strings.TrimSpace(item.Category),
		Magnet:   magnet,
		ViewURL:  strings.TrimSpace(item.ViewURL),
	}, true
}

// sanitizeItems 过滤无效项，按 hash 去重。
func sanitizeItems(items []Item) []Item {
	out := make([]Item, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		clean, ok := sanitizeItem(item)
		if !ok {
			continue
		}
		if _, dup := seen[clean.Hash]; dup {
			continue
		}
		out = append(out, clean)
		seen[clean.Hash] = struct{}{}
	}
	sortByCreatedDesc(out)
	return out
}

func sortByCreatedDesc(items []Item) {
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].CreatedAt > items[j].CreatedAt
	})
}

func cloneState(state State) State {
	out := State{Items: make([]Item, 0, len(state.Items))}
	for _, item := range state.Items {
		out.Items = append(out.Items, item)
	}
	return out
}
