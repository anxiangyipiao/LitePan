// Package wopan 是沃云盘（pan.wo.cn，中国联通）驱动。
// 底层与平台的通信协议（AES 加密、header 签名、分片上传）复用
// github.com/OpenListTeam/wopan-sdk-go，本包只负责把 SDK 接入 LitePan 的
// 驱动接口、目录结构与注册方式。
package wopan

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"litepan/internal/domain"
	"litepan/internal/driver"
	"litepan/internal/httpx"

	wo "github.com/OpenListTeam/wopan-sdk-go"
)

// Driver 是沃云盘驱动实例。
type Driver struct {
	add    Addition
	client *http.Client

	intervalGate driver.RequestIntervalGate
	persist      driver.AuthPersistFunc

	mu      sync.Mutex
	token   string
	refresh string
	wo      *wo.WoClient
	// defaultFamilyID 是 Init 时由账号信息解析出的默认家庭空间 ID。
	defaultFamilyID string
}

var config = driver.Config{
	Name:           "wopan",
	DisplayName:    "沃云盘",
	Description:    "中国联通沃云盘（pan.wo.cn）",
	CardTags:       []string{"令牌", "家庭空间"},
	SortOrder:      10,
	AuthLabel:      "令牌",
	CardColor:      "#d81e06",
	DefaultRoot:    "0",
	AuthType:       driver.AuthToken,
	TokenLifetime:  7 * 24 * time.Hour,
	RefreshAdvance: 12 * time.Hour,
}

func New() driver.Driver { return &Driver{} }

func init() { driver.Register(New) }

func (d *Driver) Config() driver.Config { return config }

func (d *Driver) GetAddition() any { return &d.add }

func (d *Driver) SetAuthCredentials(creds domain.AuthCredentials) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.token = strings.TrimSpace(creds.AccessToken)
	d.refresh = strings.TrimSpace(creds.RefreshToken)
}

func (d *Driver) SetAuthPersister(fn driver.AuthPersistFunc) { d.persist = fn }

func (d *Driver) SetRequestIntervalGate(gate driver.RequestIntervalGate) { d.intervalGate = gate }

func (d *Driver) Init(ctx context.Context) error {
	if d.client == nil {
		d.client = httpx.NewClient(httpx.ClientOptions{Timeout: 180 * time.Second, DisableCompression: true})
	}
	d.mu.Lock()
	token := strings.TrimSpace(d.token)
	if token == "" {
		token = strings.TrimSpace(d.add.AccessToken)
	}
	refresh := strings.TrimSpace(d.refresh)
	if refresh == "" {
		refresh = strings.TrimSpace(d.add.RefreshToken)
	}
	d.mu.Unlock()
	if token == "" && refresh == "" {
		return domain.Errorf(domain.CodeValidation, "refresh_token 不能为空")
	}

	client := wo.New(
		wo.WithClient(d.client),
		wo.WithRefreshToken(refresh),
		wo.WithUA(wo.DefaultUA),
	)
	// access_token 不足 16 位时 SDK 加密会失败，留空交由 RefreshToken 换新。
	if len(token) >= 16 {
		client.SetAccessToken(token)
	}
	client.OnRefreshToken(d.onSDKRefreshToken)

	if token == "" && refresh != "" {
		if err := client.RefreshToken(); err != nil {
			return mapWopanError(err)
		}
	}
	if err := client.InitClassifyRule(); err != nil {
		return mapWopanError(err)
	}
	if err := client.InitZoneURL(); err != nil {
		return mapWopanError(err)
	}
	fml, err := client.FamilyUserCurrentEncode(d.sdkOpts(ctx)...)
	if err != nil {
		return mapWopanError(err)
	}

	d.mu.Lock()
	d.wo = client
	d.defaultFamilyID = strconv.Itoa(fml.DefaultHomeId)
	d.mu.Unlock()
	return nil
}

// Drop 关闭 idle 连接。实例在 ResetTransport 后会复用（不重建），因此只关连接、不清空 SDK 客户端。
func (d *Driver) Drop(context.Context) error {
	httpx.CloseClient(d.client)
	return nil
}

func (d *Driver) Ping(ctx context.Context) error {
	c := d.woClient()
	if c == nil {
		return domain.Errorf(domain.CodeInternal, "驱动尚未初始化")
	}
	if err := d.waitOperationDelay(ctx); err != nil {
		return err
	}
	_, err := c.FamilyUserCurrentEncode(d.sdkOpts(ctx)...)
	return mapWopanError(err)
}

// ListFiles 分页列举目录子项（wopan 每页最多 100 条，不足一页即结束）。
func (d *Driver) ListFiles(ctx context.Context, parentID string) ([]domain.FileItem, error) {
	c := d.woClient()
	if c == nil {
		return nil, domain.Errorf(domain.CodeInternal, "驱动尚未初始化")
	}
	parent := d.normalizeParent(parentID)
	spaceType := d.getSpaceType()
	familyID := d.familyID()

	var items []domain.FileItem
	pageNum := 0
	const pageSize = 100
	for {
		if err := d.waitOperationDelay(ctx); err != nil {
			return nil, err
		}
		data, err := c.QueryAllFiles(spaceType, parent, pageNum, pageSize, d.getSortRule(), familyID, d.sdkOpts(ctx)...)
		if err != nil {
			return nil, mapWopanError(err)
		}
		for _, f := range data.Files {
			items = append(items, fileToItem(f))
		}
		if len(data.Files) < pageSize {
			break
		}
		pageNum++
	}
	return items, nil
}

var (
	_ driver.Driver                  = (*Driver)(nil)
	_ driver.Downloader              = (*Driver)(nil)
	_ driver.Deleter                 = (*Driver)(nil)
	_ driver.Mover                   = (*Driver)(nil)
	_ driver.Copier                  = (*Driver)(nil)
	_ driver.Renamer                 = (*Driver)(nil)
	_ driver.FolderCreator           = (*Driver)(nil)
	_ driver.LocalUploader           = (*Driver)(nil)
	_ driver.AuthRefresher           = (*Driver)(nil)
	_ driver.AuthCredentialConsumer  = (*Driver)(nil)
	_ driver.AuthPersistConsumer     = (*Driver)(nil)
	_ driver.RequestIntervalConsumer = (*Driver)(nil)
)
