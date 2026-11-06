// Package ytuner is a Go port of YTuner (https://github.com/coffeegreg/YTuner),
// a vTuner internet radio service replacement for AVRs.
//
// Copyright (c) 2023 Greg P. (https://github.com/coffeegreg)
// YTuner is licensed under MIT license.
//
// This file corresponds to src/common.pas.
package ytuner

import (
	"fmt"
	"hash/crc32"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// LogType mirrors TLogType.
type LogType int

const (
	LtNone LogType = iota
	LtInfo
	LtWarning
	LtError
	LtDebug
)

// ResponseContentType mirrors TResponseContentType.
type ResponseContentType int

const (
	CtNone ResponseContentType = iota
	CtXML
	CtPNG
	CtJPG
	CtGIF
	CtTIFF
	CtJSON
)

// CacheType mirrors TCacheType.
type CacheType int

const (
	CatNone CacheType = iota
	CatFile
	CatMemStr
	CatDB
	CatMemDB
	CatPermMemDB
)

var cacheTypeNames = []string{"catNone", "catFile", "catMemStr", "catDB", "catMemDB", "catPermMemDB"}

func (c CacheType) String() string {
	if int(c) >= 0 && int(c) < len(cacheTypeNames) {
		return cacheTypeNames[c]
	}
	return "catUnknown"
}

// ParseCacheType is the equivalent of GetEnumValue(TypeInfo(TCacheType), ...) (case-insensitive).
func ParseCacheType(s string) (CacheType, bool) {
	for i, n := range cacheTypeNames {
		if strings.EqualFold(n, strings.TrimSpace(s)) {
			return CacheType(i), true
		}
	}
	return CatNone, false
}

// IsDB reports whether the cache type is one of [catDB, catMemDB, catPermMemDB].
func (c CacheType) IsDB() bool { return c == CatDB || c == CatMemDB || c == CatPermMemDB }

// IsFileOrMemStr reports whether the cache type is one of [catFile, catMemStr].
func (c CacheType) IsFileOrMemStr() bool { return c == CatFile || c == CatMemStr }

const (
	AppName      = "YTuner"
	AppVersion   = "1.2.6"
	AppGoVersion = "go-1"
	AppCopyright = "Copyright (c) 2024 Greg P. (https://github.com/coffeegreg)"
	IniVersion   = "1.2.2"

	YTunerUserAgent = "YTuner"
	YTunerHost      = "ytunerhost"

	MsgFileLoadError          = " file load error"
	MsgFileSaveError          = " file save error"
	MsgLoading                = "loading"
	MsgRemoved                = "removed"
	MsgGetting                = "getting"
	MsgChanging               = "changing"
	MsgError                  = "error"
	MsgLoaded                 = "loaded"
	MsgSave                   = "save"
	MsgSaved                  = "saved"
	MsgEmpty                  = "empty"
	MsgCache                  = "cache"
	MsgFile                   = "file"
	MsgDirectory              = "directory"
	MsgVersion                = "version"
	MsgObsolete               = "obsolete"
	MsgRequired               = "required"
	MsgStream                 = "stream"
	MsgObjects                = "objects"
	MsgBookmark               = "bookmark"
	MsgSuccessfullyLoaded     = "successfully loaded "
	MsgSuccessfullySaved      = "successfully saved "
	MsgSuccessfullyDownloaded = "successfully downloaded "
	MsgErrorLoad              = "load error of"
	MsgNotLoaded              = "not loaded"
	MsgNotFound               = "not found"
	MsgStations               = "stations"
	MsgGoogleTranslate        = "Google Translate:"
	MsgTranslator             = "Translator"
	MsgRBUUIDCacheFile        = "RB UUIDs cache file"

	MsgIniWarning1           = "You are running out of INI file! Some features may not work properly!"
	MsgIniWarning2           = "Your ytuner.ini file is outdated! Some features may not work properly!"
	MsgIniWarning3           = "YTuner will try to fill in the missing options, but check their description and usage on https://github.com/coffeegreg/YTuner/tree/master/cfg."
	MsgVTunerErrorLink1      = "Obslolete vTuner radio station link - trying redirect to "
	MsgVTunerErrorLink2      = "Obslolete vTuner radio station link can not be resolved - trying redirect to your first radio station of stations.ini/yaml file."
	MsgFirstStationNeeded    = "Be sure you have at least one category and one (first one) radio station DIRECT link in stations.ini/yaml file and option \"Enable=1\" is placed in \"[MyStations]\" section of ytuner.ini file ."
	MsgRedirectSupportNeeded = "Your AVR must support HTTP redirects! See \"RedirectHTTPCode\" option."

	IniConfiguration                   = "Configuration"
	IniIniVersion                      = "INIVersion"
	IniIPAddress                       = "IPAddress"
	IniActAsHost                       = "ActAsHost"
	IniUseSSL                          = "UseSSL"
	IniRedirectHTTPCode                = "RedirectHTTPCode"
	IniMessageInfoLevel                = "MessageInfoLevel"
	IniIconSize                        = "IconSize"
	IniIconCache                       = "IconCache"
	IniIconExtension                   = "IconEndPointExtension"
	IniMyToken                         = "MyToken"
	IniCommonAVRIni                    = "CommonAVRini"
	IniCacheFolderLocation             = "CacheFolderLocation"
	IniConfigFolderLocation            = "ConfigFolderLocation"
	IniDBFolderLocation                = "DBFolderLocation"
	IniDBLibFile                       = "DBLibFile"
	IniEnable                          = "Enable"
	IniMyStations                      = "MyStations"
	IniMyStationsFile                  = "MyStationsFile"
	IniMyStationsAutoRefreshPeriod     = "MyStationsAutoRefreshPeriod"
	IniRadioBrowser                    = "RadioBrowser"
	IniRBAPIURL                        = "RBAPIURL"
	IniRBPopularAndSearchStationsLimit = "RBPopularAndSearchStationsLimit"
	IniRBMinStationsPerCategory        = "RBMinStationsPerCategory"
	IniRBUUIDsCacheTTL                 = "RBUUIDsCacheTTL"
	IniRBUUIDsCacheAutoRefresh         = "RBUUIDsCacheAutoRefresh"
	IniRBCacheType                     = "RBCacheType"
	IniRBCacheTTL                      = "RBCacheTTL"
	IniBookmark                        = "Bookmark"
	IniCommonBookmark                  = "CommonBookmark"
	IniBookmarkStationsLimit           = "BookmarkStationsLimit"
	IniWebServer                       = "WebServer"
	IniWebServerIPAddress              = "WebServerIPAddress"
	IniWebServerPort                   = "WebServerPort"
	IniDNSServer                       = "DNSServer"
	IniDNSServerIPAddress              = "DNSServerIPAddress"
	IniDNSServerPort                   = "DNSServerPort"
	IniInterceptDNs                    = "InterceptDNs"
	IniDNSServers                      = "DNSServers"
	IniMaintenanceServer               = "MaintenanceServer"
	IniMaintenanceServerIPAddress      = "MaintenanceServerIPAddress"
	IniMaintenanceServerPort           = "MaintenanceServerPort"

	HTTPHeaderAccept                  = "Accept"
	HTTPHeaderUserAgent               = "User-Agent"
	HTTPHeaderLocation                = "Location"
	HTTPHeaderServer                  = "Server"
	HTTPHeaderAcceptEncoding          = "Accept-Encoding"
	HTTPHeaderAcceptLanguage          = "Accept-Language"
	HTTPHeaderCacheControl            = "Cache-Control"
	HTTPHeaderConnection              = "Connection"
	HTTPHeaderPragma                  = "Pragma"
	HTTPHeaderUpgradeInsecureRequests = "Upgrade-Insecure-Requests"

	WebBrowserUserAgent      = "Mozilla/5.0 (Windows NT 6.1; Win64; x64; rv:109.0) Gecko/20100101 Firefox/115.0"
	WebBrowserAccept         = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.9"
	WebBrowserAcceptEncoding = "gzip, deflate"
	WebBrowserAcceptLanguage = "en-US,en;q=0.9"
	WebBrowserCacheControl   = "no-cache"
	WebBrowserConnection     = "keep-alive"
	WebBrowserPragma         = "no-cache"

	MyStationsPrefix   = "MS"
	RadioBrowserPrefix = "RB"
	UnknownPrefix      = "UN"
	PathMyStations     = "mystations"

	PathParamID          = "id"
	PathParamMAC         = "mac"
	PathParamFav         = "fav"
	PathParamSearch      = "search"
	PathParamSSearchType = "sSearchtype"
	PathParamToken       = "token"

	HTTPCodeOK          = 200
	HTTPCodeRedirect    = 302
	HTTPCodeNotFound    = 404
	HTTPCodeUnavailable = 503

	DefaultString = "default"

	PathCache  = "cache"
	PathConfig = "config"
	PathDB     = "db"

	CacheExt = ".cache"

	DefIconSize                        = 200
	DefIconCache                       = true
	DefCommonAVRIni                    = true
	DefRBPopularAndSearchStationsLimit = 100
	DefRBMinStationsPerCategory        = 3
	DefRBUUIDsCacheTTL                 = 0
	DefRBCacheTTL                      = 0

	SQLiteVerXMin = 3
	SQLiteVerYMin = 33
	SQLiteVerZMin = 0
)

// HTTPResponseContentType mirrors HTTP_RESPONSE_CONTENT_TYPE.
var HTTPResponseContentType = [...]string{
	"text/html; charset=utf-8", "application/xml", "image/png", "image/jpeg", "image/gif", "image/tiff", "application/json",
}

var logTypeMsg = [...]string{"", "Inf", "Wrn", "Err", "Dbg"}

// Global configuration/state (the Pascal unit-level variables of common.pas).
var (
	MyIPAddress = DefaultString
	URLHost     = ""
	LogLevel    = LtError
	MyAppPath   string
	UseSSL      = true
	CachePath   = DefaultString
	ConfigPath  = DefaultString
	DBPath      = DefaultString
	DBLibFile   = DefaultString

	logMu  sync.Mutex
	logger = log.New(os.Stdout, "", 0)
)

// LineEnding is the platform line ending (Pascal LineEnding). It is used for
// XML output and INI files so files stay byte compatible with the Pascal build.
var LineEnding = func() string {
	if runtime.GOOS == "windows" {
		return "\r\n"
	}
	return "\n"
}()

// Logging prints a log line if ALogType <= configured level (procedure Logging).
func Logging(lt LogType, msg string) {
	if lt > LogLevel || lt <= LtNone {
		return
	}
	if msg != "" {
		r, size := utf8.DecodeRuneInString(msg)
		msg = string(unicode.ToUpper(r)) + msg[size:]
	}
	logMu.Lock()
	logger.Printf("%s : %s : %s.", time.Now().Format("2006-01-02 15:04:05"), logTypeMsg[lt], msg)
	logMu.Unlock()
}

// Logf is a convenience wrapper around Logging.
func Logf(lt LogType, format string, args ...any) {
	if lt > LogLevel {
		return
	}
	Logging(lt, fmt.Sprintf(format, args...))
}

// JoinMsg mirrors string.Join(' ', [...]).
func JoinMsg(parts ...string) string { return strings.Join(parts, " ") }

// isIPv4 mirrors the nested IsIP_v4 of GetLocalIP.
func isIPv4(s string) bool {
	p := strings.Split(s, ".")
	if len(p) != 4 {
		return false
	}
	for _, x := range p {
		n, err := strconv.Atoi(x)
		if err != nil || n < 0 || n > 255 {
			return false
		}
	}
	return true
}

// localIPv4Addresses returns all IPv4 addresses of the host (including loopback).
var localIPv4Addresses = func() ([]string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	var res []string
	for _, a := range addrs {
		var ip net.IP
		switch v := a.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip4 := ip.To4(); ip4 != nil {
			res = append(res, ip4.String())
		}
	}
	return res, nil
}

// GetLocalIP mirrors GetLocalIP: returns ADefaultIP if it is a loopback address
// or one of the local IPv4 addresses, otherwise the first non-loopback IPv4
// address, otherwise a loopback address.
func GetLocalIP(defaultIP string) string {
	if isIPv4(defaultIP) && strings.Split(defaultIP, ".")[0] == "127" {
		return defaultIP
	}
	list, err := localIPv4Addresses()
	if err != nil {
		Logging(LtError, "IP error: "+err.Error())
		return ""
	}
	if len(list) == 0 {
		Logging(LtError, "No entries on IP List")
		return ""
	}
	result, loopback := "", ""
	for _, ip := range list {
		if result == defaultIP {
			break
		}
		if strings.Split(ip, ".")[0] == "127" {
			loopback = ip
		} else if result == "" {
			result = ip
		}
		if ip == defaultIP {
			result = defaultIP
		}
	}
	if result == "" && loopback != "" {
		result = loopback
	}
	return result
}

// CalcFileCRC32 mirrors CalcFileCRC32 (zlib CRC32 == IEEE).
func CalcFileCRC32(fileName string) (uint32, error) {
	b, err := os.ReadFile(fileName)
	if err != nil {
		return 0, err
	}
	return crc32.ChecksumIEEE(b), nil
}

// HaveCommonElements mirrors HaveCommonElements: splits s by , / ; and checks
// each (trimmed) element against the wildcard patterns (case-insensitive).
func HaveCommonElements(s string, patterns []string) bool {
	for _, e := range pasSplit(s, ",/;") {
		e = strings.TrimSpace(e)
		for _, p := range patterns {
			if IsWild(e, p, true) {
				return true
			}
		}
	}
	return false
}

// pasSplit mirrors TStringHelper.Split with a set of single-char separators:
// empty elements are kept, an empty input yields no elements.
func pasSplit(s, seps string) []string {
	if s == "" {
		return nil
	}
	var res []string
	start := 0
	for i := 0; i < len(s); i++ {
		if strings.IndexByte(seps, s[i]) >= 0 {
			res = append(res, s[start:i])
			start = i + 1
		}
	}
	return append(res, s[start:])
}

// ContainsIn mirrors ContainsIn (case-insensitive substring of any).
func ContainsIn(s string, subs []string) bool {
	ls := strings.ToLower(s)
	for _, x := range subs {
		if strings.Contains(ls, strings.ToLower(x)) {
			return true
		}
	}
	return false
}

// IsWild mirrors StrUtils.IsWild: '*' matches any sequence, '?' any single char.
func IsWild(input, wilds string, ignoreCase bool) bool {
	if ignoreCase {
		input = strings.ToLower(input)
		wilds = strings.ToLower(wilds)
	}
	in := []rune(input)
	pat := []rune(wilds)
	// classic iterative wildcard matching with backtracking on last '*'
	i, p := 0, 0
	star, mark := -1, 0
	for i < len(in) {
		switch {
		case p < len(pat) && (pat[p] == '?' || pat[p] == in[i]):
			i++
			p++
		case p < len(pat) && pat[p] == '*':
			star = p
			mark = i
			p++
		case star >= 0:
			p = star + 1
			mark++
			i = mark
		default:
			return false
		}
	}
	for p < len(pat) && pat[p] == '*' {
		p++
	}
	return p == len(pat)
}

// StripChars mirrors StripChars: replaces every listed char with a space,
// trims and collapses runs of spaces into one (DelSpace1).
func StripChars(s string, chars []string) string {
	if len(chars) > 0 {
		pairs := make([]string, 0, 2*len(chars))
		for _, c := range chars {
			pairs = append(pairs, c, " ")
		}
		s = strings.NewReplacer(pairs...).Replace(s)
	}
	s = strings.TrimFunc(s, func(r rune) bool { return r <= ' ' })
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if r == ' ' {
			if prevSpace {
				continue
			}
			prevSpace = true
		} else {
			prevSpace = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

// URLEncode mirrors URLEncode: every byte outside A-Z/a-z becomes %XX.
func URLEncode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// HTTPDecode mirrors httpdefs.HTTPDecode: '+' -> ' ' and %XX decoding
// (invalid escapes are kept as-is).
func HTTPDecode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '+':
			b.WriteByte(' ')
		case c == '%' && i+2 < len(s) && isHex(s[i+1]) && isHex(s[i+2]):
			v, _ := strconv.ParseUint(s[i+1:i+3], 16, 8)
			b.WriteByte(byte(v))
			i += 2
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// HTTPEncode mirrors httpdefs.HTTPEncode: keeps A-Z a-z 0-9 and "-_.!~*'()",
// space -> '+', everything else %XX.
func HTTPEncode(s string) string {
	const safe = "-_.!~*'()"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || strings.IndexByte(safe, c) >= 0:
			b.WriteByte(c)
		case c == ' ':
			b.WriteByte('+')
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// PascalStringHash mirrors TStringHelper.GetHashCode (used in cache file names
// so cache files of the Pascal build can be reused).
func PascalStringHash(s string) int32 {
	var h uint32
	for i := 0; i < len(s); i++ {
		h = uint32(int32(h<<5)-int32(h)) ^ uint32(s[i])
	}
	return int32(h)
}

// AbsInt32 mirrors Abs() on a Pascal Integer (as int64 to avoid overflow of MinInt32).
func AbsInt32(v int32) int64 {
	if v < 0 {
		return -int64(v)
	}
	return int64(v)
}

// asciiLower mirrors AnsiString.ToLower (ASCII only).
func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// pasSubstring mirrors Pascal string.Substring(start, length) with clamping
// (FPC returns ” / shortened strings instead of raising).
func pasSubstring(s string, start, length int) string {
	if start < 0 {
		start = 0
	}
	if start >= len(s) {
		return ""
	}
	end := start + length
	if length < 0 || end > len(s) {
		end = len(s)
	}
	return s[start:end]
}

// tryStrToInt mirrors TryStrToInt: on failure the value is 0. Accepts
// Pascal hex notation ($FF, 0x..).
func tryStrToInt(s string) (int, bool) {
	t := strings.TrimSpace(s)
	if t != s || t == "" {
		// FPC Val tolerates leading spaces only; keep it simple and strict
		if t == "" {
			return 0, false
		}
	}
	base := 10
	neg := false
	if strings.HasPrefix(t, "-") {
		neg = true
		t = t[1:]
	} else if strings.HasPrefix(t, "+") {
		t = t[1:]
	}
	switch {
	case strings.HasPrefix(t, "$"):
		base, t = 16, t[1:]
	case strings.HasPrefix(strings.ToLower(t), "0x"):
		base, t = 16, t[2:]
	}
	v, err := strconv.ParseInt(t, base, 32)
	if err != nil {
		return 0, false
	}
	if neg {
		v = -v
	}
	return int(v), true
}

// pathJoin mirrors Path+DirectorySeparator+Name concatenation.
func pathJoin(dir, name string) string { return filepath.Join(dir, name) }

// fileExists mirrors FileExists.
func fileExists(name string) bool {
	st, err := os.Stat(name)
	return err == nil && !st.IsDir()
}

// dirExists mirrors DirectoryExists.
func dirExists(name string) bool {
	st, err := os.Stat(name)
	return err == nil && st.IsDir()
}

// fileAge returns the modification time of a file (FileDateToDateTime(FileAge(..))).
func fileAge(name string) (time.Time, bool) {
	st, err := os.Stat(name)
	if err != nil {
		return time.Time{}, false
	}
	return st.ModTime(), true
}

// expiredAfterHours reports whether t + hours < now.
func expiredAfterHours(t time.Time, hours int) bool {
	return t.Add(time.Duration(hours) * time.Hour).Before(time.Now())
}

// GetMyAppPath mirrors GetMyAppPath (directory of the executable).
func GetMyAppPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "." + string(os.PathSeparator)
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	if strings.Contains(exe, "ld-musl-") {
		return "./"
	}
	return filepath.Dir(exe) + string(os.PathSeparator)
}
