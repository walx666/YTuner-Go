package ytuner

// Radio-browser.info local SQLite DB cache (src/radiobrowserdb.pas).
//
// Uses the pure Go driver modernc.org/sqlite, so no sqlite3.dll / libsqlite3
// is needed (the DBLibFile option of ytuner.ini is ignored).

import (
	"bytes"
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	sqlite "modernc.org/sqlite"
)

//go:embed sql/*.sql
var sqlFS embed.FS

const (
	RBDBFileName    = "rb.db"
	RBTmpDBFileName = "rb-tmp.db"

	RBJSONCountries = "countries"
	RBJSONLanguages = "languages"
	RBJSONCodecs    = "codecs"
	RBJSONTags      = "tags"
	RBJSONStations  = "stations"

	MsgRBDBDB         = "database"
	MsgRBDBLibrary    = "library"
	MsgRBDBError      = "error"
	MsgRBDBConnection = "connection"
	MsgRBDBDisconnect = "disconnect"
	MsgRBDBCreate     = "create"
	MsgRBDBDelete     = "delete"
	MsgRBDBRestore    = "restore"
	MsgRBDBBackup     = "backup"
	MsgRBDBDBInUse    = "(DB in use?)"
	MsgRBDBLocal      = "local"
	MsgRBDBWait       = "This may take a while.."
	MsgRBDBClosing    = "closing"
	MsgRBDBChecking   = "checking"
	MsgRBDBPreparing  = "preparing"
	MsgRBDBCreating   = "creating"
	MsgRBDBRebuilding = "rebuilding"
	MsgRBDBCleaning   = "cleaning"
	MsgRBDBFilling    = "filling out"
	MsgRBDBExecution  = "execution"
	MsgRBDBScript     = "script"
	MsgRBDBQuery      = "query"
	MsgRBDBView       = "view"
	MsgRBDBTable      = "table"
	MsgRBDBTables     = "tables"
	MsgRBDBIndexes    = "indexes"
	MsgRBDBReady      = "is ready"
	MsgRBDBCannot     = "Can not"

	RBDBSQLVersion        = "RBDB_SQL_VERSION"
	RBDBSQLListViews      = "RBDB_SQL_LIST_VIEWS"
	RBDBSQLCreateViewAVR  = "RBDB_SQL_CREATE_VIEW_AVR"
	RBDBSQLDropViewAVR    = "RBDB_SQL_DROP_VIEW_AVR"
	RBDBSQLSelectCategory = "RBDB_SQL_SELECT_CATEGORY"
	RBDBSQLSelectStation  = "RBDB_SQL_SELECT_STATION"
	RBDBSQLSelectStations = "RBDB_SQL_SELECT_STATIONS"
	RBDBSQLSelectRoot     = "RBDB_SQL_SELECT_ROOT"

	RBDBMarkerAt       = "@@@"
	RBDBMarkerJSON     = "JSONCONTENTS"
	RBDBMarkerCategory = "Category"
	RBDBMarkerID       = "ID"
	RBDBMarkerAVR      = "AVR"
	RBDBMarkerOrder    = "Order"
	RBDBMarkerDirOrder = "DirOrder"
	RBDBMarkerStart    = "Start"
	RBDBMarkerOffset   = "Offset"
	RBDBMarkerBegin    = "--Begin "
	RBDBMarkerEnd      = "--End "
	RBDBNextStep       = "--Next Step--"

	// rbdbJSONChunk is the approximate size of one JSON chunk passed to json_each().
	rbdbJSONChunk = 4 << 20
)

// RBDBFieldsCategories mirrors RBDB_FIELDS_CATEGORIES.
var RBDBFieldsCategories = [5]string{"tag", "country", "language", "tc", "name"}

var (
	dbMu                 sync.RWMutex
	dbMain               *sql.DB
	rbdbCreationDateTime time.Time
	dbPrepareMu          sync.Mutex
)

func mainDB() *sql.DB {
	dbMu.RLock()
	defer dbMu.RUnlock()
	return dbMain
}

// DBRBGetResourceSQLScript returns an embedded SQL script.
func DBRBGetResourceSQLScript(name string) string {
	b, err := sqlFS.ReadFile("sql/" + name + ".sql")
	if err != nil {
		Logging(LtError, "SQL resource not found: "+name)
		return ""
	}
	return string(b)
}

// openSQLite opens a file DB or (memory=true) a private in-memory DB.
// A single connection is used, like the Pascal TSQLite3Connection.
func openSQLite(fileName string, memory bool) (*sql.DB, error) {
	dsn := "file:" + strings.ReplaceAll(fileName, "\\", "/")
	if memory {
		dsn = ":memory:"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(0)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// CheckSQLite3LibVer mirrors CheckSQLite3LibVer.
func CheckSQLite3LibVer() bool {
	db, err := openSQLite("", true)
	if err != nil {
		Logging(LtError, JoinMsg("CheckSQLite3LibVer:", MsgRBDBDB, MsgRBDBLibrary, MsgRBDBChecking, MsgRBDBError, " ("+err.Error()+")"))
		return false
	}
	defer db.Close()
	var ver string
	if err := db.QueryRow(DBRBGetResourceSQLScript(RBDBSQLVersion)).Scan(&ver); err != nil {
		Logging(LtError, JoinMsg("CheckSQLite3LibVer:", MsgRBDBDB, MsgRBDBLibrary, MsgRBDBChecking, MsgRBDBError, " ("+err.Error()+")"))
		return false
	}
	Logging(LtDebug, JoinMsg(MsgRBDBDB, MsgRBDBLibrary, ":", ver, "(modernc.org/sqlite, pure Go)"))
	p := strings.Split(ver, ".")
	for len(p) < 3 {
		p = append(p, "0")
	}
	x, _ := strconv.Atoi(p[0])
	y, _ := strconv.Atoi(p[1])
	z, _ := strconv.Atoi(p[2])
	ok := x*1000000+y*1000+z >= SQLiteVerXMin*1000000+SQLiteVerYMin*1000+SQLiteVerZMin
	if !ok {
		Logging(LtError, JoinMsg(MsgObsolete, MsgRBDBDB, MsgRBDBLibrary, MsgVersion, ":", ver, MsgRequired, ">=",
			fmt.Sprintf("%d.%d.%d", SQLiteVerXMin, SQLiteVerYMin, SQLiteVerZMin)))
	}
	return ok
}

// DBRBScriptExec mirrors DBRBScriptExec.
func DBRBScriptExec(db *sql.DB, script string) bool {
	if _, err := db.Exec(script); err != nil {
		Logging(LtError, JoinMsg("DBRBScriptExec:", MsgRBDBScript, MsgRBDBExecution, MsgRBDBError, " : ", firstLine(script), " ("+err.Error()+")"))
		return false
	}
	return true
}

func firstLine(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return s[:i] + " ..."
	}
	return s
}

// jsonArrayChunks splits a JSON array into JSON arrays of about chunkSize bytes
// (element boundaries are respected).
func jsonArrayChunks(data []byte, chunkSize int) ([]string, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '[' {
		return nil, errors.New("JSON array expected")
	}
	var chunks []string
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			chunks = append(chunks, "["+b.String()+"]")
			b.Reset()
		}
	}
	for dec.More() {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.Write(raw)
		if b.Len() >= chunkSize {
			flush()
		}
	}
	flush()
	return chunks, nil
}

// DBRBScriptExecWithJSON mirrors DBRBScriptExecWithJSON: executes the
// "--Next Step--" separated script parts; parts with @@@JSONCONTENTS are run
// once per JSON chunk with the chunk bound as parameter.
func DBRBScriptExecWithJSON(db *sql.DB, scripts, apiURL string) bool {
	data := RadiobrowserAPIRequest(apiURL)
	if len(data) <= 3 || string(data) == "[]" {
		return false
	}
	chunks, err := jsonArrayChunks(data, rbdbJSONChunk)
	if err != nil {
		Logging(LtError, JoinMsg("DBRBScriptExecWithJSON:", MsgRBDBError, apiURL, "("+err.Error()+")"))
		return false
	}
	data = nil
	for _, part := range strings.Split(scripts, RBDBNextStep) {
		if !strings.Contains(part, RBDBMarkerAt+RBDBMarkerJSON) {
			if strings.TrimSpace(stripSQLComments(part)) == "" {
				continue
			}
			if _, err := db.Exec(part); err != nil {
				Logging(LtError, JoinMsg("DBRBScriptExecWithJSON:", MsgRBDBScript, MsgRBDBExecution, MsgRBDBError, " : ", firstLine(part), " ("+err.Error()+")"))
				return false
			}
			continue
		}
		stmt := strings.Replace(part, RBDBMarkerAt+RBDBMarkerJSON, "?", 1)
		for _, ch := range chunks {
			if _, err := db.Exec(stmt, ch); err != nil {
				Logging(LtError, JoinMsg("DBRBScriptExecWithJSON:", MsgRBDBScript, MsgRBDBExecution, MsgRBDBError, " : ", firstLine(part), " ("+err.Error()+")"))
				return false
			}
		}
	}
	return true
}

func stripSQLComments(s string) string {
	var b strings.Builder
	for _, l := range strings.Split(s, "\n") {
		if i := strings.Index(l, "--"); i >= 0 {
			l = l[:i]
		}
		b.WriteString(l + "\n")
	}
	return b.String()
}

// DBRBValid mirrors DBRBValid.
func DBRBValid() bool {
	switch RBCacheType {
	case CatDB, CatPermMemDB:
		age, ok := fileAge(pathJoin(DBPath, RBDBFileName))
		return ok && (RBCacheTTL <= 0 || !expiredAfterHours(age, RBCacheTTL))
	case CatMemDB:
		return mainDB() != nil && (RBCacheTTL <= 0 || !expiredAfterHours(rbdbCreationDateTime, RBCacheTTL))
	}
	return false
}

// fillDB creates tables and loads all Radio-browser data into db.
func fillDB(db *sql.DB) bool {
	Logging(LtInfo, JoinMsg(MsgRBDBPreparing, MsgRBDBLocal, MsgRBDBDB, MsgRBDBWait))
	Logging(LtDebug, JoinMsg(MsgRBDBCreating, MsgRBDBTables))
	if !DBRBScriptExec(db, DBRBGetResourceSQLScript("RBDB_SQL_CREATE_TABLES")) {
		Logging(LtError, JoinMsg(MsgRBDBError, MsgRBDBCreating, MsgRBDBTables))
		return false
	}
	for _, step := range []struct{ res, api string }{
		{"RBDB_SQL_INSERT_COUNTRIES", RBJSONCountries},
		{"RBDB_SQL_INSERT_LANGUAGES", RBJSONLanguages},
		{"RBDB_SQL_INSERT_CODECS", RBJSONCodecs},
		{"RBDB_SQL_INSERT_TAGS", RBJSONTags},
	} {
		Logging(LtDebug, JoinMsg(MsgRBDBFilling, step.api, MsgRBDBTable))
		if !DBRBScriptExecWithJSON(db, DBRBGetResourceSQLScript(step.res), step.api) {
			Logging(LtError, JoinMsg(MsgRBDBError, MsgRBDBFilling, step.api, MsgRBDBTable))
			return false
		}
	}
	Logging(LtDebug, JoinMsg(MsgRBDBCreating, MsgRBDBIndexes))
	if !DBRBScriptExec(db, DBRBGetResourceSQLScript("RBDB_SQL_CREATE_IDX")) {
		Logging(LtError, JoinMsg(MsgRBDBError, MsgRBDBCreating, MsgRBDBIndexes))
		return false
	}
	Logging(LtDebug, JoinMsg(MsgRBDBFilling, RBJSONStations, MsgRBDBTable))
	if !DBRBScriptExecWithJSON(db, DBRBGetResourceSQLScript("RBDB_SQL_INSERT_STATIONS"), RBJSONStations) {
		Logging(LtError, JoinMsg(MsgRBDBError, MsgRBDBFilling, RBJSONStations, MsgRBDBTable))
		return false
	}
	Logging(LtDebug, JoinMsg(MsgRBDBCleaning, MsgRBDBTables))
	if !DBRBScriptExec(db, DBRBGetResourceSQLScript("RBDB_SQL_FIX_DATA")) {
		Logging(LtError, JoinMsg(MsgRBDBError, MsgRBDBCleaning, MsgRBDBTables))
		return false
	}
	Logging(LtDebug, JoinMsg(MsgRBDBRebuilding, MsgRBDBIndexes))
	if !DBRBScriptExec(db, DBRBGetResourceSQLScript("RBDB_SQL_CREATE_REIDX")) {
		Logging(LtError, JoinMsg(MsgRBDBError, MsgRBDBRebuilding, MsgRBDBIndexes))
		return false
	}
	return true
}

type sqliteBackuper interface {
	NewBackup(dstURI string) (*sqlite.Backup, error)
	NewRestore(srcURI string) (*sqlite.Backup, error)
}

// copyDB runs an online backup (restore=false: db -> file, restore=true: file -> db).
func copyDB(db *sql.DB, fileName string, restore bool) error {
	conn, err := db.Conn(context.Background())
	if err != nil {
		return err
	}
	defer conn.Close()
	uri := "file:" + strings.ReplaceAll(fileName, "\\", "/")
	return conn.Raw(func(dc any) error {
		b, ok := dc.(sqliteBackuper)
		if !ok {
			return errors.New("driver does not support backup")
		}
		var bk *sqlite.Backup
		if restore {
			bk, err = b.NewRestore(uri)
		} else {
			bk, err = b.NewBackup(uri)
		}
		if err != nil {
			return err
		}
		for {
			more, err := bk.Step(-1)
			if err != nil {
				bk.Finish()
				return err
			}
			if !more {
				break
			}
		}
		return bk.Finish()
	})
}

func swapMainDB(db *sql.DB, created time.Time) {
	dbMu.Lock()
	old := dbMain
	dbMain = db
	rbdbCreationDateTime = created
	dbMu.Unlock()
	if old != nil && old != db {
		old.Close()
	}
}

// DBRBPrepare mirrors DBRBPrepare: opens a valid existing DB or (re)builds it.
func DBRBPrepare() bool {
	dbPrepareMu.Lock()
	defer dbPrepareMu.Unlock()
	Logging(LtInfo, JoinMsg(MsgRBDBChecking, MsgRBDBLocal, MsgRBDBDB))
	dbFile := pathJoin(DBPath, RBDBFileName)
	ok := dbPrepare(dbFile)
	if ok {
		Logging(LtInfo, JoinMsg(MsgRBDBLocal, MsgRBDBDB, MsgRBDBReady))
	}
	return ok
}

func dbPrepare(dbFile string) bool {
	// 1. Reuse an existing, still valid DB file (only on first start).
	if mainDB() == nil && (RBCacheType == CatDB || RBCacheType == CatPermMemDB) {
		if age, ok := fileAge(dbFile); ok && (RBCacheTTL == 0 || !expiredAfterHours(age, RBCacheTTL)) {
			switch RBCacheType {
			case CatDB:
				db, err := openSQLite(dbFile, false)
				if err != nil {
					Logging(LtError, JoinMsg("DBRBPrepare:", MsgRBDBError, MsgRBDBConnection, " ("+err.Error()+")"))
					return false
				}
				swapMainDB(db, age)
				return true
			case CatPermMemDB:
				db, err := openSQLite("", true)
				if err == nil {
					err = copyDB(db, dbFile, true)
				}
				if err != nil {
					if db != nil {
						db.Close()
					}
					Logging(LtError, JoinMsg("DBRBPrepare:", MsgRBDBCannot, MsgRBDBRestore, MsgRBDBDB, " ("+err.Error()+")"))
					return false
				}
				swapMainDB(db, age)
				return true
			}
		}
	}

	// 2. Build a new DB (only if the current one is older than the TTL).
	if !rbdbCreationDateTime.IsZero() && RBCacheTTL > 0 && !expiredAfterHours(rbdbCreationDateTime, RBCacheTTL) {
		return false
	}
	var db *sql.DB
	var err error
	tmpFile := pathJoin(DBPath, RBTmpDBFileName)
	if RBCacheType == CatDB {
		if fileExists(tmpFile) {
			if err := os.Remove(tmpFile); err != nil {
				Logging(LtError, JoinMsg("DBRBPrepare:", MsgRBDBError, MsgRBDBDelete, tmpFile, MsgRBDBDBInUse))
				return false
			}
		}
		db, err = openSQLite(tmpFile, false)
	} else {
		db, err = openSQLite("", true)
	}
	if err != nil {
		Logging(LtError, JoinMsg("DBRBPrepare:", MsgRBDBError, MsgRBDBConnection, " ("+err.Error()+")"))
		return false
	}
	if !fillDB(db) {
		db.Close()
		if RBCacheType == CatDB {
			_ = os.Remove(tmpFile)
		}
		return false
	}

	switch RBCacheType {
	case CatDB:
		db.Close()
		// Close the old main DB before replacing the file (Windows locks open files).
		swapMainDB(nil, time.Time{})
		if fileExists(dbFile) {
			if err := os.Remove(dbFile); err != nil {
				Logging(LtError, JoinMsg("DBRBPrepare:", MsgRBDBError, MsgRBDBDelete, RBDBFileName, MsgRBDBDBInUse))
				return false
			}
		}
		if err := os.Rename(tmpFile, dbFile); err != nil {
			Logging(LtError, JoinMsg("DBRBPrepare:", MsgRBDBError, "rename", RBTmpDBFileName, "("+err.Error()+")"))
			return false
		}
		ndb, err := openSQLite(dbFile, false)
		if err != nil {
			Logging(LtError, JoinMsg("DBRBPrepare:", MsgRBDBError, MsgRBDBConnection, " ("+err.Error()+")"))
			return false
		}
		swapMainDB(ndb, time.Now())
	case CatPermMemDB:
		if fileExists(dbFile) {
			if err := os.Remove(dbFile); err != nil {
				Logging(LtWarning, JoinMsg("DBRBPrepare:", MsgRBDBError, MsgRBDBDelete, dbFile, MsgRBDBDBInUse))
			}
		}
		if !fileExists(dbFile) {
			if err := copyDB(db, dbFile, false); err != nil {
				Logging(LtError, JoinMsg("DBRBPrepare:", MsgRBDBCannot, MsgRBDBBackup, MsgRBDBDB, " ("+err.Error()+")"))
				db.Close()
				return false
			}
		}
		swapMainDB(db, time.Now())
	default: // catMemDB
		swapMainDB(db, time.Now())
	}
	return true
}

// DBRBRefresh rebuilds the DB (timer) and recreates the views of all known AVRs.
func DBRBRefresh() {
	if DBRBPrepare() {
		for i := 0; i < avrCount(); i++ {
			DBRBCheckAVRView(i)
		}
	}
}

// DBRBClose closes the main DB connection.
func DBRBClose() { swapMainDB(nil, time.Time{}) }

// DBRBCreateAVRView mirrors DBRBCreateAVRView: builds the filtered station
// view of an AVR from RBDB_SQL_CREATE_VIEW_AVR.sql.
func DBRBCreateAVRView(avrIdx int) bool {
	db := mainDB()
	if db == nil {
		return false
	}
	lines := strings.Split(strings.ReplaceAll(DBRBGetResourceSQLScript(RBDBSQLCreateViewAVR), "\r\n", "\n"), "\n")
	deleteSection := func(marker string) {
		for {
			start := -1
			for i, l := range lines {
				if strings.TrimRight(l, " \t") == RBDBMarkerBegin+marker {
					start = i
					break
				}
			}
			if start < 0 {
				return
			}
			end := start
			for end < len(lines) && strings.TrimRight(lines[end], " \t") != RBDBMarkerEnd+marker {
				end++
			}
			if end < len(lines) {
				end++ // remove the end marker too
			}
			lines = append(lines[:start], lines[end:]...)
		}
	}
	replaceVariable := func(marker string, values []string) {
		text := strings.Join(lines, "\n")
		text = strings.ReplaceAll(text, RBDBMarkerAt+marker, StripChars(strings.Join(values, ";"), RBFilterStripChars))
		text = strings.ReplaceAll(text, "*", "%")
		lines = strings.Split(text, "\n")
	}
	replaceVariable(RBDBMarkerAVR, []string{avrViewName(avrIdx)})
	f := &avrConfigAt(avrIdx).RBFilter
	for _, x := range []struct {
		marker string
		values []string
	}{
		{AVRIniAllowedTags, f.AllowedTags},
		{AVRIniNotAllowedTags, f.NotAllowedTags},
		{AVRIniAllowedCountries, f.AllowedCountries},
		{AVRIniAllowedLanguages, f.AllowedLanguages},
		{AVRIniAllowedCodecs, f.AllowedCodecs},
		{AVRIniNotAllowedCodecs, f.NotAllowedCodecs},
		{AVRIniNotAllowedInName, f.NotAllowedInName},
		{AVRIniNotAllowedInURL, f.NotAllowedInURL},
	} {
		if len(x.values) == 0 {
			deleteSection(x.marker)
		} else {
			replaceVariable(x.marker, x.values)
		}
	}
	if f.BitrateMax == 0 {
		deleteSection(AVRIniBitrateMax)
	} else {
		replaceVariable(AVRIniBitrateMax, []string{strconv.Itoa(f.BitrateMax)})
	}
	if _, err := db.Exec(strings.Join(lines, "\n")); err != nil {
		Logging(LtError, JoinMsg("DBRBCreateAVRView:", MsgRBDBScript, MsgRBDBExecution, MsgRBDBError, " : ", RBDBSQLCreateViewAVR, " ("+err.Error()+")"))
		return false
	}
	return true
}

// DBRBCheckAVRView mirrors DBRBCheckAVRView: drops outdated views of the AVR
// (other CRC32) and (re)creates the current one.
func DBRBCheckAVRView(avrIdx int) bool {
	db := mainDB()
	if db == nil {
		return false
	}
	rows, err := db.Query(DBRBGetResourceSQLScript(RBDBSQLListViews))
	if err != nil {
		Logging(LtError, JoinMsg("DBRBCheckAVRView:", MsgRBDBQuery, MsgRBDBExecution, MsgRBDBError, " : ", RBDBSQLListViews, " ("+err.Error()+")"))
		return false
	}
	var views []string
	for rows.Next() {
		var v string
		if rows.Scan(&v) == nil {
			views = append(views, v)
		}
	}
	rows.Close()
	crc := uintToStr(avrConfigAt(avrIdx).CRC32)
	for _, v := range views {
		p := strings.Split(v, "_")
		if len(p) == 2 && p[0] == AVRAVR+avrMACAt(avrIdx) && p[1] != crc {
			if !DBRBScriptExec(db, strings.ReplaceAll(DBRBGetResourceSQLScript(RBDBSQLDropViewAVR), RBDBMarkerAt+RBDBMarkerAVR, v)) {
				Logging(LtWarning, JoinMsg("DBRBCheckAVRView:", MsgRBDBDelete, MsgRBDBView, MsgRBDBError, " : ", v))
			}
		}
	}
	return DBRBCreateAVRView(avrIdx)
}

func dbQuery(query, what string) (*sql.Rows, error) {
	db := mainDB()
	if db == nil {
		return nil, errors.New("no database connection")
	}
	rows, err := db.Query(query)
	if err != nil {
		Logging(LtError, JoinMsg("DBRBQuery:", MsgRBDBQuery, MsgRBDBExecution, MsgRBDBError, " : ", what, " ("+err.Error()+")"))
	}
	return rows, err
}

// DBRBGetCategoryItems mirrors DBRBGetCategoryItems.
func DBRBGetCategoryItems(t RBAllCategoryType, avrIdx, start, howMany int) ([]RBCategory, int) {
	q := DBRBGetResourceSQLScript(RBDBSQLSelectCategory)
	q = strings.ReplaceAll(q, RBDBMarkerAt+RBDBMarkerCategory, RBDBFieldsCategories[t])
	q = strings.ReplaceAll(q, RBDBMarkerAt+RBDBMarkerAVR, avrViewName(avrIdx))
	q = strings.Replace(q, RBDBMarkerAt+RBDBMarkerStart, strconv.Itoa(start), 1)
	q = strings.Replace(q, RBDBMarkerAt+RBDBMarkerOffset, strconv.Itoa(howMany), 1)
	rows, err := dbQuery(q, RBDBFieldsCategories[t])
	if err != nil {
		return nil, 0
	}
	defer rows.Close()
	var res []RBCategory
	total := 0
	for rows.Next() {
		var tc int64
		var name sql.NullString
		var cnt sql.NullInt64
		if err := rows.Scan(&tc, &name, &cnt); err != nil {
			Logging(LtError, JoinMsg("DBRBGetCategoryItems:", MsgRBDBQuery, MsgRBDBError, " ("+err.Error()+")"))
			break
		}
		if total == 0 {
			total = int(tc)
		}
		res = append(res, RBCategory{Name: name.String, MaxCount: int(cnt.Int64)})
	}
	return res, total
}

func scanStation(rows *sql.Rows, withTotal bool) (RBStation, int, error) {
	var tc int64
	var f [11]sql.NullString
	dest := []any{}
	if withTotal {
		dest = append(dest, &tc)
	}
	for i := range f {
		dest = append(dest, &f[i])
	}
	if err := rows.Scan(dest...); err != nil {
		return RBStation{}, 0, err
	}
	// column order: stationuuid, name, url, homepage, favicon, votes, bitrate, country, language, codec, tags
	return RBStation{
		ID: f[0].String, Name: f[1].String, URL: f[2].String, HomePageURL: f[3].String, Icon: f[4].String,
		Votes: f[5].String, Bitrate: f[6].String, Country: f[7].String, Language: f[8].String, Codec: f[9].String, Tags: f[10].String,
	}, int(tc), nil
}

// DBRBGetStationsByCategory mirrors DBRBGetStationsByCategory.
func DBRBGetStationsByCategory(t RBAllCategoryType, name string, avrIdx, start, howMany int) ([]RBStation, int) {
	cfg := avrConfigAt(avrIdx)
	dir := "ASC"
	if cfg.RBSort.Reverse {
		dir = "DESC"
	}
	q := DBRBGetResourceSQLScript(RBDBSQLSelectStations)
	q = strings.ReplaceAll(q, RBDBMarkerAt+RBDBMarkerAVR, avrViewName(avrIdx))
	esc := func(s string) string { return strings.ReplaceAll(s, "'", "''") }
	switch t {
	case RbctGenre, RbctCountry, RbctLanguage:
		q = strings.ReplaceAll(q, RBDBMarkerAt+RBDBMarkerCategory, RBDBFieldsCategories[t]+"= '"+esc(StripChars(name, RBFilterStripChars))+"'")
		q = strings.Replace(q, RBDBMarkerAt+RBDBMarkerDirOrder, dir, 1) // before @@@Order (prefix!)
		q = strings.Replace(q, RBDBMarkerAt+RBDBMarkerOrder, cfg.RBSort.Order, 1)
	case RbctPopular:
		q = strings.ReplaceAll(q, RBDBMarkerAt+RBDBMarkerCategory, RBDBFieldsCategories[t]+"=1")
		q = strings.Replace(q, RBDBMarkerAt+RBDBMarkerDirOrder, "DESC", 1)
		q = strings.Replace(q, RBDBMarkerAt+RBDBMarkerOrder, "votes", 1)
	case RbctSearch:
		q = strings.ReplaceAll(q, RBDBMarkerAt+RBDBMarkerCategory, RBDBFieldsCategories[t]+" LIKE '%"+esc(StripChars(name, RBFilterStripChars))+"%'")
		q = strings.Replace(q, RBDBMarkerAt+RBDBMarkerDirOrder, dir, 1)
		q = strings.Replace(q, RBDBMarkerAt+RBDBMarkerOrder, cfg.RBSort.Order, 1)
	}
	q = strings.Replace(q, RBDBMarkerAt+RBDBMarkerStart, strconv.Itoa(start), 1)
	q = strings.Replace(q, RBDBMarkerAt+RBDBMarkerOffset, strconv.Itoa(howMany), 1)
	rows, err := dbQuery(q, RBDBFieldsCategories[t])
	if err != nil {
		return nil, 0
	}
	defer rows.Close()
	var res []RBStation
	total := 0
	for rows.Next() {
		st, tc, err := scanStation(rows, true)
		if err != nil {
			Logging(LtError, JoinMsg("DBRBGetStationsByCategory:", MsgRBDBQuery, MsgRBDBError, " ("+err.Error()+")"))
			break
		}
		if total == 0 {
			total = tc
		}
		res = append(res, st)
	}
	return res, total
}

// DBRBGetStationByID mirrors DBRBGetStationByID (id = 12 hex chars).
func DBRBGetStationByID(id string, avrIdx int) (RBStation, int) {
	q := DBRBGetResourceSQLScript(RBDBSQLSelectStation)
	q = strings.ReplaceAll(q, RBDBMarkerAt+RBDBMarkerAVR, avrViewName(avrIdx))
	q = strings.Replace(q, RBDBMarkerAt+RBDBMarkerID, "'"+strings.ReplaceAll(insertAt(StripChars(id, RBFilterStripChars), 8, "-"), "'", "''")+"-%'", 1)
	rows, err := dbQuery(q, id)
	if err != nil {
		return RBStation{}, 0
	}
	defer rows.Close()
	if rows.Next() {
		st, _, err := scanStation(rows, false)
		if err == nil {
			return st, 1
		}
		Logging(LtError, JoinMsg("DBRBGetStationByID:", MsgRBDBQuery, MsgRBDBError, " ("+err.Error()+")"))
	}
	return RBStation{}, 0
}

// DBRBGetRootItems mirrors DBRBGetRootItems.
func DBRBGetRootItems(avrIdx int) (RBRootCategoryTypesCount, int) {
	var c RBRootCategoryTypesCount
	q := strings.ReplaceAll(DBRBGetResourceSQLScript(RBDBSQLSelectRoot), RBDBMarkerAt+RBDBMarkerAVR, avrViewName(avrIdx))
	rows, err := dbQuery(q, RBDBSQLSelectRoot)
	if err != nil {
		return c, 0
	}
	defer rows.Close()
	if rows.Next() {
		var lang, country, tags, popular int64
		if err := rows.Scan(&lang, &country, &tags, &popular); err == nil {
			c[RbctGenre], c[RbctCountry], c[RbctLanguage], c[RbctPopular] = int(tags), int(country), int(lang), int(popular)
			return c, 1
		}
	}
	return c, 0
}
