package settings

import "litepan/internal/domain"

// 全局设置：默认值在代码，DB 仅存用户改过的项。

// 设置键。oauth 复用 domain 常量，保证与驱动层读取一致。
const (
	KeyOAuthServerURL              = domain.SettingOAuthServerURL
	KeyCacheEnabled                = "cache_enabled"
	KeyCacheTTL                    = "cache_ttl"
	KeyCacheMaxItems               = "cache_max_items"
	KeyCacheMemoryLimitMB          = "cache_memory_limit_mb"
	KeyCachePersistenceEnabled     = "cache_persistence_enabled"
	KeyCachePersistenceIntervalMin = "cache_persistence_interval_minutes"
	KeyUploadTaskConcurrency       = "upload_task_concurrency"
	KeyWebDAVCacheEnabled          = "webdav_cache_enabled"
	KeyFuseReadCacheEnabled        = "fuse_read_cache_enabled"
	KeyFuseReadCacheMaxGB          = "fuse_read_cache_max_gb"
	KeyFuseReadCacheRetentionDays  = "fuse_read_cache_retention_days"
	KeyFuseReadCacheEvictionPolicy = "fuse_read_cache_eviction_policy"
	KeyAuthActiveRefresh           = "auth_active_refresh_enabled"
	KeyLogLevel                    = "log_level"
	KeyLogRetentionDays            = "log_retention_days"
	KeyLogErrorAckAt               = "log_error_ack_at"
	KeyEmbyEnabled                 = "emby_enabled"
	KeyEmbyURL                     = "emby_url"
	KeyEmbyAPIKey                  = "emby_api_key"
	KeyEmbyProxyPort               = "emby_proxy_port"
	KeyFnosEnabled                 = "fnos_enabled"
	KeyFnosURL                     = "fnos_url"
	KeyFnosProxyPort               = "fnos_proxy_port"
	KeyFnosStrmPathMaps            = "fnos_strm_path_maps"
	KeyStrmToken                   = "strm_token"
	KeyStrmBaseURL                 = "strm_base_url"
	KeyStrmSignatureEnabled        = "strm_signature_enabled"
	KeyStrmDefaultScanInterval     = "strm_default_scan_interval"
	KeyStrmDefaultExtensions       = "strm_default_extensions"
	KeyStrmISOFilenameEnabled      = "strm_iso_filename_enabled"
	KeyStrmMinFileSizeMB           = "strm_min_file_size_mb"
	KeyStrmConflictPolicy          = "strm_conflict_policy"
	KeyStrmTaskConcurrency         = "strm_task_concurrency"
	KeyStrmMetadataExtensions      = "strm_metadata_extensions"
	KeyStrmMetadataMaxSizeMB       = "strm_metadata_max_size_mb"
	KeyStrmMetadataParentEnabled   = "strm_metadata_parent_enabled"
	KeyStrmMetadataSyncMode        = "strm_metadata_sync_mode"
	KeyStrmScrapeWriteMode         = "strm_scrape_write_mode"
	KeyStrmScrapeSource            = "strm_scrape_source"
	KeyStrmScrapeMetaTubeURL       = "strm_scrape_metatube_url"
	KeyMediaLibraryRoots           = "media_library_roots"

	KeyMagnetSearchBaseURL       = "magnet_search_base_url"
	KeyMagnetSearchProxyURL      = "magnet_search_proxy_url"
	KeyMagnetSearchProxyUsername = "magnet_search_proxy_username"
	KeyMagnetSearchProxyPassword = "magnet_search_proxy_password"

	KeyRSSDefaultFetchInterval    = "rss_default_fetch_interval_minutes"
	KeyRSSConvertTorrentToMagnet  = "rss_convert_torrent_to_magnet"

	KeyQBURL      = "qb_url"
	KeyQBUsername = "qb_username"
	KeyQBPassword = "qb_password"
	KeyQBSavePath = "qb_save_path"
	KeyQBCategory = "qb_category"

	KeyMOProxyEnabled          = "mo_proxy_enabled"
	KeyMOProxyURL              = "mo_proxy_url"
	KeyMOProxyUsername         = "mo_proxy_username"
	KeyMOProxyPassword         = "mo_proxy_password"
	KeyMOTmdbAPIKey            = "mo_tmdb_api_key"
	KeyMOTmdbLanguage          = "mo_tmdb_language"
	KeyMOAPIRequestIntervalMS  = "mo_api_request_interval_ms"
	KeyMOTmdbRequestIntervalMS = "mo_tmdb_request_interval_ms"
	KeyMOFileExtensions        = "mo_file_extensions"
	KeyMOMetadataExtensions    = "mo_metadata_extensions"
	KeyMOMediaTagOrder         = "mo_media_tag_order"
	KeyMOAlignMediaTags        = "mo_align_media_tags"
	KeyMOMaxWorksPerRun        = "mo_max_works_per_run"
	KeyMOOverwriteExisting     = "mo_overwrite_existing"
)

// Type 决定后台表单控件与校验方式。
type Type string

const (
	TypeString Type = "string"
	TypeInt    Type = "int"
	TypeBool   Type = "bool"
	TypeSelect Type = "select"
)

// Option 是 select 类型的可选项。
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Spec 声明单个全局设置的元数据，驱动后台表单渲染与写入校验。
type Spec struct {
	Key         string
	Type        Type
	Category    string
	Label       string
	Description string
	Default     string // 默认值的规范字符串形式（与 configs 表存储一致）
	Unit        string
	Min, Max    *int     // 仅 TypeInt
	Options     []Option // 仅 TypeSelect
	Sensitive   bool
	Hidden      bool
	// normalize 对字符串值做规范化/兜底（如 OAuth 地址校验），nil 表示不处理。
	normalize func(string) string
}

// Category 是设置分组，用于后台分区展示。
type Category struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

func intp(n int) *int { return &n }

// defaultSpecs 是全部全局设置的有序声明。新增全局设置只改这里。
func defaultSpecs() []Spec {
	return []Spec{
		{
			Key:         KeyOAuthServerURL,
			Type:        TypeString,
			Category:    "system",
			Label:       "OAuth 代理服务地址",
			Description: "添加账号时「自动获取 Token」经此服务转发。留空或无效地址将回落默认值。本地调试可填 http://127.0.0.1:8000。",
			Default:     domain.DefaultOAuthServerURL,
			normalize:   domain.NormalizeOAuthServerURL,
		},
		{
			Key:         KeyCacheEnabled,
			Type:        TypeBool,
			Category:    "performance",
			Label:       "启用元数据缓存",
			Description: "关闭后所有目录列表都直连网盘，不走缓存。",
			Default:     "true",
		},
		{
			Key:         KeyCacheTTL,
			Type:        TypeInt,
			Category:    "performance",
			Label:       "全局缓存时间",
			Description: "缓存过期时间",
			Default:     "30",
			Unit:        "分钟",
			Min:         intp(0),
			Max:         intp(1440),
		},
		{
			Key:         KeyCacheMaxItems,
			Type:        TypeInt,
			Category:    "performance",
			Label:       "缓存条目上限",
			Description: "元数据缓存最多保留的条目数，超出按 LRU 淘汰。",
			Default:     "10000",
			Unit:        "条",
			Min:         intp(1000),
			Max:         intp(1000000),
		},
		{
			Key:         KeyCacheMemoryLimitMB,
			Type:        TypeInt,
			Category:    "performance",
			Label:       "缓存内存上限",
			Description: "元数据缓存的字节软上限，接近上限触发分级淘汰。",
			Default:     "128",
			Unit:        "MB",
			Min:         intp(64),
			Max:         intp(16384),
		},
		{
			Key:         KeyCachePersistenceEnabled,
			Type:        TypeBool,
			Category:    "performance",
			Label:       "启用缓存持久化",
			Description: "定时将未过期元数据缓存写入磁盘，重启后恢复。",
			Default:     "true",
		},
		{
			Key:         KeyCachePersistenceIntervalMin,
			Type:        TypeInt,
			Category:    "performance",
			Label:       "持久化快照间隔",
			Description: "缓存写入磁盘的间隔，修改后立即生效。",
			Default:     "10",
			Unit:        "分钟",
			Min:         intp(1),
			Max:         intp(1440),
		},
		{
			Key:         KeyUploadTaskConcurrency,
			Type:        TypeInt,
			Category:    "performance",
			Label:       "上传任务并发数",
			Description: "同一时间最多进行几个上传，超出的在任务面板里排队等待；修改后立即生效。",
			Default:     "3",
			Unit:        "个",
			Min:         intp(1),
			Max:         intp(5),
		},
		{
			Key:         KeyWebDAVCacheEnabled,
			Type:        TypeBool,
			Category:    "performance",
			Label:       "WebDAV 路径与 PROPFIND 缓存",
			Description: "开启后缓存 WebDAV 路径解析与 PROPFIND 响应，减少客户端列目录时的网盘 API 调用。",
			Default:     "true",
		},
		{
			Key:         KeyFuseReadCacheEnabled,
			Type:        TypeBool,
			Category:    "performance",
			Label:       "FUSE 读缓存",
			Description: "开启后 FUSE 读取过的文件块会写入本地磁盘，与元数据缓存无关。在「文件共享 → 本地挂载」页配置。",
			Default:     "false",
		},
		{
			Key:         KeyFuseReadCacheMaxGB,
			Type:        TypeInt,
			Category:    "performance",
			Label:       "FUSE 读缓存容量上限",
			Description: "磁盘块缓存最大占用，在「文件共享 → 本地挂载」页配置。",
			Default:     "10",
			Unit:        "GB",
			Min:         intp(1),
			Max:         intp(500),
		},
		{
			Key:         KeyFuseReadCacheRetentionDays,
			Type:        TypeInt,
			Category:    "performance",
			Label:       "FUSE 读缓存保留天数",
			Description: "超过该天数的缓存块会被删除，在「文件共享 → 本地挂载」页配置。",
			Default:     "7",
			Unit:        "天",
			Min:         intp(1),
			Max:         intp(90),
		},
		{
			Key:         KeyFuseReadCacheEvictionPolicy,
			Type:        TypeSelect,
			Category:    "performance",
			Label:       "FUSE 读缓存淘汰策略",
			Description: "容量满时的淘汰方式，在「文件共享 → 本地挂载」页配置。",
			Default:     "lru",
			Options: []Option{
				{Value: "lru", Label: "最近最少使用（LRU）"},
				{Value: "large_file", Label: "大文件优先"},
			},
		},
		{
			Key:         KeyAuthActiveRefresh,
			Type:        TypeBool,
			Category:    "system",
			Label:       "智能主动认证刷新",
			Description: "后台按 token 有效期预刷新、Cookie 健康检查；关闭后仅保留被动刷新。",
			Default:     "true",
		},
		{
			Key:         KeyLogLevel,
			Type:        TypeSelect,
			Category:    "system",
			Label:       "日志级别",
			Description: "控制控制台与落盘日志的最低级别；认证调度、刷新结果等默认可在 Info 查看。",
			Default:     "info",
			Options: []Option{
				{Value: "debug", Label: "Debug（调试）"},
				{Value: "info", Label: "Info（常规）"},
				{Value: "warn", Label: "Warn（警告）"},
				{Value: "error", Label: "Error（错误）"},
			},
		},
		{
			Key:         KeyLogRetentionDays,
			Type:        TypeInt,
			Category:    "system",
			Label:       "日志保留天数",
			Description: "按天落盘日志的保留期。自动清理与日志页手动清理都会按该天数删除更早的旧日志。",
			Default:     "30",
			Unit:        "天",
			Min:         intp(1),
			Max:         intp(365),
		},
		{
			Key:         KeyMagnetSearchBaseURL,
			Type:        TypeString,
			Category:    "system",
			Label:       "磁力搜索站点地址",
			Description: "磁力搜索接口的站点根地址，默认 https://sukebei.nyaa.si。",
			Default:     "https://sukebei.nyaa.si",
		},
		{
			Key:         KeyMagnetSearchProxyURL,
			Type:        TypeString,
			Category:    "system",
			Label:       "磁力搜索代理地址",
			Description: "HTTP/HTTPS 代理地址，例如 http://127.0.0.1:7890；站点不可直连时填写。",
			Default:     "",
		},
		{
			Key:         KeyMagnetSearchProxyUsername,
			Type:        TypeString,
			Category:    "system",
			Label:       "磁力搜索代理用户名",
			Description: "代理认证用户名，无认证可留空。",
			Default:     "",
		},
		{
			Key:         KeyMagnetSearchProxyPassword,
			Type:        TypeString,
			Category:    "system",
			Label:       "磁力搜索代理密码",
			Description: "代理认证密码。",
			Default:     "",
			Sensitive:   true,
		},
		{
			Key:         KeyRSSDefaultFetchInterval,
			Type:        TypeInt,
			Category:    "rss",
			Label:       "RSS 订阅默认抓取间隔",
			Description: "订阅未单独指定抓取间隔时使用。抓取失败会自动退避（指数递增，上限 6 小时）。",
			Default:     "30",
			Unit:        "分钟",
			Min:         intp(1),
			Max:         intp(1440),
		},
		{
			Key:         KeyRSSConvertTorrentToMagnet,
			Type:        TypeBool,
			Category:    "rss",
			Label:       "离线目标转换 .torrent 为磁力",
			Description: "开启后，只有 http .torrent 链接的条目会下载种子文件解析 infohash 转成磁力链再离线；关闭则直接跳过此类条目。",
			Default:     "true",
		},
		{
			Key:         KeyQBURL,
			Type:        TypeString,
			Category:    "system",
			Label:       "qBittorrent WebUI 地址",
			Description: "本地 qB 的 WebUI 根地址，例如 http://192.168.1.10:8080；留空则不启用一键下载到 qB。",
			Default:     "",
		},
		{
			Key:         KeyQBUsername,
			Type:        TypeString,
			Category:    "system",
			Label:       "qB 用户名",
			Description: "qB WebUI 登录用户名，无鉴权可留空。",
			Default:     "",
		},
		{
			Key:         KeyQBPassword,
			Type:        TypeString,
			Category:    "system",
			Label:       "qB 密码",
			Description: "qB WebUI 登录密码。",
			Default:     "",
			Sensitive:   true,
		},
		{
			Key:         KeyQBSavePath,
			Type:        TypeString,
			Category:    "system",
			Label:       "qB 默认保存路径",
			Description: "推送磁力时默认保存目录，留空使用 qB 默认。例：/downloads",
			Default:     "",
		},
		{
			Key:         KeyQBCategory,
			Type:        TypeString,
			Category:    "system",
			Label:       "qB 默认分类",
			Description: "推送磁力时默认分类，留空不分类。",
			Default:     "",
		},
		{
			Key:     KeyLogErrorAckAt,
			Type:    TypeString,
			Default: "",
			Hidden:  true,
		},
		{
			Key:         KeyEmbyEnabled,
			Type:        TypeBool,
			Category:    "emby",
			Label:       "启用 Emby 反代",
			Description: "开启后且填写反代端口时，LitePan 会启动 Emby 反代服务；不填端口时仅保存 Emby 连接配置。",
			Default:     "false",
		},
		{
			Key:         KeyEmbyURL,
			Type:        TypeString,
			Category:    "emby",
			Label:       "Emby 地址",
			Description: "用于 Emby 反代与后续自动化刷库，例如 http://192.168.1.10:8096。",
			Default:     "",
		},
		{
			Key:         KeyEmbyAPIKey,
			Type:        TypeString,
			Category:    "emby",
			Label:       "Emby API Key",
			Description: "用于访问 Emby 管理 API。返回后台时会脱敏显示。",
			Default:     "",
			Sensitive:   true,
		},
		{
			Key:         KeyEmbyProxyPort,
			Type:        TypeString,
			Category:    "emby",
			Label:       "反代端口",
			Description: "可留空。填写并启用后，LitePan 会在该端口启动 Emby 反代服务。",
			Default:     "",
		},
		{
			Key:         KeyFnosEnabled,
			Type:        TypeBool,
			Category:    "fnos",
			Label:       "启用飞牛影视反代",
			Description: "开启后且填写反代端口时，LitePan 会启动飞牛影视反代服务。",
			Default:     "false",
		},
		{
			Key:         KeyFnosURL,
			Type:        TypeString,
			Category:    "fnos",
			Label:       "飞牛影视地址",
			Description: "飞牛影视服务地址，默认端口 8005，例如 http://192.168.1.10:8005。",
			Default:     "",
		},
		{
			Key:         KeyFnosProxyPort,
			Type:        TypeString,
			Category:    "fnos",
			Label:       "反代端口",
			Description: "可留空。填写并启用后，LitePan 会在该端口启动飞牛影视反代服务。",
			Default:     "",
		},
		{
			Key:         KeyFnosStrmPathMaps,
			Type:        TypeString,
			Category:    "fnos",
			Label:       "飞牛 STRM 目录",
			Description: "填写 Docker 中映射到 /app/strm 的左边路径。例：/vol1/.../LitePanGO:/app/strm → 填 /vol1/.../LitePanGO。两边相同可留空。",
			Default:     "",
		},
		{
			Key:         KeyStrmToken,
			Type:        TypeString,
			Category:    "strm",
			Label:       "STRM 播放令牌",
			Description: "STRM 播放路径鉴权令牌，请在系统设置「API 秘钥」中管理。",
			Default:     "",
			Sensitive:   true,
		},
		{
			Key:         KeyStrmBaseURL,
			Type:        TypeString,
			Category:    "strm",
			Label:       "STRM 基础地址",
			Description: "生成本地 .strm 时使用的站点基址（例如 https://example.com）。留空时使用当前服务监听地址。",
			Default:     "",
		},
		{
			Key:         KeyStrmSignatureEnabled,
			Type:        TypeBool,
			Category:    "strm",
			Label:       "启用 STRM 路径签名",
			Description: "开启后 /api/strm/play 路径必须携带有效签名。",
			Default:     "false",
		},
		{
			Key:         KeyStrmDefaultScanInterval,
			Type:        TypeInt,
			Category:    "strm",
			Label:       "STRM 默认扫描间隔",
			Description: "新建任务未指定扫描间隔时使用。",
			Default:     "360",
			Unit:        "分钟",
			Min:         intp(1),
			Max:         intp(1440),
		},
		{
			Key:         KeyStrmDefaultExtensions,
			Type:        TypeString,
			Category:    "strm",
			Label:       "默认同步文件类型",
			Description: "STRM 任务未单独指定扩展名时使用，英文分号分隔。",
			Default:     "mp4;mkv;avi;mov;wmv;flv;ts;m2ts;mpg;mpeg;webm;m4v;iso;rmvb;mp3;flac;aac;wav;m4a",
		},
		{
			Key:         KeyStrmISOFilenameEnabled,
			Type:        TypeBool,
			Category:    "strm",
			Label:       "ISO 使用 .iso.strm 文件名",
			Description: "开启后网盘 .iso 文件生成“文件名.iso.strm”，方便 Infuse 识别 ISO。关闭时保持现有“文件名.strm”命名。",
			Default:     "false",
		},
		{
			Key:         KeyStrmMinFileSizeMB,
			Type:        TypeInt,
			Category:    "strm",
			Label:       "小文件过滤",
			Description: "忽略小于该大小的媒体文件，0 表示不过滤。",
			Default:     "0",
			Unit:        "MB",
			Min:         intp(0),
			Max:         intp(10240),
		},
		{
			Key:         KeyStrmConflictPolicy,
			Type:        TypeString,
			Category:    "strm",
			Label:       "同名冲突策略",
			Description: "同目录同名不同后缀时保留哪一个：size_desc / size_asc / name_asc。",
			Default:     "size_desc",
		},
		{
			Key:         KeyStrmTaskConcurrency,
			Type:        TypeInt,
			Category:    "strm",
			Label:       "STRM 任务并发",
			Description: "同时运行的 STRM 扫描任务上限。",
			Default:     "3",
			Min:         intp(1),
			Max:         intp(10),
		},
		{
			Key:         KeyStrmMetadataExtensions,
			Type:        TypeString,
			Category:    "strm",
			Label:       "元数据扩展名",
			Description: "任务开启同步元数据时使用的扩展名，英文分号分隔。",
			Default:     "srt;ass;ssa;sub;sup;idx;vtt;nfo;jpg;jpeg;png;webp;bmp;gif",
		},
		{
			Key:         KeyStrmMetadataMaxSizeMB,
			Type:        TypeInt,
			Category:    "strm",
			Label:       "元数据大小上限",
			Description: "同步元数据时忽略超过该大小的文件。",
			Default:     "10",
			Unit:        "MB",
			Min:         intp(1),
			Max:         intp(1024),
		},
		{
			Key:         KeyStrmMetadataParentEnabled,
			Type:        TypeBool,
			Category:    "strm",
			Label:       "父目录元数据同步",
			Description: "子目录有影片时，也同步父目录下的海报、nfo 等元数据。",
			Default:     "true",
		},
		{
			Key:         KeyStrmMetadataSyncMode,
			Type:        TypeSelect,
			Category:    "strm",
			Label:       "元数据同步策略",
			Description: "local_primary=保留本地并从云端补缺；cloud_primary=本地目录与云端保持一致；bidirectional=本地与云端互相补缺。",
			Default:     "local_primary",
			Options: []Option{
				{Value: "cloud_primary", Label: "网盘元数据为主"},
				{Value: "local_primary", Label: "本地元数据补缺"},
				{Value: "bidirectional", Label: "本地与云端互补"},
			},
		},
		{
			Key:         KeyStrmScrapeWriteMode,
			Type:        TypeString,
			Category:    "strm",
			Label:       "STRM 刮削写入策略",
			Description: "missing_only=仅补缺；overwrite=覆盖已有 nfo/海报。",
			Default:     "missing_only",
		},
		{
			Key:         KeyStrmScrapeSource,
			Type:        TypeSelect,
			Category:    "strm",
			Label:       "刮削数据源",
			Description: "tmdb=The Movie Database（需 API Key）；metatube=自托管 MetaTube 元数据服务（无需 Key）。",
			Default:     "tmdb",
			Options: []Option{
				{Value: "tmdb", Label: "TMDB"},
				{Value: "metatube", Label: "MetaTube"},
			},
		},
		{
			Key:         KeyStrmScrapeMetaTubeURL,
			Type:        TypeString,
			Category:    "strm",
			Label:       "MetaTube API 地址",
			Description: "MetaTube 服务地址，例如 https://your-metatube.example.com/。留空且数据源选 MetaTube 时刮削会提示未配置。",
			Default:     "",
		},
		{
			Key:         KeyMediaLibraryRoots,
			Type:        TypeString,
			Category:    "strm",
			Label:       "影视库根目录（JSON）",
			Description: "影视模式读取的刮削输出根目录列表，JSON 数组 [{id,name,path}]。建议在影视页「配置库」里维护，此处一般无需手填。",
			Default:     "[]",
		},
		{
			Key:         KeyMOProxyEnabled,
			Type:        TypeBool,
			Category:    "media_organize",
			Label:       "启用代理",
			Description: "TMDB 请求经代理出站。",
			Default:     "false",
		},
		{
			Key:         KeyMOProxyURL,
			Type:        TypeString,
			Category:    "media_organize",
			Label:       "代理地址",
			Description: "HTTP/HTTPS 代理地址，例如 http://127.0.0.1:7890。",
			Default:     "",
		},
		{
			Key:         KeyMOProxyUsername,
			Type:        TypeString,
			Category:    "media_organize",
			Label:       "代理用户名",
			Description: "代理认证用户名，无认证可留空。",
			Default:     "",
		},
		{
			Key:         KeyMOProxyPassword,
			Type:        TypeString,
			Category:    "media_organize",
			Label:       "代理密码",
			Description: "代理认证密码。",
			Default:     "",
			Sensitive:   true,
		},
		{
			Key:         KeyMOTmdbAPIKey,
			Type:        TypeString,
			Category:    "media_organize",
			Label:       "TMDB API Key",
			Description: "The Movie Database API 密钥。",
			Default:     "",
			Sensitive:   true,
		},
		{
			Key:         KeyMOTmdbLanguage,
			Type:        TypeString,
			Category:    "media_organize",
			Label:       "TMDB 搜索语言",
			Description: "TMDB 搜索与详情语言，例如 zh-CN。",
			Default:     "zh-CN",
		},
		{
			Key:         KeyMOAPIRequestIntervalMS,
			Type:        TypeInt,
			Category:    "media_organize",
			Label:       "API 额外补偿间隔",
			Description: "网盘 API 请求之间的额外等待时间。",
			Default:     "300",
			Unit:        "毫秒",
			Min:         intp(50),
			Max:         intp(10000),
		},
		{
			Key:         KeyMOTmdbRequestIntervalMS,
			Type:        TypeInt,
			Category:    "media_organize",
			Label:       "TMDB 请求间隔",
			Description: "两次 TMDB API 请求之间的最小间隔。",
			Default:     "250",
			Unit:        "毫秒",
			Min:         intp(100),
			Max:         intp(5000),
		},
		{
			Key:         KeyMOFileExtensions,
			Type:        TypeString,
			Category:    "media_organize",
			Label:       "媒体文件扩展名",
			Description: "参与整理的媒体扩展名，英文分号分隔。",
			Default:     "mkv;mp4;avi;ts;mov;wmv;iso;m2ts;rmvb;flv;m4v;webm",
		},
		{
			Key:         KeyMOMetadataExtensions,
			Type:        TypeString,
			Category:    "media_organize",
			Label:       "元数据文件扩展名",
			Description: "随媒体一起整理的元数据扩展名，英文分号分隔。",
			Default:     "nfo;ass;ssa;srt;sub;idx;sup;vtt;jpg;jpeg;png;webp;bmp",
		},
		{
			Key:         KeyMOMediaTagOrder,
			Type:        TypeString,
			Category:    "media_organize",
			Label:       "媒体信息标签排序",
			Description: "重命名时媒体标签的排列顺序，JSON 数组字符串。",
			Default:     `["screen_size","video_codec","audio_codec","audio_channels"]`,
		},
		{
			Key:         KeyMOAlignMediaTags,
			Type:        TypeBool,
			Category:    "media_organize",
			Label:       "强迫症模式",
			Description: "同后缀文件保持媒体信息标签一致。",
			Default:     "false",
		},
		{
			Key:         KeyMOMaxWorksPerRun,
			Type:        TypeInt,
			Category:    "media_organize",
			Label:       "每次最多整理作品数",
			Description: "单次执行最多处理的作品数，0 表示不限制。",
			Default:     "50",
			Min:         intp(0),
			Max:         intp(10000),
		},
		{
			Key:         KeyMOOverwriteExisting,
			Type:        TypeBool,
			Category:    "media_organize",
			Label:       "同名冲突时覆盖",
			Description: "目标位置已有同名文件时覆盖，默认跳过。",
			Default:     "false",
		},
	}
}

// categories 返回有序分组定义；只保留当前实际用到的分组。
func categories() []Category {
	return []Category{
		{ID: "system", Label: "系统设置"},
		{ID: "performance", Label: "性能设置"},
		{ID: "strm", Label: "STRM 设置"},
		{ID: "media_organize", Label: "媒体整理设置"},
		{ID: "rss", Label: "RSS 订阅"},
	}
}
