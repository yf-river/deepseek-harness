// 执行历史与机器人状态的持久化。
//
// 为什么用 SQLite 而不是目录扫描：原来每次请求历史都要遍历所有批次目录、
// 逐个读取 final-state.json，条目越多越慢，且没有任何清理机制。
// 改成单文件数据库后，列表查询走索引，写入是单条 INSERT，历史量增长不影响响应。
//
// 驱动选 modernc.org/sqlite：纯 Go 实现，不需要 cgo，
// 部署机用 CGO_ENABLED=0 构建也能正常工作。

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// store 封装运行记录数据库。
type store struct {
	db *sql.DB
}

// openStore 打开（必要时创建）数据库文件并建表。
func openStore(path string) (*store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("创建数据目录失败: %w", err)
	}
	// _pragma 参数确保并发写入时有 WAL 与忙等待，避免 "database is locked"。
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	s := &store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// migrate 建表。用 IF NOT EXISTS，重复启动无副作用。
func (s *store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS batch (
    id           TEXT PRIMARY KEY,
    trigger_kind TEXT NOT NULL,
    verdict      TEXT NOT NULL,
    started_at   TEXT NOT NULL,
    ended_at     TEXT NOT NULL,
    subject      TEXT NOT NULL,
    mail_status  TEXT NOT NULL,
    pass_count   INTEGER NOT NULL,
    fail_count   INTEGER NOT NULL,
    incomplete   INTEGER NOT NULL,
    last_error   TEXT NOT NULL,
    suites_json  TEXT NOT NULL,
    detail_json  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_batch_started ON batch(started_at DESC);

CREATE TABLE IF NOT EXISTS robot_state (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    payload    TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
`
	_, err := s.db.Exec(schema)
	return err
}

func (s *store) close() error { return s.db.Close() }

// saveBatch 写入或覆盖一条批次记录。批次号唯一，重复写入按覆盖处理，
// 这样执行中途更新状态也不会产生重复行。
func (s *store) saveBatch(entry historyEntry) error {
	suites, err := json.Marshal(entry.Suites)
	if err != nil {
		return err
	}
	detail, err := json.Marshal(entry.Detail)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
INSERT INTO batch (id, trigger_kind, verdict, started_at, ended_at, subject, mail_status,
                   pass_count, fail_count, incomplete, last_error, suites_json, detail_json)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
    trigger_kind=excluded.trigger_kind, verdict=excluded.verdict,
    started_at=excluded.started_at, ended_at=excluded.ended_at,
    subject=excluded.subject, mail_status=excluded.mail_status,
    pass_count=excluded.pass_count, fail_count=excluded.fail_count,
    incomplete=excluded.incomplete, last_error=excluded.last_error,
    suites_json=excluded.suites_json, detail_json=excluded.detail_json`,
		entry.Dir, entry.Trigger, entry.Verdict, entry.StartedAt, entry.EndedAt,
		entry.Subject, entry.MailState, entry.PassCount, entry.FailCount,
		entry.Incomplete, entry.LastError, string(suites), string(detail))
	return err
}

// loadPage 返回一页历史（按开始时间倒序）与总条数。
// withDetail 为 false 时不加载 detail_json，列表页用不到逐套件全量数据。
func (s *store) loadPage(page, pageSize int, withDetail bool) ([]historyEntry, int, error) {
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM batch`).Scan(&total); err != nil {
		return nil, 0, err
	}
	page, pageSize = normalizePage(page, pageSize)
	offset := (page - 1) * pageSize
	if offset >= total {
		// 返回空切片而不是 nil：前端拿到 null 会当成异常，
		// 空列表和"没有记录"在语义上是同一件事。
		return []historyEntry{}, total, nil
	}

	rows, err := s.db.Query(`
SELECT id, trigger_kind, verdict, started_at, ended_at, subject, mail_status,
       pass_count, fail_count, incomplete, last_error, suites_json, detail_json
FROM batch ORDER BY started_at DESC LIMIT ? OFFSET ?`, pageSize, offset)
	if err != nil {
		return nil, total, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]historyEntry, 0, pageSize)
	for rows.Next() {
		var (
			entry                historyEntry
			suitesRaw, detailRaw string
		)
		if err := rows.Scan(&entry.Dir, &entry.Trigger, &entry.Verdict, &entry.StartedAt,
			&entry.EndedAt, &entry.Subject, &entry.MailState, &entry.PassCount,
			&entry.FailCount, &entry.Incomplete, &entry.LastError, &suitesRaw, &detailRaw); err != nil {
			return nil, total, err
		}
		_ = json.Unmarshal([]byte(suitesRaw), &entry.Suites)
		if withDetail {
			_ = json.Unmarshal([]byte(detailRaw), &entry.Detail)
		}
		items = append(items, entry)
	}
	return items, total, rows.Err()
}

// hasBatchOn 判断某一天（按给定时区）是否已有批次，用于启动补跑判断。
func (s *store) hasBatchOn(day time.Time, location *time.Location) (bool, error) {
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, location)
	end := start.AddDate(0, 0, 1)
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM batch WHERE started_at >= ? AND started_at < ?`,
		start.Format(time.RFC3339), end.Format(time.RFC3339)).Scan(&count)
	return count > 0, err
}

// saveRobotState 持久化机器人状态，重启后据此恢复上次结果。
func (s *store) saveRobotState(state robotState) error {
	payload, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
INSERT INTO robot_state (id, payload, updated_at) VALUES (1, ?, ?)
ON CONFLICT(id) DO UPDATE SET payload=excluded.payload, updated_at=excluded.updated_at`,
		string(payload), time.Now().Format(time.RFC3339))
	return err
}

// loadRobotState 读取上次保存的机器人状态。没记录时返回 false。
func (s *store) loadRobotState() (robotState, bool) {
	var payload string
	if err := s.db.QueryRow(`SELECT payload FROM robot_state WHERE id = 1`).Scan(&payload); err != nil {
		return robotState{}, false
	}
	var state robotState
	if err := json.Unmarshal([]byte(payload), &state); err != nil {
		return robotState{}, false
	}
	return state, true
}

// pruneBefore 删除早于给定时间的批次，返回删除条数。
// 保留量可控，避免数据库无限增长。
func (s *store) pruneBefore(before time.Time) (int64, error) {
	result, err := s.db.Exec(`DELETE FROM batch WHERE started_at < ?`, before.Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
