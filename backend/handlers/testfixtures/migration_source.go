package testfixtures

// #2342: 迁移链 fresh-install 可重放——M1-via-shim（运行层幂等化）。
//
// 背景（#2341 实测）：`backend/database/migrations/` 的 217 个 up 文件在空库上
// 无法整链重放，也不是「AutoMigrate 先行」可重放的：
//   - 001_initial_schema 的 `CREATE TABLE users (...)` 无 IF NOT EXISTS，
//     AutoMigrate 已建表 → duplicate table；
//   - 20260630003_add_appeal_fields 先 `ALTER TABLE appeals`，
//     而 appeals 由更晚的 20260706002_add_appeals_table 创建 → relation does not exist。
//
// 裁定（#2342 D1/D2/D3）：**不修改历史迁移文件**（golang-migrate 只按版本+dirty
// 追踪、无内容校验和，改历史文件对已应用库零收益），而是在测试模板构建器内新增
// 一个内存幂等化迁移源：
//   CREATE TABLE            -> CREATE TABLE IF NOT EXISTS
//   CREATE [UNIQUE] INDEX   -> CREATE [UNIQUE] INDEX IF NOT EXISTS
//   ALTER TABLE t           -> ALTER TABLE IF EXISTS t
//   ALTER TABLE ... ADD COLUMN c -> ADD COLUMN IF NOT EXISTS c
// 不满足的语句按 replay 失败逐例补规则（D5），禁止静默吞失败。
// 生产 migrate 路径不引用本文件（只被 handlers 测试模板使用）。

import (
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing/fstest"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// MigrationsDirCandidates 是按优先级探测的迁移目录（go test 的 cwd 是包目录）。
var MigrationsDirCandidates = []string{
	filepath.Join("..", "database", "migrations"),
	filepath.Join("database", "migrations"),
}

// ResolveMigrationsDir 返回可用的迁移目录；都不可用时显式报错（不静默跳过）。
func ResolveMigrationsDir() (string, error) {
	if v := strings.TrimSpace(os.Getenv("MIGRATION_PATH")); v != "" {
		if st, err := os.Stat(v); err == nil && st.IsDir() {
			return v, nil
		}
		return "", fmt.Errorf("MIGRATION_PATH=%q is not a directory", v)
	}
	for _, cand := range MigrationsDirCandidates {
		if st, err := os.Stat(cand); err == nil && st.IsDir() {
			return cand, nil
		}
	}
	return "", fmt.Errorf("migrations dir not found (tried %s); set MIGRATION_PATH",
		strings.Join(MigrationsDirCandidates, ", "))
}

// ShimStats 记录幂等化变换的计数（写入模板 meta，便于追溯）。
type ShimStats struct {
	Files       int
	UpFiles     int
	CreateTable int
	CreateIndex int
	IndexGuard  int
	DMLGuard    int
	AlterTable  int
	AddColumn   int
	Statements  int
}

// Ops 返回被改写的语句总数。
func (s ShimStats) Ops() int {
	return s.CreateTable + s.CreateIndex + s.IndexGuard + s.DMLGuard + s.AlterTable + s.AddColumn
}

// String 用于日志与 meta。
func (s ShimStats) String() string {
	return fmt.Sprintf("files=%d up=%d statements=%d create_table=%d create_index=%d index_guard=%d dml_guard=%d alter_table=%d add_column=%d",
		s.Files, s.UpFiles, s.Statements, s.CreateTable, s.CreateIndex, s.IndexGuard, s.DMLGuard, s.AlterTable, s.AddColumn)
}

// MigrationSignature 返回迁移目录的稳定指纹与 up 文件数（模板过期判定用）。
// 指纹基于**幂等化后**的内容：shim 规则变化同样触发模板重建。
func MigrationSignature(dir string) (hash string, upFiles int, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", 0, fmt.Errorf("migrations signature: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	h := sha1.New()
	// 证据模式下原始文件重放，指纹必须与 shim 模式区分（避免互相判定为「未过期」）。
	if shimDisabled() {
		fmt.Fprint(h, "shim=off\n")
	}
	for _, name := range names {
		raw, rerr := os.ReadFile(filepath.Join(dir, name))
		if rerr != nil {
			return "", 0, fmt.Errorf("migrations signature: read %s: %w", name, rerr)
		}
		transformed, _ := shimSQL(string(raw))
		fmt.Fprintf(h, "%s\n%d\n%s\n", name, len(transformed), transformed)
		if strings.HasSuffix(name, ".up.sql") {
			upFiles++
		}
	}
	return hex.EncodeToString(h.Sum(nil)), upFiles, nil
}

// shimDisabled 是证据/诊断用的显式开关：TUNELOOP_TEMPLATE_MIGRATION_SHIM=off
// 时跳过幂等化变换，按原始文件重放（用于复现 #2341 的红：整链重放失败）。
// 默认（未设置或任何其他值）= 开启 shim，即模板构建的生产等价路径。
func shimDisabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("TUNELOOP_TEMPLATE_MIGRATION_SHIM")), "off")
}

// IdempotentMigrationSource 读取磁盘上的迁移文件、施加幂等化变换，
// 返回 golang-migrate 的内存 source driver（不改磁盘文件）。
func IdempotentMigrationSource(dir string) (source.Driver, ShimStats, error) {
	var stats ShimStats
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, stats, fmt.Errorf("migration source: read dir %s: %w", dir, err)
	}
	shimOff := shimDisabled()
	fsys := fstest.MapFS{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		raw, rerr := os.ReadFile(filepath.Join(dir, e.Name()))
		if rerr != nil {
			return nil, stats, fmt.Errorf("migration source: read %s: %w", e.Name(), rerr)
		}
		transformed, st := shimSQL(string(raw))
		stats.Files++
		stats.Statements += st.Statements
		stats.CreateTable += st.CreateTable
		stats.CreateIndex += st.CreateIndex
		stats.IndexGuard += st.IndexGuard
		stats.DMLGuard += st.DMLGuard
		stats.AlterTable += st.AlterTable
		stats.AddColumn += st.AddColumn
		if strings.HasSuffix(e.Name(), ".up.sql") {
			stats.UpFiles++
		}
		if shimOff {
			transformed = string(raw)
		}
		fsys[e.Name()] = &fstest.MapFile{Data: []byte(transformed)}
	}
	if stats.Files == 0 {
		return nil, stats, fmt.Errorf("migration source: no .sql files in %s", dir)
	}
	driver, err := iofs.New(fsys, ".")
	if err != nil {
		return nil, stats, fmt.Errorf("migration source: iofs: %w", err)
	}
	return driver, stats, nil
}

// RunShimmedMigrations 在独立连接上重放整条迁移链（幂等化后）。
// 使用独立 *sql.DB：golang-migrate 的 postgres driver 会独占一个连接并
// 在 Close 时关闭 db，不能借用 GORM 共享池（否则模板库残留连接导致克隆失败）。
func RunShimmedMigrations(dsn, dir string) (ShimStats, uint, error) {
	var stats ShimStats
	driver, stats, err := IdempotentMigrationSource(dir)
	if err != nil {
		return stats, 0, err
	}
	sqlDB, err := sql.Open("postgres", dsn)
	if err != nil {
		return stats, 0, fmt.Errorf("shimmed migrations: open: %w", err)
	}
	defer sqlDB.Close()
	pg, err := postgres.WithInstance(sqlDB, &postgres.Config{})
	if err != nil {
		return stats, 0, fmt.Errorf("shimmed migrations: postgres driver: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", driver, "postgres", pg)
	if err != nil {
		return stats, 0, fmt.Errorf("shimmed migrations: migrate instance: %w", err)
	}
	defer m.Close()
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return stats, 0, fmt.Errorf("shimmed migrations: up: %w", err)
	}
	version, _, verr := pg.Version()
	if verr != nil && verr != migrate.ErrNilVersion {
		return stats, 0, fmt.Errorf("shimmed migrations: version: %w", verr)
	}
	if version < 0 {
		version = 0
	}
	return stats, uint(version), nil
}

// ShimmedMigrationVersion 读取模板库当前的 schema 版本（模板 meta 断言用）。
func ShimmedMigrationVersion(db *sql.DB) (uint, error) {
	var version sql.NullInt64
	err := db.QueryRow(`SELECT version FROM schema_migrations LIMIT 1`).Scan(&version)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	if !version.Valid {
		return 0, nil
	}
	return uint(version.Int64), nil
}

// ------------------------------------------------------------------ shim core

type shimCounts struct {
	Statements  int
	CreateTable int
	CreateIndex int
	IndexGuard  int
	DMLGuard    int
	AlterTable  int
	AddColumn   int
}

// shimSQL 对整份迁移文件做语句级幂等化，返回新内容与改写计数。
func shimSQL(src string) (string, shimCounts) {
	var counts shimCounts
	stmts := splitSQLStatements(src)
	out := make([]string, 0, len(stmts))
	for _, stmt := range stmts {
		if strings.TrimSpace(stmt) == "" {
			continue
		}
		counts.Statements++
		transformed, changed := shimStatement(stmt)
		if changed != "" {
			switch changed {
			case "create_table":
				counts.CreateTable++
			case "create_index":
				counts.CreateIndex++
			case "index_guard":
				counts.IndexGuard++
			case "alter_table":
				counts.AlterTable++
			case "add_column":
				counts.AddColumn++
			case "dml_guard":
				counts.DMLGuard++
			}
		}
		out = append(out, transformed)
	}
	if len(out) == 0 {
		return "", counts
	}
	return strings.Join(out, ";\n") + ";\n", counts
}

// shimStatement 对单条语句施加幂等化规则，返回改写后的语句与命中规则名。
func shimStatement(stmt string) (string, string) {
	// 跳过前导空白与注释（-- ... / /* ... */），按真实语句首关键字分类；
	// 前导注释原样保留，保证迁移文件可读性不变。
	codeAt := skipLeadingTrivia(stmt)
	prefix := stmt[:codeAt]
	body := stmt[codeAt:]
	upper := strings.ToUpper(body)
	hit := ""
	isIndex := false

	switch {
	case strings.HasPrefix(upper, "CREATE TABLE "):
		body, hit = injectIfNotExists(body, len("CREATE TABLE "), "create_table")
	case strings.HasPrefix(upper, "CREATE UNLOGGED TABLE "):
		body, hit = injectIfNotExists(body, len("CREATE UNLOGGED TABLE "), "create_table")
	case strings.HasPrefix(upper, "CREATE TEMP TABLE "):
		body, hit = injectIfNotExists(body, len("CREATE TEMP TABLE "), "create_table")
	case strings.HasPrefix(upper, "CREATE TEMPORARY TABLE "):
		body, hit = injectIfNotExists(body, len("CREATE TEMPORARY TABLE "), "create_table")
	case strings.HasPrefix(upper, "CREATE UNIQUE INDEX "), strings.HasPrefix(upper, "CREATE INDEX "):
		n := len("CREATE INDEX ")
		if strings.HasPrefix(upper, "CREATE UNIQUE INDEX ") {
			n = len("CREATE UNIQUE INDEX ")
		}
		body, hit = injectIfNotExists(body, n, "create_index")
		isIndex = true
	case strings.HasPrefix(upper, "ALTER TABLE "):
		var ok bool
		body, ok = injectIfExists(body, len("ALTER TABLE "))
		if ok {
			hit = "alter_table"
		}
		var n int
		body, n = injectAddColumn(body)
		if n > 0 {
			hit = "add_column"
		}
	}

	if isIndex {
		if guarded, ok := guardMissingTable(body); ok {
			return prefix + guarded, "index_guard"
		}
		if hit == "" {
			hit = "create_index"
		}
	}
	if isDMLStatement(upper) {
		if guarded, ok := guardDMLMissingTables(body); ok {
			return prefix + guarded, "dml_guard"
		}
	}
	return prefix + body, hit
}

// skipLeadingTrivia 返回跳过前导空白/注释后的偏移。
func skipLeadingTrivia(s string) int {
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			i++
		case c == '-' && i+1 < len(s) && s[i+1] == '-':
			for i < len(s) && s[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(s) && s[i+1] == '*':
			depth := 0
			for i < len(s) {
				if i+1 < len(s) && s[i] == '/' && s[i+1] == '*' {
					depth++
					i += 2
					continue
				}
				if i+1 < len(s) && s[i] == '*' && s[i+1] == '/' {
					depth--
					i += 2
					if depth == 0 {
						break
					}
					continue
				}
				i++
			}
		default:
			return i
		}
	}
	return i
}

// guardMissingTable 把 CREATE INDEX 包进 DO 块，仅当目标表已存在时执行。
// 背景：#2342 链内 20260630003 先给 appeals 建索引，而 appeals 由更晚的
// 20260706002 创建；Postgres 的 CREATE INDEX 没有「表不存在则跳过」语法。
// 后续迁移会再次 CREATE INDEX IF NOT EXISTS 同一批索引，最终 schema 不变。
func guardMissingTable(stmt string) (string, bool) {
	upper := strings.ToUpper(stmt)
	onIdx := strings.Index(upper, " ON ")
	if onIdx < 0 {
		return stmt, false
	}
	rest := strings.TrimLeft(stmt[onIdx+len(" ON "):], " \t\r\n")
	end := strings.IndexAny(rest, " (\t\r\n")
	if end <= 0 {
		return stmt, false
	}
	table := rest[:end]
	outer := uniqueDollarTag(stmt+table, "shim")
	inner := uniqueDollarTag(stmt+table, "idx")
	return "DO " + outer + "\nBEGIN\n  IF to_regclass('" + strings.ReplaceAll(table, "'", "''") +
		"') IS NOT NULL THEN\n    EXECUTE " + inner + stmt + inner +
		";\n  END IF;\nEND\n" + outer, true
}

// uniqueDollarTag 返回一个不会与内容冲突的 $tag$ 定界符。
func uniqueDollarTag(content, base string) string {
	for i := 0; ; i++ {
		tag := "$" + base
		if i > 0 {
			tag += fmt.Sprintf("%d", i)
		}
		tag += "$"
		if !strings.Contains(content, tag) {
			return tag
		}
	}
}

// isDMLStatement 判断是否是需要「依赖表存在」保护的数据语句。
func isDMLStatement(upper string) bool {
	for _, p := range []string{"INSERT INTO ", "UPDATE ", "DELETE FROM ", "WITH "} {
		if strings.HasPrefix(upper, p) {
			return true
		}
	}
	return false
}

// dmlTableRefs 粗略抽取 DML 语句引用的表名（FROM/JOIN/UPDATE/INTO 之后的标识符）。
// 只用于 to_regclass 存在性守卫；解析不出的标记（函数调用、子查询）会被跳过。
func dmlTableRefs(stmt string) []string {
	keywords := []string{" FROM ", " JOIN ", " UPDATE ", " INTO "}
	seen := map[string]bool{}
	var out []string
	upper := strings.ToUpper(stmt)
	for _, kw := range keywords {
		for off := 0; ; {
			idx := strings.Index(upper[off:], kw)
			if idx < 0 {
				break
			}
			start := off + idx + len(kw)
			rest := strings.TrimLeft(stmt[start:], " \t\r\n")
			// 跳过函数调用/子查询/关键字
			if rest == "" || strings.HasPrefix(rest, "(") {
				off = start
				continue
			}
			end := strings.IndexAny(rest, " ()\t\r\n,")
			if end <= 0 {
				off = start
				continue
			}
			tok := rest[:end]
			up := strings.ToUpper(tok)
			if up == "SELECT" || up == "LATERAL" || up == "ONLY" || up == "VALUES" || up == "SET" {
				off = start
				continue
			}
			if strings.ContainsAny(tok, "-+*/") {
				off = start
				continue
			}
			if !seen[tok] {
				seen[tok] = true
				out = append(out, tok)
			}
			off = start
		}
	}
	return out
}

// guardDMLMissingTables 把 DML 包进 DO 块，仅当引用的表都已存在时执行。
// 背景：#2342 链内存在「先回填、后建表」的语序问题（如 appeals 回填早于
// 20260706002 建表）。空库重放时这些回填本就无数据可搬，跳过等价；
// 表存在时语句照常执行（不改变数据语义）。
func guardDMLMissingTables(stmt string) (string, bool) {
	tables := dmlTableRefs(stmt)
	if len(tables) == 0 {
		return stmt, false
	}
	conds := make([]string, 0, len(tables))
	for _, t := range tables {
		conds = append(conds, "to_regclass('"+strings.ReplaceAll(t, "'", "''")+"') IS NOT NULL")
	}
	outer := uniqueDollarTag(stmt+strings.Join(tables, ""), "shim")
	inner := uniqueDollarTag(stmt+strings.Join(tables, ""), "dml")
	return "DO " + outer + "\nBEGIN\n  IF " + strings.Join(conds, " AND ") +
		" THEN\n    EXECUTE " + inner + stmt + inner +
		";\n  END IF;\nEND\n" + outer, true
}

// injectIfNotExists 在关键字后插入 "IF NOT EXISTS "（已存在则原样返回）。
func injectIfNotExists(stmt string, keywordLen int, rule string) (string, string) {
	rest := stmt[keywordLen:]
	if hasWordPrefix(rest, "IF NOT EXISTS") {
		return stmt, ""
	}
	return stmt[:keywordLen] + "IF NOT EXISTS " + rest, rule
}

// injectIfExists 在 "ALTER TABLE " 后插入 "IF EXISTS "（已存在则原样返回）。
func injectIfExists(stmt string, keywordLen int) (string, bool) {
	rest := stmt[keywordLen:]
	if hasWordPrefix(rest, "IF EXISTS") {
		return stmt, false
	}
	return stmt[:keywordLen] + "IF EXISTS " + rest, true
}

// injectAddColumn 把 ALTER TABLE ... ADD COLUMN 变为 ADD COLUMN IF NOT EXISTS。
func injectAddColumn(stmt string) (string, int) {
	const marker = "ADD COLUMN "
	count := 0
	var b strings.Builder
	rest := stmt
	for {
		idx := indexFold(rest, marker)
		if idx < 0 {
			b.WriteString(rest)
			break
		}
		b.WriteString(rest[:idx])
		b.WriteString(rest[idx : idx+len(marker)])
		after := rest[idx+len(marker):]
		if hasWordPrefix(after, "IF NOT EXISTS") {
			rest = after
			continue
		}
		b.WriteString("IF NOT EXISTS ")
		count++
		rest = after
	}
	return b.String(), count
}

func hasWordPrefix(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	return strings.EqualFold(s[:len(prefix)], prefix)
}

func indexFold(s, sub string) int {
	return strings.Index(strings.ToUpper(s), strings.ToUpper(sub))
}

// splitSQLStatements 按顶层 ';' 切分语句，保留字符串/引用/注释/$tag$ 内部的分号。
func splitSQLStatements(src string) []string {
	var out []string
	var b strings.Builder
	i, n := 0, len(src)
	for i < n {
		c := src[i]
		switch {
		case c == '\'':
			b.WriteByte(c)
			i++
			for i < n {
				if src[i] == '\'' {
					if i+1 < n && src[i+1] == '\'' {
						b.WriteString("''")
						i += 2
						continue
					}
					b.WriteByte('\'')
					i++
					break
				}
				b.WriteByte(src[i])
				i++
			}
		case c == '"':
			b.WriteByte(c)
			i++
			for i < n {
				if src[i] == '"' {
					if i+1 < n && src[i+1] == '"' {
						b.WriteString(`""`)
						i += 2
						continue
					}
					b.WriteByte('"')
					i++
					break
				}
				b.WriteByte(src[i])
				i++
			}
		case c == '-' && i+1 < n && src[i+1] == '-':
			for i < n && src[i] != '\n' {
				b.WriteByte(src[i])
				i++
			}
		case c == '/' && i+1 < n && src[i+1] == '*':
			depth := 0
			for i < n {
				if i+1 < n && src[i] == '/' && src[i+1] == '*' {
					depth++
					b.WriteString("/*")
					i += 2
					continue
				}
				if i+1 < n && src[i] == '*' && src[i+1] == '/' {
					depth--
					b.WriteString("*/")
					i += 2
					if depth == 0 {
						break
					}
					continue
				}
				b.WriteByte(src[i])
				i++
			}
		case c == '$':
			if tag, end, ok := parseDollarTag(src, i); ok {
				b.WriteString(tag)
				i = end
				idx := strings.Index(src[i:], tag)
				if idx < 0 {
					b.WriteString(src[i:])
					i = n
					continue
				}
				b.WriteString(src[i : i+idx])
				b.WriteString(tag)
				i += idx + len(tag)
				continue
			}
			b.WriteByte(c)
			i++
		case c == ';':
			out = append(out, b.String())
			b.Reset()
			i++
		default:
			b.WriteByte(c)
			i++
		}
	}
	if strings.TrimSpace(b.String()) != "" {
		out = append(out, b.String())
	}
	return out
}

// parseDollarTag 解析 $tag$ 起始标记（tag 可为空或 [A-Za-z0-9_]）。
func parseDollarTag(src string, i int) (tag string, end int, ok bool) {
	if i >= len(src) || src[i] != '$' {
		return "", i, false
	}
	j := i + 1
	for j < len(src) {
		c := src[j]
		if c == '$' {
			return src[i : j+1], j + 1, true
		}
		if !(c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return "", i, false
		}
		j++
	}
	return "", i, false
}
