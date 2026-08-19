package fusereadcache

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

type blockMeta struct {
	AccountID  int64
	FileID     string
	BlockIdx   int64
	ByteLen    int64
	CreatedAt  int64
	AccessedAt int64
}

type blockKey struct {
	AccountID int64
	FileID    string
	BlockIdx  int64
}

type storeLayer struct {
	root        string
	blocks      string
	db          *sql.DB
	lastTouches map[blockKey]int64
	// touchesMu 只保护 lastTouches：touchBlock 在 s.mu.RLock 下运行，
	// 并发读会同时写该 map，不隔离会触发 "concurrent map writes"。
	touchesMu sync.Mutex
	// usedBytes 是块占用量的内存计数，避免热路径上每次写块都做全表 SUM 扫描。
	// 只在启动时用 DB 统计播种一次，之后由 putBlock/deleteBlock/clearAll 维护。
	usedBytes atomic.Int64
}

func openStore(ctx context.Context, root string) (*storeLayer, error) {
	if err := os.MkdirAll(filepath.Join(root, "blocks"), 0o755); err != nil {
		return nil, err
	}
	dsn := "file:" + filepath.Join(root, "index.sqlite") +
		"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)" +
		"&_pragma=foreign_keys(on)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS blocks (
  account_id INTEGER NOT NULL,
  file_id TEXT NOT NULL,
  block_idx INTEGER NOT NULL,
  byte_len INTEGER NOT NULL,
  created_at INTEGER NOT NULL,
  accessed_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, file_id, block_idx)
);
CREATE INDEX IF NOT EXISTS idx_blocks_accessed ON blocks(accessed_at);
`); err != nil {
		_ = db.Close()
		return nil, err
	}
	st := &storeLayer{
		root:        root,
		blocks:      filepath.Join(root, "blocks"),
		db:          db,
		lastTouches: make(map[blockKey]int64),
	}
	// 用现有索引一次性播种 usedBytes（仅启动时一次全表统计）。
	if used, _, err := st.stats(); err == nil {
		st.usedBytes.Store(used)
	}
	return st, nil
}

func (s *storeLayer) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func fileDir(fileID string) string {
	sum := sha256.Sum256([]byte(fileID))
	return hex.EncodeToString(sum[:16])
}

func (s *storeLayer) blockPath(accountID int64, fileID string, blockIdx int64) string {
	return filepath.Join(s.blocks, strconv.FormatInt(accountID, 10), fileDir(fileID), strconv.FormatInt(blockIdx, 10)+".bin")
}

func (s *storeLayer) loadBlockRange(accountID int64, fileID string, blockIdx, blockOff int64, dest []byte) (int, bool, error) {
	path := s.blockPath(accountID, fileID, blockIdx)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, false, nil
		}
		return 0, false, err
	}
	defer f.Close()
	n, readErr := f.ReadAt(dest, blockOff)
	if readErr != nil && readErr != io.EOF {
		return n, true, readErr
	}
	if err := s.touchBlock(accountID, fileID, blockIdx); err != nil {
		return n, true, err
	}
	return n, true, readErr
}

func (s *storeLayer) touchBlock(accountID int64, fileID string, blockIdx int64) error {
	now := time.Now().Unix()
	key := blockKey{AccountID: accountID, FileID: fileID, BlockIdx: blockIdx}
	s.touchesMu.Lock()
	if s.lastTouches[key] == now {
		s.touchesMu.Unlock()
		return nil
	}
	s.touchesMu.Unlock()
	_, err := s.db.Exec(`
UPDATE blocks SET accessed_at=? WHERE account_id=? AND file_id=? AND block_idx=?`,
		now, accountID, fileID, blockIdx)
	if err == nil {
		s.touchesMu.Lock()
		s.lastTouches[key] = now
		s.touchesMu.Unlock()
	}
	return err
}

func (s *storeLayer) putBlock(accountID int64, fileID string, blockIdx int64, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	path := s.blockPath(accountID, fileID, blockIdx)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	now := time.Now().Unix()
	_, err := s.db.Exec(`
INSERT INTO blocks(account_id,file_id,block_idx,byte_len,created_at,accessed_at)
VALUES(?,?,?,?,?,?)
ON CONFLICT(account_id,file_id,block_idx) DO UPDATE SET
  byte_len=excluded.byte_len,
  accessed_at=excluded.accessed_at`,
		accountID, fileID, blockIdx, len(data), now, now)
	if err == nil {
		s.usedBytes.Add(int64(len(data)))
		s.touchesMu.Lock()
		s.lastTouches[blockKey{AccountID: accountID, FileID: fileID, BlockIdx: blockIdx}] = now
		s.touchesMu.Unlock()
	}
	return err
}

func (s *storeLayer) deleteBlock(meta blockMeta) error {
	_ = os.Remove(s.blockPath(meta.AccountID, meta.FileID, meta.BlockIdx))
	s.touchesMu.Lock()
	delete(s.lastTouches, blockKey{AccountID: meta.AccountID, FileID: meta.FileID, BlockIdx: meta.BlockIdx})
	s.touchesMu.Unlock()
	_, err := s.db.Exec(`DELETE FROM blocks WHERE account_id=? AND file_id=? AND block_idx=?`,
		meta.AccountID, meta.FileID, meta.BlockIdx)
	if err == nil {
		s.usedBytes.Add(-meta.ByteLen)
	}
	return err
}

func (s *storeLayer) stats() (used int64, blocks int64, err error) {
	row := s.db.QueryRow(`SELECT COALESCE(SUM(byte_len),0), COUNT(*) FROM blocks`)
	if err := row.Scan(&used, &blocks); err != nil {
		return 0, 0, err
	}
	return used, blocks, nil
}

func (s *storeLayer) expireBefore(cutoff int64) error {
	rows, err := s.db.Query(`
SELECT account_id,file_id,block_idx,byte_len,created_at,accessed_at
FROM blocks WHERE created_at < ?`, cutoff)
	if err != nil {
		return err
	}
	var metas []blockMeta
	for rows.Next() {
		var m blockMeta
		if err := rows.Scan(&m.AccountID, &m.FileID, &m.BlockIdx, &m.ByteLen, &m.CreatedAt, &m.AccessedAt); err != nil {
			_ = rows.Close()
			return err
		}
		metas = append(metas, m)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, m := range metas {
		if err := s.deleteBlock(m); err != nil {
			return err
		}
	}
	return nil
}

func (s *storeLayer) pickEvictLRU() (blockMeta, bool, error) {
	row := s.db.QueryRow(`
SELECT account_id,file_id,block_idx,byte_len,created_at,accessed_at
FROM blocks ORDER BY accessed_at ASC LIMIT 1`)
	var m blockMeta
	if err := row.Scan(&m.AccountID, &m.FileID, &m.BlockIdx, &m.ByteLen, &m.CreatedAt, &m.AccessedAt); err != nil {
		if err == sql.ErrNoRows {
			return blockMeta{}, false, nil
		}
		return blockMeta{}, false, err
	}
	return m, true, nil
}

func (s *storeLayer) pickEvictLargeFile() (blockMeta, bool, error) {
	row := s.db.QueryRow(`
SELECT b.account_id,b.file_id,b.block_idx,b.byte_len,b.created_at,b.accessed_at
FROM blocks b
JOIN (
  SELECT account_id,file_id,SUM(byte_len) AS total
  FROM blocks GROUP BY account_id,file_id
  ORDER BY total DESC LIMIT 1
) t ON b.account_id=t.account_id AND b.file_id=t.file_id
ORDER BY b.accessed_at ASC LIMIT 1`)
	var m blockMeta
	if err := row.Scan(&m.AccountID, &m.FileID, &m.BlockIdx, &m.ByteLen, &m.CreatedAt, &m.AccessedAt); err != nil {
		if err == sql.ErrNoRows {
			return blockMeta{}, false, nil
		}
		return blockMeta{}, false, err
	}
	return m, true, nil
}

func (s *storeLayer) invalidateFile(accountID int64, fileID string) error {
	rows, err := s.db.Query(`
SELECT account_id,file_id,block_idx,byte_len,created_at,accessed_at
FROM blocks WHERE account_id=? AND file_id=?`, accountID, fileID)
	if err != nil {
		return err
	}
	var metas []blockMeta
	for rows.Next() {
		var m blockMeta
		if err := rows.Scan(&m.AccountID, &m.FileID, &m.BlockIdx, &m.ByteLen, &m.CreatedAt, &m.AccessedAt); err != nil {
			_ = rows.Close()
			return err
		}
		metas = append(metas, m)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, m := range metas {
		if err := s.deleteBlock(m); err != nil {
			return err
		}
	}
	_ = os.RemoveAll(filepath.Join(s.blocks, strconv.FormatInt(accountID, 10), fileDir(fileID)))
	return nil
}

func (s *storeLayer) invalidateAccount(accountID int64) error {
	rows, err := s.db.Query(`
SELECT account_id,file_id,block_idx,byte_len,created_at,accessed_at
FROM blocks WHERE account_id=?`, accountID)
	if err != nil {
		return err
	}
	var metas []blockMeta
	for rows.Next() {
		var m blockMeta
		if err := rows.Scan(&m.AccountID, &m.FileID, &m.BlockIdx, &m.ByteLen, &m.CreatedAt, &m.AccessedAt); err != nil {
			_ = rows.Close()
			return err
		}
		metas = append(metas, m)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, m := range metas {
		if err := s.deleteBlock(m); err != nil {
			return err
		}
	}
	_ = os.RemoveAll(filepath.Join(s.blocks, strconv.FormatInt(accountID, 10)))
	return nil
}

func (s *storeLayer) clearAll() error {
	if err := os.RemoveAll(s.blocks); err != nil {
		return err
	}
	if err := os.MkdirAll(s.blocks, 0o755); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM blocks`)
	if err == nil {
		s.usedBytes.Store(0)
		s.touchesMu.Lock()
		clear(s.lastTouches)
		s.touchesMu.Unlock()
	}
	return err
}

func (s *storeLayer) rootPath() string {
	if s == nil {
		return ""
	}
	return s.root
}

func validateRoot(root string) error {
	if root == "" {
		return fmt.Errorf("empty cache root")
	}
	return nil
}
