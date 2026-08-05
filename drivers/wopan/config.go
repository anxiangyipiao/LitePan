package wopan

// Addition 是沃云盘账号配置。访问令牌可留空：仅凭 refresh_token 即可在 Init/Refresh 时自动换新。
type Addition struct {
	AccessToken  string `json:"access_token" label:"访问令牌 access_token（可选）" type:"password" form:"pair=auth"`
	RefreshToken string `json:"refresh_token" label:"刷新令牌 refresh_token" type:"password" form:"required,pair=auth"`
	FamilyID     string `json:"family_id" label:"家庭空间 ID（留空使用个人空间）" form:"pair=opts1"`
	SortRule     string `json:"sort_rule" label:"列表排序" type:"select" options:"name_asc:名称升序,name_desc:名称降序,time_asc:时间升序,time_desc:时间降序,size_asc:大小升序,size_desc:大小降序" default:"name_asc" form:"pair=opts1"`
	DownloadMode string `json:"download_mode" label:"下载模式" type:"select" options:"redirect:302重定向,proxy:本机代理" default:"redirect" form:"pair=opts2"`
	RootFolderID string `json:"root_folder_id" label:"根目录 ID" default:"0" form:"pair=opts2"`
}
