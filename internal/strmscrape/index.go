package strmscrape

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"litepan/internal/domain"

	_ "modernc.org/sqlite"
)

const indexSchemaVersion = "2"

// TaskIndexPath 返回任务刮削索引库路径：data/strmscrape/{task_id}.sqlite
func TaskIndexPath(dataDir string, taskID int64) string {
	return filepath.Join(strings.TrimSpace(dataDir), "strmscrape", strconv.FormatInt(taskID, 10)+".sqlite")
}

// RemoveTaskIndex 删除任务索引及其 WAL/SHM（任务删除时调用）。
func RemoveTaskIndex(dataDir string, taskID int64) {
	base := TaskIndexPath(dataDir, taskID)
	for _, p := range []string{base, base + "-wal", base + "-shm"} {
		_ = os.Remove(p)
	}
}

func (s *Service) indexPath(taskID int64) string {
	return TaskIndexPath(s.dataDir, taskID)
}

func (s *Service) withTaskIndexLock(taskID int64, fn func() error) error {
	v, _ := s.indexLocks.LoadOrStore(taskID, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	return fn()
}

func openTaskIndexDB(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	dsn := "file:" + filepath.ToSlash(abs) +
		"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureIndexSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func ensureIndexSchema(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS items (
  id TEXT PRIMARY KEY,
  rel_dir TEXT NOT NULL DEFAULT '',
  strm_name TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL DEFAULT '',
  year INTEGER,
  media_type TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT '',
  has_nfo INTEGER NOT NULL DEFAULT 0,
  has_poster INTEGER NOT NULL DEFAULT 0,
  has_pending INTEGER NOT NULL DEFAULT 0,
  tmdb_id TEXT NOT NULL DEFAULT '',
  poster_rel TEXT NOT NULL DEFAULT '',
  folder_name TEXT NOT NULL DEFAULT '',
  file_count INTEGER NOT NULL DEFAULT 0,
  ep_local INTEGER NOT NULL DEFAULT 0,
  ep_tmdb INTEGER NOT NULL DEFAULT 0,
  ep_scraped INTEGER NOT NULL DEFAULT 0,
  tv_state TEXT NOT NULL DEFAULT '',
  added_at TEXT NOT NULL DEFAULT '',
  genres_csv TEXT NOT NULL DEFAULT '',
  actors_csv TEXT NOT NULL DEFAULT ''
);
`)
	if err != nil {
		return err
	}
	_ = migrateIndexFacets(db)
	return nil
}

func migrateIndexFacets(db *sql.DB) error {
	cols := map[string]bool{}
	rows, err := db.Query(`SELECT name FROM pragma_table_info('items')`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		cols[name] = true
	}
	if !cols["genres_csv"] {
		if _, err := db.Exec(`ALTER TABLE items ADD COLUMN genres_csv TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	if !cols["actors_csv"] {
		if _, err := db.Exec(`ALTER TABLE items ADD COLUMN actors_csv TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (s *Service) indexFileExists(taskID int64) bool {
	st, err := os.Stat(s.indexPath(taskID))
	return err == nil && !st.IsDir()
}

func readIndexMeta(db *sql.DB, key string) (string, bool) {
	var v string
	err := db.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	if err != nil {
		return "", false
	}
	return v, true
}

func writeIndexMeta(tx *sql.Tx, key, value string) error {
	_, err := tx.Exec(`INSERT INTO meta(key, value) VALUES(?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func itemPosterRel(root string, g workGroup, mediaType string) string {
	if !workHasPoster(g, mediaType) {
		return ""
	}
	return filepath.ToSlash(relUnder(root, workPosterFile(g, mediaType)))
}

// posterURLFromRel 生成海报访问 URL。root 为刮削根目录绝对路径（任务输出目录或自定义目录）。
func posterURLFromRel(root string, rel string) string {
	rel = strings.TrimSpace(rel)
	root = strings.TrimSpace(root)
	if rel == "" || root == "" {
		return ""
	}
	return "/api/admin/strm-scrape/poster?root=" + url.QueryEscape(filepath.ToSlash(root)) + "&rel=" + pathEscape(rel)
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// RebuildIndex 扫盘重建索引。root 非空为自定义目录模式，否则按任务输出目录。
func (s *Service) RebuildIndex(ctx context.Context, strmTaskID int64, root string) error {
	sr, err := s.resolveScrapeRoot(ctx, strmTaskID, root)
	if err != nil {
		return err
	}
	return s.withTaskIndexLock(sr.indexKey, func() error {
		return s.rebuildIndexLocked(ctx, sr.indexKey, sr.root)
	})
}

// rebuildIndexLocked 在锁内重建索引。strmTaskID 即索引键（任务 ID 或负数自定义键），不再派生。
// root 为空时回退到任务输出目录解析。
func (s *Service) rebuildIndexLocked(ctx context.Context, strmTaskID int64, root string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var err error
	if strings.TrimSpace(root) == "" {
		_, root, err = s.resolveTask(ctx, strmTaskID)
		if err != nil {
			return err
		}
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return err
	}
	st, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return domain.Errorf(domain.CodeValidation, "STRM 输出目录不存在：%s", root)
		}
		return err
	}
	if !st.IsDir() {
		return domain.Errorf(domain.CodeValidation, "STRM 输出目录无效：%s", root)
	}
	works, err := scanWorks(root)
	if err != nil {
		return err
	}
	items := make([]Item, 0, len(works))
	rels := make([]string, 0, len(works))
	for _, g := range works {
		if err := ctx.Err(); err != nil {
			return err
		}
		it := buildItem(root, g)
		items = append(items, it)
		rels = append(rels, itemPosterRel(root, g, it.MediaType))
	}

	db, err := openTaskIndexDB(s.indexPath(strmTaskID))
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM items`); err != nil {
		return err
	}
	for i, it := range items {
		if err := upsertItemTx(tx, it, rels[i]); err != nil {
			return err
		}
	}
	if err := writeIndexMeta(tx, "schema", indexSchemaVersion); err != nil {
		return err
	}
	if err := writeIndexMeta(tx, "root", root); err != nil {
		return err
	}
	if err := writeIndexMeta(tx, "built_at", time.Now().UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	return tx.Commit()
}

func upsertItemTx(tx *sql.Tx, it Item, posterRel string) error {
	var year any
	if it.Year != nil {
		year = *it.Year
	}
	genresCSV := strings.Join(it.Genres, "\x1f")
	actorsCSV := strings.Join(it.Actors, "\x1f")
	_, err := tx.Exec(`
INSERT INTO items (
  id, rel_dir, strm_name, title, year, media_type, status,
  has_nfo, has_poster, has_pending, tmdb_id, poster_rel, folder_name,
  file_count, ep_local, ep_tmdb, ep_scraped, tv_state, added_at, genres_csv, actors_csv
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  rel_dir=excluded.rel_dir,
  strm_name=excluded.strm_name,
  title=excluded.title,
  year=excluded.year,
  media_type=excluded.media_type,
  status=excluded.status,
  has_nfo=excluded.has_nfo,
  has_poster=excluded.has_poster,
  has_pending=excluded.has_pending,
  tmdb_id=excluded.tmdb_id,
  poster_rel=excluded.poster_rel,
  folder_name=excluded.folder_name,
  file_count=excluded.file_count,
  ep_local=excluded.ep_local,
  ep_tmdb=excluded.ep_tmdb,
  ep_scraped=excluded.ep_scraped,
  tv_state=excluded.tv_state,
  added_at=excluded.added_at,
  genres_csv=excluded.genres_csv,
  actors_csv=excluded.actors_csv
`, it.ID, it.RelDir, it.StrmName, it.Title, year, it.MediaType, it.Status,
		boolToInt(it.HasNFO), boolToInt(it.HasPoster), boolToInt(it.HasPending),
		it.TMDBID, posterRel, it.FolderName, it.FileCount, it.EpLocal, it.EpTMDB,
		it.EpScraped, it.TVState, it.AddedAt, genresCSV, actorsCSV)
	return err
}

func (s *Service) upsertIndexItem(ctx context.Context, strmTaskID int64, root string, g workGroup) {
	_ = s.withTaskIndexLock(strmTaskID, func() error {
		if abs, err := filepath.Abs(root); err == nil {
			root = abs
		}
		if !s.indexFileExists(strmTaskID) {
			return s.rebuildIndexLocked(ctx, strmTaskID, root)
		}
		db, err := openTaskIndexDB(s.indexPath(strmTaskID))
		if err != nil {
			return err
		}
		defer db.Close()
		if stored, ok := readIndexMeta(db, "root"); ok && stored != "" {
			storedAbs := stored
			if abs, err := filepath.Abs(stored); err == nil {
				storedAbs = abs
			}
			if storedAbs != root {
				return s.rebuildIndexLocked(ctx, strmTaskID, root)
			}
		}
		it := buildItem(root, g)
		rel := itemPosterRel(root, g, it.MediaType)
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		if err := upsertItemTx(tx, it, rel); err != nil {
			return err
		}
		return tx.Commit()
	})
}

func buildItemListWhere(query ItemListQuery) (string, []any) {
	clauses := make([]string, 0, 6)
	args := make([]any, 0, 10)
	if query.Keyword != "" {
		kw := "%" + strings.ToLower(query.Keyword) + "%"
		clauses = append(clauses, `(LOWER(title) LIKE ? OR LOWER(folder_name) LIKE ? OR LOWER(strm_name) LIKE ? OR LOWER(tmdb_id) LIKE ?)`)
		args = append(args, kw, kw, kw, kw)
	}
	if query.Status != "" {
		clauses = append(clauses, `status = ?`)
		args = append(args, query.Status)
	}
	if query.MediaType != "" {
		clauses = append(clauses, `media_type = ?`)
		args = append(args, query.MediaType)
	}
	if query.TVState != "" {
		clauses = append(clauses, `tv_state = ?`)
		args = append(args, query.TVState)
	}
	if strings.TrimSpace(query.Genre) != "" {
		clauses = append(clauses, `instr(chr(31) || genres_csv || chr(31), chr(31) || ? || chr(31)) > 0`)
		args = append(args, strings.TrimSpace(query.Genre))
	}
	if strings.TrimSpace(query.Actor) != "" {
		clauses = append(clauses, `instr(chr(31) || actors_csv || chr(31), chr(31) || ? || chr(31)) > 0`)
		args = append(args, strings.TrimSpace(query.Actor))
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

func itemListOrderBy(sort ItemListSort) string {
	switch sort {
	case ItemListSortTitleAsc:
		return "ORDER BY title COLLATE NOCASE ASC, added_at DESC"
	case ItemListSortYearDesc:
		return "ORDER BY CASE WHEN year IS NULL THEN 1 ELSE 0 END ASC, year DESC, title COLLATE NOCASE ASC"
	case ItemListSortYearAsc:
		return "ORDER BY CASE WHEN year IS NULL THEN 1 ELSE 0 END ASC, year ASC, title COLLATE NOCASE ASC"
	case ItemListSortAddedAsc:
		return "ORDER BY added_at ASC, title COLLATE NOCASE ASC"
	default:
		return "ORDER BY added_at DESC, title COLLATE NOCASE ASC"
	}
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, "\x1f")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// scanIndexItems 按行扫描索引条目，海报 URL 用索引库 meta 里记录的 root 重建。
func scanIndexItems(rows *sql.Rows, root string) ([]Item, error) {
	out := make([]Item, 0, 128)
	for rows.Next() {
		var it Item
		var year sql.NullInt64
		var hasNFO, hasPoster, hasPending int
		var posterRel, genresCSV, actorsCSV string
		if err := rows.Scan(
			&it.ID, &it.RelDir, &it.StrmName, &it.Title, &year, &it.MediaType, &it.Status,
			&hasNFO, &hasPoster, &hasPending, &it.TMDBID, &posterRel, &it.FolderName,
			&it.FileCount, &it.EpLocal, &it.EpTMDB, &it.EpScraped, &it.TVState, &it.AddedAt, &genresCSV, &actorsCSV,
		); err != nil {
			return nil, err
		}
		it.HasNFO = hasNFO != 0
		it.HasPoster = hasPoster != 0
		it.HasPending = hasPending != 0
		it.Genres = splitCSV(genresCSV)
		it.Actors = splitCSV(actorsCSV)
		if year.Valid {
			y := int(year.Int64)
			it.Year = &y
		}
		if strings.TrimSpace(root) != "" {
			it.PosterURL = posterURLFromRel(root, posterRel)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// getIndexItemByID 按条目 id 查询单条（海报 URL 用索引库 root 重建）。
func getIndexItemByID(db *sql.DB, root, id string) (*Item, error) {
	row := db.QueryRow(`
SELECT id, rel_dir, strm_name, title, year, media_type, status,
       has_nfo, has_poster, has_pending, tmdb_id, poster_rel, folder_name,
       file_count, ep_local, ep_tmdb, ep_scraped, tv_state, added_at, genres_csv, actors_csv
FROM items WHERE id = ?`, id)
	var it Item
	var year sql.NullInt64
	var hasNFO, hasPoster, hasPending int
	var posterRel, genresCSV, actorsCSV string
	if err := row.Scan(
		&it.ID, &it.RelDir, &it.StrmName, &it.Title, &year, &it.MediaType, &it.Status,
		&hasNFO, &hasPoster, &hasPending, &it.TMDBID, &posterRel, &it.FolderName,
		&it.FileCount, &it.EpLocal, &it.EpTMDB, &it.EpScraped, &it.TVState, &it.AddedAt, &genresCSV, &actorsCSV,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.Errorf(domain.CodeNotFound, "条目不存在")
		}
		return nil, err
	}
	it.HasNFO = hasNFO != 0
	it.HasPoster = hasPoster != 0
	it.HasPending = hasPending != 0
	it.Genres = splitCSV(genresCSV)
	it.Actors = splitCSV(actorsCSV)
	if year.Valid {
		y := int(year.Int64)
		it.Year = &y
	}
	if strings.TrimSpace(root) != "" {
		it.PosterURL = posterURLFromRel(root, posterRel)
	}
	return &it, nil
}


func readIndexStats(db *sql.DB) (ItemListStats, error) {
	var stats ItemListStats
	err := db.QueryRow(`
SELECT
  COUNT(*),
  SUM(CASE WHEN status = 'ok' THEN 1 ELSE 0 END),
  SUM(CASE WHEN status = 'miss' THEN 1 ELSE 0 END),
  SUM(CASE WHEN status = 'doubt' THEN 1 ELSE 0 END)
FROM items
`).Scan(&stats.Total, &stats.OK, &stats.Miss, &stats.Doubt)
	return stats, err
}

func (s *Service) listIndexItems(strmTaskID int64, query ItemListQuery) (ItemListResult, error) {
	db, err := openTaskIndexDB(s.indexPath(strmTaskID))
	if err != nil {
		return ItemListResult{}, err
	}
	defer db.Close()

	stats, err := readIndexStats(db)
	if err != nil {
		return ItemListResult{}, err
	}
	storedRoot, _ := readIndexMeta(db, "root")
	whereSQL, whereArgs := buildItemListWhere(query)
	countSQL := `SELECT COUNT(*) FROM items`
	if whereSQL != "" {
		countSQL += "\n" + whereSQL
	}
	var total int
	if err := db.QueryRow(countSQL, whereArgs...).Scan(&total); err != nil {
		return ItemListResult{}, err
	}
	querySQL := `
SELECT id, rel_dir, strm_name, title, year, media_type, status,
       has_nfo, has_poster, has_pending, tmdb_id, poster_rel, folder_name,
       file_count, ep_local, ep_tmdb, ep_scraped, tv_state, added_at, genres_csv, actors_csv
FROM items
`
	if whereSQL != "" {
		querySQL += whereSQL + "\n"
	}
	querySQL += itemListOrderBy(query.Sort) + "\nLIMIT ? OFFSET ?"
	args := append(append(make([]any, 0, len(whereArgs)+2), whereArgs...), query.Limit, query.Offset)
	rows, err := db.Query(querySQL, args...)
	if err != nil {
		return ItemListResult{}, err
	}
	defer rows.Close()
	items, err := scanIndexItems(rows, storedRoot)
	if err != nil {
		return ItemListResult{}, err
	}
	return ItemListResult{
		Items:   items,
		Total:   total,
		Offset:  query.Offset,
		Limit:   query.Limit,
		HasMore: query.Offset+len(items) < total,
		Stats:   stats,
	}, nil
}

func (s *Service) ensureIndexLocked(ctx context.Context, strmTaskID int64, root string) error {
	rootAbs := root
	if abs, err := filepath.Abs(root); err == nil {
		rootAbs = abs
	}
	if !s.indexFileExists(strmTaskID) {
		return s.rebuildIndexLocked(ctx, strmTaskID, root)
	}
	db, err := openTaskIndexDB(s.indexPath(strmTaskID))
	if err != nil {
		return s.rebuildIndexLocked(ctx, strmTaskID, root)
	}
	defer db.Close()
	if ver, ok := readIndexMeta(db, "schema"); !ok || ver != indexSchemaVersion {
		return s.rebuildIndexLocked(ctx, strmTaskID, root)
	}
	if stored, ok := readIndexMeta(db, "root"); ok && stored != "" {
		storedAbs := stored
		if abs, err := filepath.Abs(stored); err == nil {
			storedAbs = abs
		}
		if storedAbs != rootAbs {
			return s.rebuildIndexLocked(ctx, strmTaskID, root)
		}
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM items`).Scan(&n); err != nil || n == 0 {
		// 空索引常见于目录当时为空或重建中断；再扫一次对齐磁盘
		return s.rebuildIndexLocked(ctx, strmTaskID, root)
	}
	return nil
}
