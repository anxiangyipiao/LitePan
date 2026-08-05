// Package alias 提供「路径别名」驱动：把多个账号的目录虚拟合并成一个账号。
// 根目录列出配置的虚拟名；进入虚拟名后展示各目标账号对应目录的合并内容（按文件名去重）。
// 子目录/文件操作委托给所属账号的驱动实例。
package alias

import (
	"context"
	"fmt"
	"strings"
	"time"

	"litepan/internal/domain"
	"litepan/internal/driver"
)

// aliasTarget 一个合并目标：某个账号下的一段路径（segments 为空表示该账号根目录）。
type aliasTarget struct {
	accountName string
	segments    []string
}

type Driver struct {
	add Addition

	resolver driver.AccountRefResolver
	order    []string
	byKey    map[string][]aliasTarget
}

var config = driver.Config{
	Name:        "alias",
	DisplayName: "路径别名",
	Description: "把多个账号的目录合并成一个虚拟账号",
	CardTags:    []string{"聚合", "虚拟目录"},
	SortOrder:   200,
	AuthLabel:   "无需认证",
	CardColor:   "#8b5cf6",
	DefaultRoot: "",
	AuthType:    driver.AuthNone,
}

func New() driver.Driver { return &Driver{} }

func init() { driver.Register(New) }

func (d *Driver) Config() driver.Config { return config }

func (d *Driver) GetAddition() any { return &d.add }

func (d *Driver) SetAccountResolver(r driver.AccountRefResolver) { d.resolver = r }

func (d *Driver) Init(ctx context.Context) error {
	_ = ctx
	order := make([]string, 0, 4)
	byKey := make(map[string][]aliasTarget, 4)
	for _, line := range strings.Split(d.add.Paths, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, rest, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			continue
		}
		if _, dup := byKey[key]; !dup {
			order = append(order, key)
		}
		if targets := parseTargets(rest); len(targets) > 0 {
			byKey[key] = append(byKey[key], targets...)
		}
	}
	if len(order) == 0 {
		return domain.Errorf(domain.CodeValidation, "别名映射为空，请配置「虚拟名=账号:路径」")
	}
	d.order = order
	d.byKey = byKey
	return nil
}

func (d *Driver) Drop(context.Context) error { return nil }

// Ping 别名是虚拟驱动，无可连通的真实服务；配置合法性已在 Init 校验。
// 注意：创建账号的临时实例不会注入账号解析器，这里必须返回 nil，否则添加账号会误报认证失败。
func (d *Driver) Ping(context.Context) error { return nil }

// ---------- 目录与文件 ----------

func (d *Driver) ListFiles(ctx context.Context, parentID string) ([]domain.FileItem, error) {
	switch kind, accountID, realID, key := parseID(parentID); kind {
	case idRoot:
		out := make([]domain.FileItem, 0, len(d.order))
		for _, k := range d.order {
			out = append(out, domain.FileItem{
				ID:      "virt:" + k,
				Name:    k,
				IsDir:   true,
				ModTime: time.Now(),
			})
		}
		return out, nil
	case idVirtual:
		return d.listVirtual(ctx, key)
	case idReal:
		target, err := d.resolveByID(ctx, accountID)
		if err != nil {
			return nil, err
		}
		items, err := target.ListFiles(ctx, realID)
		if err != nil {
			return nil, err
		}
		return prefixRealItems(items, accountID), nil
	default:
		return nil, domain.Errorf(domain.CodeValidation, "非法目录 ID")
	}
}

// listVirtual 列出虚拟名对应所有目标目录的合并内容，按文件名去重（先到先得）。
func (d *Driver) listVirtual(ctx context.Context, key string) ([]domain.FileItem, error) {
	targets := d.byKey[key]
	if len(targets) == 0 {
		return []domain.FileItem{}, nil
	}
	merged := make([]domain.FileItem, 0, 64)
	seen := make(map[string]struct{}, 64)
	for _, t := range targets {
		accountID, drv, err := d.resolveByName(ctx, t.accountName)
		if err != nil {
			continue // 单个目标不可用不拖垮整个虚拟目录
		}
		folderID, err := resolvePath(ctx, drv, t.segments)
		if err != nil {
			continue
		}
		items, err := drv.ListFiles(ctx, folderID)
		if err != nil {
			continue
		}
		for _, it := range items {
			name := it.Name
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			merged = append(merged, domain.FileItem{
				ID:      "acct:" + itoa(accountID) + ":" + it.ID,
				Name:    it.Name,
				Size:    it.Size,
				IsDir:   it.IsDir,
				ModTime: it.ModTime,
				Hash:    it.Hash,
			})
		}
	}
	return merged, nil
}

func (d *Driver) GetFileInfo(ctx context.Context, fileID string) (*domain.FileItem, error) {
	switch kind, accountID, realID, key := parseID(fileID); kind {
	case idVirtual:
		return &domain.FileItem{ID: fileID, Name: key, IsDir: true, ModTime: time.Now()}, nil
	case idReal:
		target, err := d.resolveByID(ctx, accountID)
		if err != nil {
			return nil, err
		}
		ig, ok := target.(driver.InfoGetter)
		if !ok {
			return nil, domain.Errorf(domain.CodeValidation, "目标账号不支持文件信息查询")
		}
		it, err := ig.GetFileInfo(ctx, realID)
		if err != nil {
			return nil, err
		}
		cloned := *it
		cloned.ID = "acct:" + itoa(accountID) + ":" + it.ID
		return &cloned, nil
	default:
		return nil, domain.Errorf(domain.CodeValidation, "非法文件 ID")
	}
}

func (d *Driver) ResolveDownload(ctx context.Context, req driver.DownloadRequest) (*domain.DownloadInfo, error) {
	kind, accountID, realID, _ := parseID(req.FileID)
	if kind != idReal {
		return nil, domain.Errorf(domain.CodeValidation, "别名虚拟目录不提供下载")
	}
	target, err := d.resolveByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	dl, ok := target.(driver.Downloader)
	if !ok {
		return nil, domain.Errorf(domain.CodeValidation, "目标账号不支持下载")
	}
	req.FileID = realID
	return dl.ResolveDownload(ctx, req)
}

// ---------- 写入操作（仅支持真实账号内的路径） ----------

func (d *Driver) CreateFolder(ctx context.Context, parentID, name string) (*domain.FileItem, error) {
	kind, accountID, realID, _ := parseID(parentID)
	if kind != idReal {
		return nil, domain.Errorf(domain.CodeValidation, "别名虚拟目录不支持直接创建文件夹")
	}
	target, err := d.resolveByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	fc, ok := target.(driver.FolderCreator)
	if !ok {
		return nil, domain.Errorf(domain.CodeValidation, "目标账号不支持创建文件夹")
	}
	it, err := fc.CreateFolder(ctx, realID, name)
	if err != nil {
		return nil, err
	}
	if it == nil {
		return nil, nil
	}
	cloned := *it
	cloned.ID = "acct:" + itoa(accountID) + ":" + it.ID
	return &cloned, nil
}

func (d *Driver) DeleteFiles(ctx context.Context, fileIDs []string) error {
	ids, accountID, err := splitRealIDs(fileIDs)
	if err != nil {
		return err
	}
	target, err := d.resolveByID(ctx, accountID)
	if err != nil {
		return err
	}
	dl, ok := target.(driver.Deleter)
	if !ok {
		return domain.Errorf(domain.CodeValidation, "目标账号不支持删除")
	}
	return dl.DeleteFiles(ctx, ids)
}

func (d *Driver) MoveFiles(ctx context.Context, fileIDs []string, targetParentID, sourceParentID string) error {
	ids, from, err := splitRealIDs(fileIDs)
	if err != nil {
		return err
	}
	toAccount, to, err := realParent(targetParentID)
	if err != nil {
		return err
	}
	_, fromParent, err := realParent(sourceParentID)
	if err != nil {
		return err
	}
	if from != toAccount {
		return domain.Errorf(domain.CodeValidation, "别名驱动不支持跨账号移动")
	}
	target, err := d.resolveByID(ctx, from)
	if err != nil {
		return err
	}
	mv, ok := target.(driver.Mover)
	if !ok {
		return domain.Errorf(domain.CodeValidation, "目标账号不支持移动")
	}
	return mv.MoveFiles(ctx, ids, to, fromParent)
}

func (d *Driver) CopyFiles(ctx context.Context, fileIDs []string, targetParentID string) error {
	ids, from, err := splitRealIDs(fileIDs)
	if err != nil {
		return err
	}
	toAccount, to, err := realParent(targetParentID)
	if err != nil {
		return err
	}
	if from != toAccount {
		return domain.Errorf(domain.CodeValidation, "别名驱动不支持跨账号复制")
	}
	target, err := d.resolveByID(ctx, from)
	if err != nil {
		return err
	}
	cp, ok := target.(driver.Copier)
	if !ok {
		return domain.Errorf(domain.CodeValidation, "目标账号不支持复制")
	}
	return cp.CopyFiles(ctx, ids, to)
}

func (d *Driver) RenameFile(ctx context.Context, fileID, newName string) error {
	kind, accountID, realID, _ := parseID(fileID)
	if kind != idReal {
		return domain.Errorf(domain.CodeValidation, "别名虚拟目录不支持重命名")
	}
	target, err := d.resolveByID(ctx, accountID)
	if err != nil {
		return err
	}
	rn, ok := target.(driver.Renamer)
	if !ok {
		return domain.Errorf(domain.CodeValidation, "目标账号不支持重命名")
	}
	return rn.RenameFile(ctx, realID, newName)
}

// ---------- 辅助 ----------

// idKind 描述文件 ID 的种类。
type idKind int

const (
	idInvalid idKind = iota
	idRoot
	idVirtual // virt:<key>
	idReal    // acct:<accountID>:<realID>
)

// parseID 解析别名驱动内部文件 ID。
func parseID(fileID string) (idKind, int64, string, string) {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return idRoot, 0, "", ""
	}
	if v, ok := strings.CutPrefix(fileID, "virt:"); ok {
		return idVirtual, 0, "", v
	}
	if v, ok := strings.CutPrefix(fileID, "acct:"); ok {
		accountID, realID, ok := strings.Cut(v, ":")
		if !ok {
			return idInvalid, 0, "", ""
		}
		var id int64
		for _, c := range accountID {
			if c < '0' || c > '9' {
				return idInvalid, 0, "", ""
			}
			id = id*10 + int64(c-'0')
		}
		return idReal, id, realID, ""
	}
	return idInvalid, 0, "", ""
}

func (d *Driver) resolveByName(ctx context.Context, accountName string) (int64, driver.Driver, error) {
	if d.resolver.ByName == nil {
		return 0, nil, domain.Errorf(domain.CodeInternal, "别名驱动未注入账号解析器")
	}
	return d.resolver.ByName(ctx, accountName)
}

func (d *Driver) resolveByID(ctx context.Context, accountID int64) (driver.Driver, error) {
	if d.resolver.ByID == nil {
		return nil, domain.Errorf(domain.CodeInternal, "别名驱动未注入账号解析器")
	}
	return d.resolver.ByID(ctx, accountID)
}

func splitRealIDs(fileIDs []string) ([]string, int64, error) {
	if len(fileIDs) == 0 {
		return nil, 0, nil
	}
	var accountID int64 = -1
	ids := make([]string, 0, len(fileIDs))
	for _, f := range fileIDs {
		kind, id, realID, _ := parseID(f)
		if kind != idReal {
			return nil, 0, domain.Errorf(domain.CodeValidation, "别名虚拟目录不支持该操作")
		}
		if accountID == -1 {
			accountID = id
		} else if accountID != id {
			return nil, 0, domain.Errorf(domain.CodeValidation, "别名驱动不支持跨账号操作")
		}
		ids = append(ids, realID)
	}
	return ids, accountID, nil
}

func realParent(parentID string) (int64, string, error) {
	kind, id, real, _ := parseID(parentID)
	if kind != idReal {
		return 0, "", domain.Errorf(domain.CodeValidation, "别名虚拟目录不支持该操作")
	}
	return id, real, nil
}

func prefixRealItems(items []domain.FileItem, accountID int64) []domain.FileItem {
	out := make([]domain.FileItem, 0, len(items))
	for _, it := range items {
		it.ID = "acct:" + itoa(accountID) + ":" + it.ID
		out = append(out, it)
	}
	return out
}

// resolvePath 在目标账号内按路径段名逐层定位文件夹，返回其文件夹 ID。
func resolvePath(ctx context.Context, drv driver.Driver, segments []string) (string, error) {
	current := drv.Config().DefaultRoot
	for _, seg := range segments {
		if seg == "" {
			continue
		}
		items, err := drv.ListFiles(ctx, current)
		if err != nil {
			return "", err
		}
		found := ""
		for _, it := range items {
			if it.IsDir && it.Name == seg {
				found = it.ID
				break
			}
		}
		if found == "" {
			return "", fmt.Errorf("在目标账号中找不到目录：%s", seg)
		}
		current = found
	}
	return current, nil
}

// parseTargets 解析目标列表。目标形如「账号名:路径」或「账号名/路径」（路径可省略=根目录）。
func parseTargets(raw string) []aliasTarget {
	var out []aliasTarget
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, pathPart, hasColon := strings.Cut(part, ":")
		if !hasColon {
			// 兼容「账号名/路径」写法（冒号与斜杠都可作账号/路径分隔符）
			if i := strings.Index(part, "/"); i >= 0 {
				name = part[:i]
				pathPart = part[i:]
			}
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		t := aliasTarget{accountName: name}
		if pathPart != "" {
			pathPart = strings.Trim(pathPart, "/ ")
			if pathPart != "" {
				t.segments = strings.Split(pathPart, "/")
			}
		}
		out = append(out, t)
	}
	return out
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

var (
	_ driver.Driver                  = (*Driver)(nil)
	_ driver.InfoGetter              = (*Driver)(nil)
	_ driver.Downloader              = (*Driver)(nil)
	_ driver.FolderCreator           = (*Driver)(nil)
	_ driver.Deleter                 = (*Driver)(nil)
	_ driver.Mover                   = (*Driver)(nil)
	_ driver.Copier                  = (*Driver)(nil)
	_ driver.Renamer                 = (*Driver)(nil)
	_ driver.AccountResolverConsumer = (*Driver)(nil)
)
