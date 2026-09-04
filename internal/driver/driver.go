package driver

import (
	"context"
	"time"

	"litepan/internal/domain"
)

// AuthType 决定该驱动的认证刷新策略。
type AuthType string

const (
	AuthNone   AuthType = "none"
	AuthToken  AuthType = "token"
	AuthCookie AuthType = "cookie"
)

// Config 是驱动的静态元信息，决定公共层对它的处理方式。
type Config struct {
	Name        string   // 唯一标识，如 "localfs"/"123_open"
	DisplayName string   // 前端展示名
	Description string   // 前端驱动选择说明
	CardTags    []string // 驱动卡片展示标签；未声明则前端不显示胶囊
	SortOrder   int      // 驱动选择排序，0 表示使用默认靠后顺序
	AuthLabel   string   // 前端展示的认证方式标签
	CardColor   string   // 前端卡片色
	CardLogo    string   // 前端卡片 logo 路径，如 /logos/123.png
	DefaultRoot string   // 默认根目录 ID
	AuthType    AuthType // 认证类型
	// 主动刷新调度参数（AuthToken 型驱动填写 TokenLifetime/RefreshAdvance）。
	TokenLifetime          time.Duration
	RefreshAdvance         time.Duration
	HealthCheckInterval    time.Duration // AuthCookie 型驱动填写
	ProvideHashes          []string      // 跨盘秒传：源盘可提供的指纹类型（sha1/md5）
	RapidUploadHashes      []string      // 跨盘秒传：目标盘支持的指纹秒传类型
	UploadConflictPolicies []string      // 跨盘秒传/上传：前端可选冲突策略
}

// Meta 是所有驱动必须实现的元信息与生命周期接口。
type Meta interface {
	Config() Config
	GetAddition() any // 配置结构指针，用于反射生成表单与 JSON 反序列化
	Init(ctx context.Context) error
	Drop(ctx context.Context) error
	Ping(ctx context.Context) error
}

// Lister 提供目录列举能力，是驱动的基础能力。
type Lister interface {
	ListFiles(ctx context.Context, parentID string) ([]domain.FileItem, error)
}

// InfoGetter 可选：单文件信息。
type InfoGetter interface {
	GetFileInfo(ctx context.Context, fileID string) (*domain.FileItem, error)
}

// DownloadRequest 是驱动解析直链时的输入。
type DownloadRequest struct {
	FileID string
	UA     string
}

// Downloader 可选：解析下载/播放直链。
type Downloader interface {
	ResolveDownload(ctx context.Context, req DownloadRequest) (*domain.DownloadInfo, error)
}

type Deleter interface {
	DeleteFiles(ctx context.Context, fileIDs []string) error
}

type Mover interface {
	MoveFiles(ctx context.Context, fileIDs []string, targetParentID, sourceParentID string) error
}

type Copier interface {
	CopyFiles(ctx context.Context, fileIDs []string, targetParentID string) error
}

type Renamer interface {
	RenameFile(ctx context.Context, fileID, newName string) error
}

type FolderCreator interface {
	CreateFolder(ctx context.Context, parentID, name string) (*domain.FileItem, error)
}

// AuthCredentialConsumer 运行前注入 token/cookie。
type AuthCredentialConsumer interface {
	SetAuthCredentials(creds domain.AuthCredentials)
}

// AuthPersistFunc 驱动自行刷新成功后回写 account_auth_states 的回调。
type AuthPersistFunc func(ctx context.Context, creds domain.AuthCredentials) error

// AuthPersistConsumer 由会在运行期更新令牌的驱动实现（如 OAuth 刷新）。
type AuthPersistConsumer interface {
	SetAuthPersister(fn AuthPersistFunc)
}

// ConnectionErrorExplainer 可选：自定义连通性错误文案。
type ConnectionErrorExplainer interface {
	ExplainConnectionError(technical string, saving bool) string
}

// RequestIntervalConsumer 可选：账号级 API 请求间隔。
type RequestIntervalConsumer interface {
	SetRequestIntervalGate(gate RequestIntervalGate)
}

// AccountRefResolver 按账号名或账号 ID 解析另一个账号的驱动实例（alias 等聚合驱动用）。
type AccountRefResolver struct {
	ByName func(ctx context.Context, accountName string) (accountID int64, drv Driver, err error)
	ByID   func(ctx context.Context, accountID int64) (drv Driver, err error)
}

// AccountResolverConsumer 可选：接收按账号名/ID 解析其他驱动实例的回调。
type AccountResolverConsumer interface {
	SetAccountResolver(r AccountRefResolver)
}

// Driver 最小契约：元信息 + 列目录；其余能力走可选接口。
type Driver interface {
	Meta
	Lister
}
