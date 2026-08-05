package wopan

import (
	"context"
	"net/http"
	"strings"

	"litepan/internal/domain"
	"litepan/internal/driver"
)

// ResolveDownload 解析下载直链。wopan 的 downloadUrl 自含签名，默认 302 重定向；
// proxy 模式由 LitePan 本机代理转发，需带上 Accesstoken 等鉴权头。
func (d *Driver) ResolveDownload(ctx context.Context, req driver.DownloadRequest) (*domain.DownloadInfo, error) {
	c := d.woClient()
	if c == nil {
		return nil, domain.Errorf(domain.CodeInternal, "驱动尚未初始化")
	}
	fid := decodeWopanFID(strings.TrimSpace(req.FileID))
	if fid == "" {
		return nil, domain.Errorf(domain.CodeValidation, "无法解析下载标识")
	}
	if err := d.waitOperationDelay(ctx); err != nil {
		return nil, err
	}
	res, err := c.GetDownloadUrlV2([]string{fid}, d.sdkOpts(ctx)...)
	if err != nil {
		return nil, mapWopanError(err)
	}
	if len(res.List) == 0 || strings.TrimSpace(res.List[0].DownloadUrl) == "" {
		return nil, domain.Errorf(domain.CodeDriverError, "获取下载链接失败：响应缺少 downloadUrl")
	}

	info := &domain.DownloadInfo{
		URL:  res.List[0].DownloadUrl,
		Mode: domain.DownloadRedirect,
	}
	if strings.EqualFold(strings.TrimSpace(d.add.DownloadMode), "proxy") {
		info.Mode = domain.DownloadProxy
		info.Headers = http.Header{}
		info.Headers.Set("Accesstoken", d.currentToken())
		info.Headers.Set("Origin", "https://pan.wo.cn")
		info.Headers.Set("Referer", "https://pan.wo.cn/")
	}
	return info, nil
}

func (d *Driver) DeleteFiles(ctx context.Context, fileIDs []string) error {
	c := d.woClient()
	if c == nil {
		return domain.Errorf(domain.CodeInternal, "驱动尚未初始化")
	}
	dirs, files := splitWopanIDs(fileIDs)
	if len(dirs) == 0 && len(files) == 0 {
		return nil
	}
	if err := d.waitOperationDelay(ctx); err != nil {
		return err
	}
	err := c.DeleteFile(d.getSpaceType(), dirs, files, d.sdkOpts(ctx)...)
	return mapWopanError(err)
}

func (d *Driver) MoveFiles(ctx context.Context, fileIDs []string, targetParentID, _ string) error {
	c := d.woClient()
	if c == nil {
		return domain.Errorf(domain.CodeInternal, "驱动尚未初始化")
	}
	dirs, files := splitWopanIDs(fileIDs)
	if len(dirs) == 0 && len(files) == 0 {
		return nil
	}
	target := d.normalizeParent(targetParentID)
	if err := d.waitOperationDelay(ctx); err != nil {
		return err
	}
	err := c.MoveFile(dirs, files, target, d.getSpaceType(), d.getSpaceType(), d.familyID(), d.familyID(), d.sdkOpts(ctx)...)
	return mapWopanError(err)
}

func (d *Driver) CopyFiles(ctx context.Context, fileIDs []string, targetParentID string) error {
	c := d.woClient()
	if c == nil {
		return domain.Errorf(domain.CodeInternal, "驱动尚未初始化")
	}
	dirs, files := splitWopanIDs(fileIDs)
	if len(dirs) == 0 && len(files) == 0 {
		return nil
	}
	target := d.normalizeParent(targetParentID)
	if err := d.waitOperationDelay(ctx); err != nil {
		return err
	}
	err := c.CopyFile(dirs, files, target, d.getSpaceType(), d.getSpaceType(), d.familyID(), d.familyID(), d.sdkOpts(ctx)...)
	return mapWopanError(err)
}

func (d *Driver) RenameFile(ctx context.Context, fileID, newName string) error {
	c := d.woClient()
	if c == nil {
		return domain.Errorf(domain.CodeInternal, "驱动尚未初始化")
	}
	id := strings.TrimSpace(fileID)
	name := strings.TrimSpace(newName)
	if id == "" {
		return domain.Errorf(domain.CodeValidation, "file_id 不能为空")
	}
	if name == "" {
		return domain.Errorf(domain.CodeValidation, "新名称不能为空")
	}
	_type := 1
	if isWopanDir(id) {
		_type = 0
	}
	if err := d.waitOperationDelay(ctx); err != nil {
		return err
	}
	err := c.RenameFileOrDirectory(d.getSpaceType(), _type, decodeWopanID(id), name, d.familyID(), d.sdkOpts(ctx)...)
	return mapWopanError(err)
}

func (d *Driver) CreateFolder(ctx context.Context, parentID, name string) (*domain.FileItem, error) {
	c := d.woClient()
	if c == nil {
		return nil, domain.Errorf(domain.CodeInternal, "驱动尚未初始化")
	}
	folderName := strings.TrimSpace(name)
	if folderName == "" {
		return nil, domain.Errorf(domain.CodeValidation, "文件夹名称不能为空")
	}
	parent := d.normalizeParent(parentID)
	if err := d.waitOperationDelay(ctx); err != nil {
		return nil, err
	}
	res, err := c.CreateDirectory(d.getSpaceType(), parent, folderName, d.effectiveFamilyID(), d.sdkOpts(ctx)...)
	if err != nil {
		return nil, mapWopanError(err)
	}
	return &domain.FileItem{
		ID:     encodeWopanID(res.Id, "", true),
		Name:   folderName,
		IsDir:  true,
		IDKind: domain.IDStable,
	}, nil
}
