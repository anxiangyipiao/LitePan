//go:build fuse

package fuse

import (
	"context"
	"io"
	"strings"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"

	"litepan/internal/domain"
	"litepan/internal/playback"
)

type cloudDir struct {
	fs.Inode
	b    *backend
	item domain.FileItem
}

var (
	_ fs.NodeLookuper  = (*cloudDir)(nil)
	_ fs.NodeReaddirer = (*cloudDir)(nil)
	_ fs.NodeGetattrer = (*cloudDir)(nil)
	_ fs.NodeOpendirer = (*cloudDir)(nil)
	_ fs.NodeMkdirer   = (*cloudDir)(nil)
	_ fs.NodeCreater   = (*cloudDir)(nil)
	_ fs.NodeUnlinker  = (*cloudDir)(nil)
	_ fs.NodeRmdirer   = (*cloudDir)(nil)
	_ fs.NodeRenamer   = (*cloudDir)(nil)
)

func (d *cloudDir) Getattr(_ context.Context, _ fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	fillDirAttr(&out.Attr, d.b, d.item)
	return 0
}

func (d *cloudDir) Opendir(_ context.Context) syscall.Errno { return 0 }

func (d *cloudDir) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	items, err := d.b.deps.Files.List(ctx, d.b.accountID(), d.item.ID, false)
	if err != nil {
		return nil, errnoFor(err)
	}
	entries := make([]fuse.DirEntry, 0, len(items))
	for _, it := range items {
		mode := fuse.S_IFREG
		if it.IsDir {
			mode = fuse.S_IFDIR
		}
		entries = append(entries, fuse.DirEntry{
			Name: it.Name,
			Mode: uint32(mode),
			Ino:  fileIno(d.b.accountID(), it.ID),
		})
	}
	return fs.NewListDirStream(entries), 0
}

func (d *cloudDir) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	item, errno := d.lookupChildItem(ctx, name)
	if errno != 0 {
		return nil, errno
	}
	child := d.newChild(ctx, item)
	if child == nil {
		return nil, syscall.EIO
	}
	fillEntryAttr(out, d.b, item)
	return child, 0
}

func (d *cloudDir) newChild(ctx context.Context, item domain.FileItem) *fs.Inode {
	ino := fileIno(d.b.accountID(), item.ID)
	if item.IsDir {
		return d.NewPersistentInode(ctx, &cloudDir{b: d.b, item: item}, fs.StableAttr{
			Ino:  ino,
			Mode: fuse.S_IFDIR,
		})
	}
	return d.NewPersistentInode(ctx, &cloudFile{b: d.b, item: item}, fs.StableAttr{
		Ino:  ino,
		Mode: fuse.S_IFREG,
	})
}

func (d *cloudDir) Mkdir(ctx context.Context, name string, _ uint32, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	name, errno := normalizeChildName(name)
	if errno != 0 {
		return nil, errno
	}
	item, err := d.b.deps.Files.CreateFolder(ctx, d.b.accountID(), d.item.ID, name)
	if err != nil {
		return nil, errnoFor(err)
	}
	if item == nil {
		return nil, syscall.EIO
	}
	child := d.newChild(ctx, *item)
	if child == nil {
		return nil, syscall.EIO
	}
	fillEntryAttr(out, d.b, *item)
	return child, 0
}

func (d *cloudDir) Unlink(ctx context.Context, name string) syscall.Errno {
	name, errno := normalizeChildName(name)
	if errno != 0 {
		return errno
	}
	item, errno := d.lookupChildItem(ctx, name)
	if errno != 0 {
		return errno
	}
	if item.IsDir {
		return syscall.EISDIR
	}
	if err := d.b.deps.Files.DeleteFiles(ctx, d.b.accountID(), []string{item.ID}, d.item.ID); err != nil {
		return errnoFor(err)
	}
	return 0
}

func (d *cloudDir) Rmdir(ctx context.Context, name string) syscall.Errno {
	name, errno := normalizeChildName(name)
	if errno != 0 {
		return errno
	}
	item, errno := d.lookupChildItem(ctx, name)
	if errno != 0 {
		return errno
	}
	if !item.IsDir {
		return syscall.ENOTDIR
	}
	if err := d.b.deps.Files.DeleteFiles(ctx, d.b.accountID(), []string{item.ID}, d.item.ID); err != nil {
		return errnoFor(err)
	}
	return 0
}

func (d *cloudDir) Rename(ctx context.Context, name string, newParent fs.InodeEmbedder, newName string, flags uint32) syscall.Errno {
	name, errno := normalizeChildName(name)
	if errno != 0 {
		return errno
	}
	newName, errno = normalizeChildName(newName)
	if errno != 0 {
		return errno
	}
	if flags != 0 {
		return syscall.EINVAL
	}
	targetDir, ok := newParent.(*cloudDir)
	if !ok {
		return syscall.EXDEV
	}
	if targetDir.b.mount == nil || d.b.mount == nil || targetDir.b.mount.ID != d.b.mount.ID {
		return syscall.EXDEV
	}
	item, errno := d.lookupChildItem(ctx, name)
	if errno != 0 {
		return errno
	}
	if targetDir.item.ID == d.item.ID && item.Name == newName {
		return 0
	}
	if item.IsDir && targetDir.item.ID == item.ID {
		return syscall.EINVAL
	}
	if _, found, errno := targetDir.lookupChildItemIfExists(ctx, newName); errno != 0 {
		return errno
	} else if found {
		return syscall.EEXIST
	}
	if targetDir.item.ID != d.item.ID {
		if err := d.b.deps.Files.MoveFiles(ctx, d.b.accountID(), []string{item.ID}, targetDir.item.ID, d.item.ID); err != nil {
			return errnoFor(err)
		}
		if newName != item.Name {
			if err := d.b.deps.Files.RenameFile(ctx, d.b.accountID(), item.ID, newName, targetDir.item.ID); err != nil {
				if rollbackErr := d.b.deps.Files.MoveFiles(ctx, d.b.accountID(), []string{item.ID}, d.item.ID, targetDir.item.ID); rollbackErr != nil && d.b.deps.Log != nil {
					d.b.deps.Log.Error("FUSE move+rename 回滚失败",
						"account_id", d.b.accountID(),
						"file_id", item.ID,
						"source_parent_id", d.item.ID,
						"target_parent_id", targetDir.item.ID,
						"target_name", newName,
						"rename_err", err,
						"rollback_err", rollbackErr,
					)
				}
				return errnoFor(err)
			}
		}
		return 0
	}
	if err := d.b.deps.Files.RenameFile(ctx, d.b.accountID(), item.ID, newName, d.item.ID); err != nil {
		return errnoFor(err)
	}
	return 0
}

func (d *cloudDir) lookupChildItem(ctx context.Context, name string) (domain.FileItem, syscall.Errno) {
	item, found, errno := d.lookupChildItemIfExists(ctx, name)
	if errno != 0 {
		return domain.FileItem{}, errno
	}
	if !found {
		return domain.FileItem{}, syscall.ENOENT
	}
	return item, 0
}

func (d *cloudDir) lookupChildItemIfExists(ctx context.Context, name string) (domain.FileItem, bool, syscall.Errno) {
	item, err := d.b.deps.Files.Lookup(ctx, d.b.accountID(), d.item.ID, name)
	if err != nil {
		if ae, ok := domain.AsAppError(err); ok && ae.Code == domain.CodeNotFound {
			return domain.FileItem{}, false, 0
		}
		return domain.FileItem{}, false, errnoFor(err)
	}
	return *item, true, 0
}

func normalizeChildName(name string) (string, syscall.Errno) {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || strings.Contains(name, "/") {
		return "", syscall.EINVAL
	}
	return name, 0
}

type cloudFile struct {
	fs.Inode
	b    *backend
	item domain.FileItem
}

var (
	_ fs.NodeGetattrer = (*cloudFile)(nil)
	_ fs.NodeOpener    = (*cloudFile)(nil)
	_ fs.NodeReader    = (*cloudFile)(nil)
	_ fs.FileReleaser  = (*remoteHandle)(nil)
)

func (f *cloudFile) Getattr(_ context.Context, _ fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	fillFileAttr(&out.Attr, f.b, f.item)
	return 0
}

func (f *cloudFile) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	if flags&syscall.O_ACCMODE != syscall.O_RDONLY {
		return nil, 0, syscall.EROFS
	}
	if f.b.deps.Playback == nil {
		return nil, 0, syscall.EIO
	}
	reader, err := f.b.deps.Playback.OpenRemoteReader(ctx, f.b.accountID(), f.item.ID, "")
	if err != nil {
		return nil, 0, errnoFor(err)
	}
	if reader.Size() > 0 {
		f.item.Size = reader.Size()
	}
	return &remoteHandle{reader: reader}, 0, 0
}

func (f *cloudFile) Read(ctx context.Context, fh fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	h, ok := fh.(*remoteHandle)
	if !ok || h.reader == nil {
		return nil, syscall.EIO
	}
	var n int
	var err error
	if rc := f.b.deps.ReadCache; rc != nil && rc.Enabled(ctx) {
		n, err = rc.ReadAt(ctx, f.b.accountID(), f.item.ID, dest, off, h.reader.ReadAt)
	} else {
		n, err = h.reader.ReadAt(dest, off)
	}
	if n > 0 {
		return fuse.ReadResultData(dest[:n]), 0
	}
	if err == io.EOF {
		return fuse.ReadResultData(nil), 0
	}
	if err != nil {
		return nil, syscall.EIO
	}
	return fuse.ReadResultData(nil), 0
}

type remoteHandle struct {
	reader *playback.RemoteReader
}

func (h *remoteHandle) Release(_ context.Context) syscall.Errno {
	if h == nil || h.reader == nil {
		return 0
	}
	_ = h.reader.Close()
	h.reader = nil
	return 0
}

func fillDirAttr(out *fuse.Attr, b *backend, item domain.FileItem) {
	out.Mode = b.mount.DirMode & 0o7777
	out.Nlink = 2
	out.Uid = b.mount.UID
	out.Gid = b.mount.GID
	if !item.ModTime.IsZero() {
		sec := uint64(item.ModTime.Unix())
		out.Mtime = sec
		out.Atime = sec
		out.Ctime = sec
	} else {
		sec := uint64(time.Now().Unix())
		out.Mtime = sec
		out.Atime = sec
		out.Ctime = sec
	}
}

func fillFileAttr(out *fuse.Attr, b *backend, item domain.FileItem) {
	out.Mode = b.mount.FileMode & 0o7777
	out.Nlink = 1
	out.Uid = b.mount.UID
	out.Gid = b.mount.GID
	out.Size = uint64(item.Size)
	if !item.ModTime.IsZero() {
		sec := uint64(item.ModTime.Unix())
		out.Mtime = sec
		out.Atime = sec
		out.Ctime = sec
	}
	const bs = 4096
	out.Blksize = bs
	if item.Size > 0 {
		out.Blocks = (uint64(item.Size) + bs - 1) / bs
	}
}

func fillEntryAttr(out *fuse.EntryOut, b *backend, item domain.FileItem) {
	if item.IsDir {
		fillDirAttr(&out.Attr, b, item)
	} else {
		fillFileAttr(&out.Attr, b, item)
	}
}

func errnoFor(err error) syscall.Errno {
	if err == nil {
		return 0
	}
	if ae, ok := domain.AsAppError(err); ok {
		switch ae.Code {
		case domain.CodeNotFound:
			return syscall.ENOENT
		case domain.CodePermissionDenied:
			return syscall.EACCES
		case domain.CodeValidation:
			return syscall.EINVAL
		}
	}
	return syscall.EIO
}
