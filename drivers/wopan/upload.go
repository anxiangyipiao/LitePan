package wopan

import (
	"context"
	"fmt"
	"os"
	"strings"

	"litepan/internal/domain"
	"litepan/internal/driver"
	"litepan/internal/driver/uploadutil"

	wo "github.com/OpenListTeam/wopan-sdk-go"
)

// UploadLocalFile 从 LitePan 本地临时文件上传到沃云盘（SDK 分片上传）。
// wopan 无断点续传接口，ResumeState 忽略，暂停后从零重传。
func (d *Driver) UploadLocalFile(ctx context.Context, req driver.LocalUploadRequest) (*driver.LocalUploadResult, error) {
	c := d.woClient()
	if c == nil {
		return nil, domain.Errorf(domain.CodeInternal, "驱动尚未初始化")
	}
	targetName := strings.TrimSpace(req.FileName)
	if targetName == "" {
		return nil, domain.Errorf(domain.CodeValidation, "上传文件名不能为空")
	}
	if err := uploadutil.ValidateFileName(targetName); err != nil {
		return nil, err
	}
	localFile, err := uploadutil.StatLocalFile(req.LocalPath)
	if err != nil {
		return nil, err
	}
	parent := d.normalizeParent(req.ParentID)

	// skip 冲突策略：父目录已存在同名文件则跳过。
	if uploadutil.NormalizeConflictPolicy(req.ConflictPolicy) == "skip" {
		existing, err := d.findByName(ctx, parent, targetName)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return &driver.LocalUploadResult{
				FileID:   existing.ID,
				ParentID: parent,
				FileName: targetName,
				Size:     localFile.Size,
				Message:  fmt.Sprintf("文件 '%s' 已存在，已跳过", targetName),
				Skipped:  true,
			}, nil
		}
	}

	f, err := os.Open(localFile.Path)
	if err != nil {
		return nil, domain.Wrap(domain.CodeDriverError, err)
	}
	defer f.Close()

	uploadutil.NotifyProgress(req.OnProgress, 0, localFile.Size, "正在上传到沃云盘")
	fid, err := c.Upload2C(d.getSpaceType(), wo.Upload2CFile{
		Name:        targetName,
		Size:        localFile.Size,
		Content:     f,
		ContentType: "application/octet-stream",
	}, parent, d.familyID(), wo.Upload2COption{
		OnProgress: func(current, total int64) {
			uploadutil.NotifyProgress(req.OnProgress, current, total, "正在上传到沃云盘")
		},
		Ctx:        ctx,
		RetryTimes: 2,
	})
	if err != nil {
		return nil, mapWopanError(err)
	}
	uploadutil.NotifyProgress(req.OnProgress, localFile.Size, localFile.Size, "上传成功")

	fileID, fileName := d.resolveUploadedFile(ctx, parent, targetName, localFile.Size, fid)
	return &driver.LocalUploadResult{
		FileID:   fileID,
		ParentID: parent,
		FileName: fileName,
		Size:     localFile.Size,
		Message:  fmt.Sprintf("文件 '%s' 上传成功", fileName),
	}, nil
}

// resolveUploadedFile 上传完成后在父目录中定位新文件，拿到带 id 的复合标识；
// 找不到时退回用 fid 构造（后续下载可用，删除/移动有兜底风险）。
func (d *Driver) resolveUploadedFile(ctx context.Context, parent, targetName string, size int64, preferredFID string) (string, string) {
	items, err := d.ListFiles(ctx, parent)
	if err == nil {
		for _, item := range items {
			if !item.IsDir && item.Name == targetName {
				return item.ID, item.Name
			}
		}
		for _, item := range items {
			if !item.IsDir && item.Size == size {
				return item.ID, item.Name
			}
		}
	}
	if strings.TrimSpace(preferredFID) != "" {
		return encodeWopanID(preferredFID, preferredFID, false), targetName
	}
	return "", targetName
}

func (d *Driver) findByName(ctx context.Context, parent, name string) (*domain.FileItem, error) {
	items, err := d.ListFiles(ctx, parent)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].Name == name {
			return &items[i], nil
		}
	}
	return nil, nil
}
