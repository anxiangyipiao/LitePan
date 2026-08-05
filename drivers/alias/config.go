package alias

// Addition 是别名驱动的账号配置：把多个账号的目录虚拟合并成一个账号。
// paths 每行一个虚拟名=目标列表，目标形如「账号名:/路径」，路径可省略（默认该账号根目录）；
// 同一虚拟名可配多个目标（逗号分隔），打开虚拟目录时按文件名去重合并。
// 示例：
//
//	电影=网盘A:/电影,网盘B:/电影
//	电视剧=网盘A:/剧集
type Addition struct {
	Paths string `json:"paths" form:"required,full" type:"textarea" label:"别名映射" default:"电影=账号1:/电影,账号2:/电影"`
}
