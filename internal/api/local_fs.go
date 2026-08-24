package api

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"litepan/internal/domain"
)

type localDirEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type localBrowseResult struct {
	Path     string          `json:"path"`
	Parent   *string         `json:"parent"`
	Dirs     []localDirEntry `json:"dirs"`
	Exists   bool            `json:"exists"`
	Writable bool            `json:"writable"`
}

func browseDefaultPath() string {
	if runtime.GOOS == "windows" {
		if dir, err := os.Getwd(); err == nil {
			if vol := filepath.VolumeName(dir); vol != "" {
				return vol + string(os.PathSeparator)
			}
		}
		return `C:\`
	}
	for _, candidate := range []string{"/app/strm", "/app/data", "/app/mounts", "/app", "/data", "/mnt", "/media", "/"} {
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate
		}
	}
	return "/"
}

func (h *Handler) browseLocalFS(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.URL.Query().Get("path"))
	if raw == "" {
		raw = browseDefaultPath()
	}
	if !filepath.IsAbs(raw) {
		writeJSON(w, http.StatusOK, Resp{
			Success:   false,
			Message:   "请使用绝对路径（如 /mnt/disk 或 D:\\backup）",
			ErrorType: string(domain.CodeValidation),
		})
		return
	}

	target, err := filepath.Abs(raw)
	if err != nil {
		writeErr(w, domain.Wrap(domain.CodeValidation, err))
		return
	}
	target = filepath.Clean(target)

	parent := parentPath(target)
	info, err := os.Stat(target)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusOK, Resp{
				Success: false,
				Message: "目录不存在: " + target,
				Data: localBrowseResult{
					Path:   target,
					Parent: parent,
					Dirs:   []localDirEntry{},
					Exists: false,
				},
			})
			return
		}
		writeErr(w, domain.Wrap(domain.CodeDriverError, err))
		return
	}
	if !info.IsDir() {
		writeJSON(w, http.StatusOK, Resp{
			Success:   false,
			Message:   "该路径不是目录: " + target,
			ErrorType: string(domain.CodeValidation),
		})
		return
	}

	entries, err := os.ReadDir(target)
	if err != nil {
		if os.IsPermission(err) {
			writeJSON(w, http.StatusOK, Resp{
				Success:   false,
				Message:   "无读取权限: " + target,
				ErrorType: string(domain.CodePermissionDenied),
			})
			return
		}
		writeErr(w, domain.Wrap(domain.CodeDriverError, err))
		return
	}

	dirs := make([]localDirEntry, 0, len(entries))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if !e.IsDir() {
			continue
		}
		dirs = append(dirs, localDirEntry{
			Name: e.Name(),
			Path: filepath.Join(target, e.Name()),
		})
	}
	sort.Slice(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name)
	})

	writeOK(w, localBrowseResult{
		Path:     target,
		Parent:   parent,
		Dirs:     dirs,
		Exists:   true,
		Writable: true,
	})
}

func parentPath(path string) *string {
	clean := filepath.Clean(path)
	if clean == "/" {
		return nil
	}
	p := filepath.Dir(clean)
	return &p
}
