package dav

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	"litepan/internal/domain"
	"litepan/internal/file"
)

// Node 是 WebDAV 路径解析后的资源节点。
type Node struct {
	IsRoot    bool
	IsAccount bool
	Account   *domain.Account
	Item      domain.FileItem
	ParentID  string
}

type Resolver struct {
	files    *file.Service
	accounts domain.AccountRepository
	wc       *webdavCache
	acct     *accountCache
}

func NewResolver(files *file.Service, accounts domain.AccountRepository, wc *webdavCache) *Resolver {
	return &Resolver{
		files:    files,
		accounts: accounts,
		wc:       wc,
		acct:     newAccountCache(15 * time.Second),
	}
}

// accountCache 缓存活跃账号快照（name→账号），避免每个 WebDAV 请求全表扫库。
// TTL 过期后自动重读；账号增删改最多延迟 ttl 生效。
type accountCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	at      time.Time
	byLower map[string]*domain.Account
	list    []*domain.Account
}

func newAccountCache(ttl time.Duration) *accountCache {
	return &accountCache{ttl: ttl, byLower: make(map[string]*domain.Account)}
}

func (r *Resolver) activeAccounts(ctx context.Context) ([]*domain.Account, map[string]*domain.Account) {
	if r.accounts == nil {
		return nil, nil
	}
	if r.acct != nil {
		if list, by := r.acct.get(); list != nil {
			return list, by
		}
	}
	list, err := r.accounts.List(ctx)
	if err != nil {
		return nil, nil
	}
	by := make(map[string]*domain.Account, len(list))
	active := make([]*domain.Account, 0, len(list))
	for _, acc := range list {
		if !acc.IsActive {
			continue
		}
		by[strings.ToLower(acc.Name)] = acc
		active = append(active, acc)
	}
	if r.acct != nil {
		r.acct.set(active, by)
	}
	return active, by
}

func (c *accountCache) get() ([]*domain.Account, map[string]*domain.Account) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.list == nil || time.Since(c.at) >= c.ttl {
		return nil, nil
	}
	return c.list, c.byLower
}

func (c *accountCache) set(list []*domain.Account, by map[string]*domain.Account) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.list = list
	c.byLower = by
	c.at = time.Now()
}

func (r *Resolver) Resolve(ctx context.Context, webPath string) (*Node, error) {
	parsed := ParseWebDAVPath(webPath)
	if parsed.AccountName == "" {
		return &Node{IsRoot: true, Item: domain.FileItem{Name: "", IsDir: true, ModTime: time.Now()}}, nil
	}
	acc, err := r.accountByName(ctx, parsed.AccountName)
	if err != nil {
		return nil, err
	}
	if len(parsed.RelParts) == 0 {
		return &Node{
			IsAccount: true,
			Account:   acc,
			Item: domain.FileItem{
				ID:      "0",
				Name:    acc.Name,
				IsDir:   true,
				ModTime: acc.CreatedAt,
			},
			ParentID: "0",
		}, nil
	}
	item, parentID, err := r.resolveUnderAccountCached(ctx, acc.ID, parsed.RelParts, true)
	if err != nil {
		return nil, err
	}
	return &Node{Account: acc, Item: *item, ParentID: parentID}, nil
}

func (r *Resolver) accountByName(ctx context.Context, name string) (*domain.Account, error) {
	if r.accounts == nil {
		return nil, os.ErrNotExist
	}
	_, by := r.activeAccounts(ctx)
	acc, ok := by[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return nil, os.ErrNotExist
	}
	return acc, nil
}

func (r *Resolver) ListChildren(ctx context.Context, node *Node) ([]domain.FileItem, error) {
	switch {
	case node.IsRoot:
		list, _ := r.activeAccounts(ctx)
		out := make([]domain.FileItem, 0, len(list))
		for _, acc := range list {
			out = append(out, domain.FileItem{
				ID:      acc.Name,
				Name:    acc.Name,
				IsDir:   true,
				ModTime: acc.CreatedAt,
			})
		}
		return out, nil
	case node.IsAccount, node.Item.IsDir:
		parentID := "0"
		if !node.IsAccount {
			parentID = node.Item.ID
		}
		return r.files.List(ctx, node.Account.ID, parentID, false)
	default:
		return nil, os.ErrInvalid
	}
}

func (r *Resolver) resolveUnderAccount(ctx context.Context, accountID int64, parts []string) (*domain.FileItem, string, error) {
	return r.resolveUnderAccountCached(ctx, accountID, parts, true)
}
