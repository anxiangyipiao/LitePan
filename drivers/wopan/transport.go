package wopan

import (
	"context"
	"strings"

	"github.com/go-resty/resty/v2"

	"litepan/internal/domain"
	"litepan/internal/driver"

	wo "github.com/OpenListTeam/wopan-sdk-go"
)

const defaultOperationDelayMS = 200

// rootID 返回根目录 ID（默认 0，可被 RootFolderID 覆盖）。
func (d *Driver) rootID() string {
	if id := strings.TrimSpace(d.add.RootFolderID); id != "" {
		return id
	}
	return "0"
}

func (d *Driver) normalizeParent(parentID string) string {
	p := strings.TrimSpace(parentID)
	if p == "" || p == "/" || p == "root" || p == "0" {
		return d.rootID()
	}
	return decodeWopanID(p)
}

// familyID 返回 Addition 显式配置的家庭空间 ID（个人空间为空）。
func (d *Driver) familyID() string { return strings.TrimSpace(d.add.FamilyID) }

// effectiveFamilyID 优先使用 Addition 家庭空间 ID，否则回落 Init 解析的默认家庭空间。
func (d *Driver) effectiveFamilyID() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if id := strings.TrimSpace(d.add.FamilyID); id != "" {
		return id
	}
	return d.defaultFamilyID
}

func (d *Driver) getSpaceType() string {
	if d.familyID() == "" {
		return wo.SpaceTypePersonal
	}
	return wo.SpaceTypeFamily
}

func (d *Driver) getSortRule() int {
	switch d.add.SortRule {
	case "name_asc":
		return wo.SortNameAsc
	case "name_desc":
		return wo.SortNameDesc
	case "time_asc":
		return wo.SortTimeAsc
	case "time_desc":
		return wo.SortTimeDesc
	case "size_asc":
		return wo.SortSizeAsc
	case "size_desc":
		return wo.SortSizeDesc
	default:
		return wo.SortNameAsc
	}
}

func (d *Driver) woClient() *wo.WoClient {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.wo
}

func (d *Driver) waitOperationDelay(ctx context.Context) error {
	return driver.WaitRequestInterval(ctx, d.intervalGate, defaultOperationDelayMS)
}

// sdkOpts 把请求 ctx 注入 SDK 的 resty 请求，用于取消与超时。
func (d *Driver) sdkOpts(ctx context.Context) []wo.RestyOption {
	return []wo.RestyOption{func(req *resty.Request) { req.SetContext(ctx) }}
}

// wopan 有两套文件标识：id 用于目录导航/移动/删除，fid 用于下载。
// LitePan 只有单一 fileID，故用 "fid|type|id" 复合编码同时携带，type 区分目录(0)/文件(1)。
func encodeWopanID(id, fid string, isDir bool) string {
	typ := "1"
	if isDir {
		typ = "0"
	}
	return fid + "|" + typ + "|" + id
}

func splitWopanID(raw string) (fid, typ, id string, ok bool) {
	parts := strings.SplitN(raw, "|", 3)
	if len(parts) != 3 {
		return "", "", raw, false
	}
	return parts[0], parts[1], parts[2], true
}

// decodeWopanID 取目录导航用 id（列表/移动/删除等平台操作的标识）。
func decodeWopanID(raw string) string {
	if _, _, id, ok := splitWopanID(raw); ok {
		return id
	}
	return raw
}

// decodeWopanFID 取下载用 fid。
func decodeWopanFID(raw string) string {
	if fid, _, _, ok := splitWopanID(raw); ok {
		return fid
	}
	return raw
}

func isWopanDir(raw string) bool {
	if _, typ, _, ok := splitWopanID(raw); ok {
		return typ == "0"
	}
	return true // 根目录等非复合格式按目录处理
}

// splitWopanIDs 按类型拆分 fileID 列表为目录/文件两份清单（对应平台 dirList/fileList）。
func splitWopanIDs(ids []string) (dirs, files []string) {
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if isWopanDir(id) {
			dirs = append(dirs, decodeWopanID(id))
		} else {
			files = append(files, decodeWopanID(id))
		}
	}
	return dirs, files
}

func mapWopanError(err error) error {
	if err == nil {
		return nil
	}
	if ae, ok := domain.AsAppError(err); ok {
		return ae
	}
	msg := err.Error()
	if isWopanAuthError(msg) {
		return domain.Errorf(domain.CodeAuthExpired, "沃云盘认证失败：%s", msg)
	}
	return domain.Errorf(domain.CodeDriverError, "沃云盘请求失败：%s", msg)
}

func isWopanAuthError(msg string) bool {
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "unauthorized") || strings.Contains(lower, "401") {
		return true
	}
	// rsp_code 9999 表示令牌失效；SDK 已自动刷新重试一次，仍走到这里即刷新链路失败。
	if strings.Contains(lower, "9999") || strings.Contains(lower, "access_token") ||
		strings.Contains(lower, "refresh_token") {
		return true
	}
	return false
}
