package ytuner

// Radio-browser.info API support (src/radiobrowser.pas).

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RBStation mirrors TRBStation.
type RBStation struct {
	ID, Name, HomePageURL, URL, Icon, Tags, Country, CountryCode, Language, Votes, Codec, Bitrate string
}

// RBCategory mirrors TRBCategory.
type RBCategory struct {
	Name     string
	MaxCount int
}

// RBAllCategoryType mirrors TRBAllCategoryTypes.
type RBAllCategoryType int

const (
	RbctGenre RBAllCategoryType = iota
	RbctCountry
	RbctLanguage
	RbctPopular
	RbctSearch
)

// RBRootCategoryTypesCount mirrors TRBRootCategoryTypesCount.
type RBRootCategoryTypesCount [5]int

const (
	PathRadioBrowser = "radiobrowser"
	APISearchPath    = "search"
	APIStationsPath  = "stations"
	APIOrderPath     = "order"
	APIReversePath   = "reverse"
	APIOffsetPath    = "offset"
	APILimitPath     = "limit"

	APIPopularFilterPath    = "?order=votes&reverse=true&limit="
	APISearchFilterPath     = "?order=name&reverse=false&limit="
	APIHideBrokenFilterPath = "hidebroken=true"

	APIByTagExactPath      = "/bytagexact/"
	APIByCountryExactPath  = "/bycountryexact/"
	APIByLanguageExactPath = "/bylanguageexact/"
	APIByUUIDPath          = "/byuuid/"

	APIAttrStationUUID  = "stationuuid"
	APIAttrServerUUID   = "serveruuid"
	APIAttrName         = "name"
	APIAttrURL          = "url"
	APIAttrHomepage     = "homepage"
	APIAttrFavicon      = "favicon"
	APIAttrTags         = "tags"
	APIAttrCountry      = "country"
	APIAttrCountryCode  = "countrycode"
	APIAttrLanguage     = "language"
	APIAttrVotes        = "votes"
	APIAttrCodec        = "codec"
	APIAttrBitrate      = "bitrate"
	APIAttrStationCount = "stationcount"

	MsgRadioBrowser        = "Radio-browser.info"
	MsgRBCacheFileType     = "catFile"
	MsgRBCacheMemStrType   = "catMemStr"
	MsgRBCacheTTLExpReload = "TTL expired. Reloading"

	RadioBrowserUUIDsFile = "rbuuids.txt"

	RBCacheStationsPrefix   = "RBS-"
	RBCacheCategoriesPrefix = "RBC-"

	HideBrokenStations = true
)

// RBCategoriesPath mirrors RB_CATEGORIES_PATH (genre, country, language).
var RBCategoriesPath = [3]string{"tags", "countries", "languages"}

// RBJSONStripChars mirrors RB_JSON_STRIP_CHARS.
var RBJSONStripChars = []string{"!", "*", "'", "(", ")", ";", ":", "@", "&", "=", "+", "$", ",", "/", "?", "%", "#", "[", "]", "-", "_", ".", "~", "\n", "\r", "\t"}

// RBFilterStripChars mirrors RB_FILTER_STRIP_CHARS.
var RBFilterStripChars = []string{"!", "\"", "'", "(", ")", ":", "@", "&", "=", "+", "$", ",", "/", "?", "%", "#", "[", "]", "-", ".", "~", "\n", "\r", "\t"}

var (
	RadioBrowserEnabled             = true
	RBAPIURL                        = "http://all.api.radio-browser.info"
	RBPopularAndSearchStationsLimit = DefRBPopularAndSearchStationsLimit
	RBMinStationsPerCategory        = DefRBMinStationsPerCategory
	RBUUIDsFilePath                 = ""
	RBUUIDsCacheTTL                 = DefRBUUIDsCacheTTL
	RBUUIDsCacheAutoRefresh         = false
	RBCacheTTL                      = DefRBCacheTTL
	RBCacheType                     = CatFile

	rbUUIDsMu          sync.RWMutex
	rbStationsUUIDsStr string

	rbCache = &RBCacheList{}
)

// rbHTTPClient is used for API requests (redirects are followed).
var rbHTTPClient = &http.Client{Timeout: 10 * time.Minute}

func rbStationsUUIDs() string {
	rbUUIDsMu.RLock()
	defer rbUUIDsMu.RUnlock()
	return rbStationsUUIDsStr
}

// GetAPIURLRange mirrors GetAPIURLRange.
func GetAPIURLRange(elementNumber, elementCount int) string {
	return "?" + APIOffsetPath + "=" + strconv.Itoa(elementNumber) + "&" + APILimitPath + "=" + strconv.Itoa(elementCount)
}

// RadiobrowserAPIRequest mirrors RadiobrowserAPIRequest ('[]' on any error).
func RadiobrowserAPIRequest(apiURL string) []byte {
	Logging(LtDebug, MsgRadioBrowser+": GET "+apiURL)
	b, err := httpGetBytes(rbHTTPClient, RBAPIURL+"/json/"+apiURL, map[string]string{
		HTTPHeaderAccept:    HTTPResponseContentType[CtJSON],
		HTTPHeaderUserAgent: YTunerUserAgent + "/" + AppVersion,
	}, 0)
	if err != nil {
		Logging(LtError, MsgRadioBrowser+": "+MsgError+" "+MsgGetting+" "+apiURL+" ("+err.Error()+")")
		return []byte("[]")
	}
	return b
}

// httpGetBytes performs a GET and returns the body for 2xx responses.
// maxBytes > 0 limits the body size.
func httpGetBytes(c *http.Client, u string, headers map[string]string, maxBytes int64) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("unexpected response status code: %d", resp.StatusCode)
	}
	var r io.Reader = resp.Body
	if maxBytes > 0 {
		r = io.LimitReader(r, maxBytes)
	}
	return io.ReadAll(r)
}

// jsonObj is a decoded radio-browser JSON object.
type jsonObj map[string]any

// RadiobrowserAPIRequestJSON mirrors RadiobrowserAPIRequestJSON for arrays of objects.
func RadiobrowserAPIRequestJSON(apiURL string) ([]jsonObj, error) {
	dec := json.NewDecoder(bytes.NewReader(RadiobrowserAPIRequest(apiURL)))
	dec.UseNumber()
	var arr []jsonObj
	if err := dec.Decode(&arr); err != nil {
		return nil, err
	}
	return arr, nil
}

// Get mirrors TJSONObject.Get(Name) converted to string.
func (o jsonObj) Get(name string) string {
	switch v := o[name].(type) {
	case nil:
		return ""
	case string:
		return v
	case json.Number:
		return v.String()
	case bool:
		if v {
			return "True"
		}
		return "False"
	default:
		return fmt.Sprint(v)
	}
}

// GetInt mirrors TJSONObject.Get(Name, Default) for integers.
func (o jsonObj) GetInt(name string, def int) int {
	switch v := o[name].(type) {
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return int(i)
		}
		if f, err := v.Float64(); err == nil {
			return int(f)
		}
	case string:
		if i, ok := tryStrToInt(v); ok {
			return i
		}
	}
	return def
}

// SetRBCategory mirrors SetRBCategory.
func SetRBCategory(o jsonObj) RBCategory {
	return RBCategory{Name: StripChars(o.Get(APIAttrName), RBJSONStripChars), MaxCount: o.GetInt(APIAttrStationCount, 0)}
}

// SetRBStation mirrors SetRBStation.
func SetRBStation(o jsonObj) RBStation {
	return RBStation{
		ID:          o.Get(APIAttrStationUUID),
		Name:        StripChars(o.Get(APIAttrName), RBJSONStripChars),
		HomePageURL: o.Get(APIAttrHomepage),
		URL:         o.Get(APIAttrURL),
		Icon:        o.Get(APIAttrFavicon),
		Tags:        StripChars(o.Get(APIAttrTags), RBJSONStripChars),
		Country:     StripChars(o.Get(APIAttrCountry), RBJSONStripChars),
		CountryCode: o.Get(APIAttrCountryCode),
		Language:    StripChars(o.Get(APIAttrLanguage), RBJSONStripChars),
		Votes:       o.Get(APIAttrVotes),
		Codec:       o.Get(APIAttrCodec),
		Bitrate:     o.Get(APIAttrBitrate),
	}
}

func countUUIDs(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, ",") + 1
}

// LoadRBStationsUUIDs mirrors LoadRBStationsUUIDs (returns true if the
// cache file was loaded and is still valid).
func LoadRBStationsUUIDs() bool {
	RBUUIDsFilePath = pathJoin(CachePath, RadioBrowserUUIDsFile)
	if !fileExists(RBUUIDsFilePath) {
		return false
	}
	Logging(LtInfo, MsgLoading+" "+MsgRBUUIDCacheFile)
	f, err := os.Open(RBUUIDsFilePath)
	if err != nil {
		Logging(LtError, MsgErrorLoad+" "+MsgRBUUIDCacheFile)
		return false
	}
	defer f.Close()
	rd := bufio.NewReaderSize(f, 1<<20)
	line, err := rd.ReadString('\n')
	if err != nil && err != io.EOF {
		Logging(LtError, MsgErrorLoad+" "+MsgRBUUIDCacheFile)
		return false
	}
	line = strings.TrimRight(line, "\r\n")
	rbUUIDsMu.Lock()
	rbStationsUUIDsStr = line
	rbUUIDsMu.Unlock()
	if len(line) <= 2 {
		Logging(LtWarning, MsgRBUUIDCacheFile+" seems empty..")
		return false
	}
	Logging(LtInfo, MsgSuccessfullyLoaded+strconv.Itoa(countUUIDs(line))+" RB UUIDs from cache file")
	age, _ := fileAge(RBUUIDsFilePath)
	if RBUUIDsCacheTTL == 0 || !expiredAfterHours(age, RBUUIDsCacheTTL) {
		return true
	}
	Logging(LtInfo, MsgRBUUIDCacheFile+" is older then "+strconv.Itoa(RBUUIDsCacheTTL)+" hours. Refreshing RB UUIDs list..")
	return false
}

// GetRBStationsUUIDs mirrors GetRBStationsUUIDs: downloads all station UUIDs
// and stores them comma separated in memory and in the cache file.
func GetRBStationsUUIDs() {
	Logging(LtInfo, MsgGetting+" "+MsgRadioBrowser+" UUIDs..")
	if RBUUIDsFilePath == "" {
		RBUUIDsFilePath = pathJoin(CachePath, RadioBrowserUUIDsFile)
	}
	body := RadiobrowserAPIRequest(APIStationsPath)
	dec := json.NewDecoder(bytes.NewReader(body))
	var arr []struct {
		StationUUID string `json:"stationuuid"`
	}
	if err := dec.Decode(&arr); err != nil {
		Logging(LtWarning, MsgRadioBrowser+" UUIDs "+MsgNotLoaded+". Error: "+err.Error())
		return
	}
	ids := make([]string, 0, len(arr))
	for _, s := range arr {
		ids = append(ids, s.StationUUID)
	}
	uuids := strings.Join(ids, ",")
	if len(uuids) <= 2 {
		return
	}
	rbUUIDsMu.Lock()
	rbStationsUUIDsStr = uuids
	rbUUIDsMu.Unlock()
	Logging(LtInfo, MsgSuccessfullyDownloaded+strconv.Itoa(countUUIDs(uuids))+" RB UUIDs")
	if err := os.WriteFile(RBUUIDsFilePath, []byte(uuids), 0o644); err != nil {
		Logging(LtError, RadioBrowserUUIDsFile+" access error")
		return
	}
	Logging(LtInfo, MsgSuccessfullySaved+strconv.Itoa(countUUIDs(uuids))+" RB UUIDs to cache file")
}

// GetRBStationByID mirrors GetRBStationByID; id is the 12 hex chars of a
// "RB_xxxxxxxxxxxx" station id. Returns a station with empty ID if not found.
func GetRBStationByID(id string, avrIdx int) RBStation {
	if RBCacheType.IsDB() {
		st, _ := DBRBGetStationByID(id, avrIdx)
		return st
	}
	uuids := rbStationsUUIDs()
	needle := strings.ToLower(insertAt(id, 8, "-") + "-")
	pos := strings.Index(strings.ToLower(uuids), needle)
	if pos < 0 || len(id) < 8 {
		return RBStation{}
	}
	uuid := pasSubstring(uuids, pos, 36)
	if RBCacheType != CatNone {
		if st, ok := GetRBStationCache(uuid, avrIdx); ok {
			return st
		}
	}
	arr, err := RadiobrowserAPIRequestJSON(APIStationsPath + APIByUUIDPath + uuid)
	if err != nil || len(arr) == 0 {
		msg := "no data"
		if err != nil {
			msg = err.Error()
		}
		Logging(LtError, "GetRBStationByID error: "+msg)
		return RBStation{}
	}
	return SetRBStation(arr[0])
}

// insertAt mirrors string.Insert(index, value) (clamped).
func insertAt(s string, idx int, v string) string {
	if idx > len(s) {
		idx = len(s)
	}
	return s[:idx] + v + s[idx:]
}

func rbStationsCacheName(avrIdx int, t RBAllCategoryType, name string) string {
	return RBCacheStationsPrefix + avrMACAt(avrIdx) + "-" + strconv.Itoa(int(t)) + "-" +
		strconv.FormatInt(AbsInt32(PascalStringHash(asciiLower(name))), 10)
}

func rbCategoriesCacheName(avrIdx int, t RBAllCategoryType) string {
	return RBCacheCategoriesPrefix + avrMACAt(avrIdx) + "-" + strconv.Itoa(int(t))
}

// GetStationsByCategoryReq mirrors GetStationsByCategory(..., AReq).
func GetStationsByCategoryReq(t RBAllCategoryType, name string, req *Request) ([]RBStation, int) {
	start, howMany, _ := GetRange(req.Query)
	if RBCacheType.IsDB() {
		return DBRBGetStationsByCategory(t, HTTPDecode(name), GetAVRConfigIdx(req), start, howMany)
	}
	return GetStationsByCategory(t, name, GetAVRConfigIdx(req), start, howMany)
}

// GetStationsByCategory mirrors GetStationsByCategory(..., AStart, AHowMany).
// Returns the requested page and the total number of (filtered) stations.
func GetStationsByCategory(t RBAllCategoryType, name string, avrIdx, start, howMany int) ([]RBStation, int) {
	cacheName := rbStationsCacheName(avrIdx, t, name)
	var result []RBStation
	total := 0
	if RBCacheType.IsFileOrMemStr() {
		var stations []RBStation
		total = rbCache.Get(&stations, nil, cacheName, avrIdx, start, howMany)
		if total > 0 {
			return stations, total
		}
	}
	cfg := avrConfigAt(avrIdx)
	sortingPath := "?" + APIOrderPath + "=" + HTTPEncode(cfg.RBSort.Order) + "&" + APIReversePath + "=" + strconv.FormatBool(cfg.RBSort.Reverse)
	var u string
	switch t {
	case RbctGenre:
		u = APIStationsPath + APIByTagExactPath + name + sortingPath
	case RbctCountry:
		u = APIStationsPath + APIByCountryExactPath + name + sortingPath
	case RbctLanguage:
		u = APIStationsPath + APIByLanguageExactPath + name + sortingPath
	case RbctPopular:
		u = APIStationsPath + APIPopularFilterPath + strconv.Itoa(RBPopularAndSearchStationsLimit)
	case RbctSearch:
		u = APIStationsPath + "/" + APISearchPath + APISearchFilterPath + strconv.Itoa(RBPopularAndSearchStationsLimit) + "&" + APIAttrName + "=" + name
	}
	if HideBrokenStations {
		u += "&" + APIHideBrokenFilterPath
	}
	arr, err := RadiobrowserAPIRequestJSON(u)
	if err != nil {
		Logging(LtError, JoinMsg("GetStationsByCategory", MsgError, " ("+err.Error()+")"))
		return nil, 0
	}
	categoriesCache := rbCategoriesCacheName(avrIdx, t)
	if len(arr) == 0 {
		if RBCacheType.IsFileOrMemStr() {
			RemoveEmptyCategory(categoriesCache, HTTPDecode(name), avrIdx)
		}
		return nil, 0
	}
	var all []RBStation
	filtered, added := 0, 0
	for _, o := range arr {
		if !RBStationsAVRFilter(o, t, avrIdx) {
			continue
		}
		st := SetRBStation(o)
		if RBCacheType.IsFileOrMemStr() {
			all = append(all, st)
		}
		if filtered >= start && (added < howMany || howMany <= 0) {
			result = append(result, st)
			added++
		}
		filtered++
	}
	total = filtered
	if RBCacheType.IsFileOrMemStr() {
		if len(all) == 0 && t <= RbctLanguage {
			RemoveEmptyCategory(categoriesCache, HTTPDecode(name), avrIdx)
		} else {
			rbCache.SetStations(all, cacheName, avrIdx)
		}
	}
	return result, total
}

// GetRootItems mirrors GetRootItems.
func GetRootItems(avrIdx int) RBRootCategoryTypesCount {
	var c RBRootCategoryTypesCount
	if RBCacheType.IsDB() {
		c, _ = DBRBGetRootItems(avrIdx)
		return c
	}
	c[RbctGenre] = GetCategoryItemsCount(RbctGenre, avrIdx)
	c[RbctCountry] = GetCategoryItemsCount(RbctCountry, avrIdx)
	c[RbctLanguage] = GetCategoryItemsCount(RbctLanguage, avrIdx)
	c[RbctPopular] = RBPopularAndSearchStationsLimit
	return c
}

// GetCategoryItemsCount mirrors GetCategoryItemsCount.
func GetCategoryItemsCount(t RBAllCategoryType, avrIdx int) int {
	if RBCacheType.IsDB() {
		_, total := DBRBGetCategoryItems(t, avrIdx, 0, 1)
		return total
	}
	_, total := GetCategoryItems(t, avrIdx, 0, 1)
	return total
}

// GetCategoryItemsReq mirrors GetCategoryItems(..., AReq).
func GetCategoryItemsReq(t RBAllCategoryType, req *Request) ([]RBCategory, int) {
	start, howMany, _ := GetRange(req.Query)
	if RBCacheType.IsDB() {
		return DBRBGetCategoryItems(t, GetAVRConfigIdx(req), start, howMany)
	}
	return GetCategoryItems(t, GetAVRConfigIdx(req), start, howMany)
}

// GetCategoryItems mirrors GetCategoryItems(..., AStart, AHowMany).
func GetCategoryItems(t RBAllCategoryType, avrIdx, start, howMany int) ([]RBCategory, int) {
	cacheName := rbCategoriesCacheName(avrIdx, t)
	if RBCacheType.IsFileOrMemStr() {
		var cats []RBCategory
		if total := rbCache.Get(nil, &cats, cacheName, avrIdx, start, howMany); total > 0 {
			return cats, total
		}
	}
	u := RBCategoriesPath[t]
	if HideBrokenStations {
		u += "?" + APIHideBrokenFilterPath
	}
	arr, err := RadiobrowserAPIRequestJSON(u)
	if err != nil {
		Logging(LtError, JoinMsg("GetCategoryItems", MsgError, " ("+err.Error()+")"))
		return nil, 0
	}
	var result, all []RBCategory
	filtered, added := 0, 0
	for _, o := range arr {
		if !RBCategoriesAVRFilter(o, t, avrIdx) {
			continue
		}
		c := SetRBCategory(o)
		if RBCacheType.IsFileOrMemStr() {
			all = append(all, c)
		}
		if filtered >= start && (added < howMany || howMany <= 0) {
			result = append(result, c)
			added++
		}
		filtered++
	}
	if RBCacheType.IsFileOrMemStr() && len(all) > 0 {
		rbCache.SetCategories(all, cacheName, avrIdx)
	}
	return result, filtered
}

// RBCategoriesAVRFilter mirrors RBCategoriesAVRFilter.
func RBCategoriesAVRFilter(o jsonObj, t RBAllCategoryType, avrIdx int) bool {
	f := &avrConfigAt(avrIdx).RBFilter
	name := o.Get(APIAttrName)
	if name == "" || o.GetInt(APIAttrStationCount, 0) < RBMinStationsPerCategory {
		return false
	}
	switch t {
	case RbctGenre:
		if len(f.AllowedTags) > 0 && !HaveCommonElements(name, f.AllowedTags) {
			return false
		}
		if len(f.NotAllowedTags) > 0 && HaveCommonElements(name, f.NotAllowedTags) {
			return false
		}
	case RbctCountry:
		if len(f.AllowedCountries) > 0 && !HaveCommonElements(name, f.AllowedCountries) {
			return false
		}
	case RbctLanguage:
		if len(f.AllowedLanguages) > 0 && !HaveCommonElements(name, f.AllowedLanguages) {
			return false
		}
	}
	return true
}

func orEmpty(s string) string {
	if s == "" {
		return AVRFilterEmpty
	}
	return s
}

// RBStationsAVRFilter mirrors RBStationsAVRFilter.
//
// NOTE: two quirks of the Pascal original are kept on purpose for identical
// results: BitrateMax parses the *codec* attribute (so it never filters in
// catNone/catFile/catMemStr mode) and NotAllowedInName checks the *URL*.
// See claude/01/result.md.
func RBStationsAVRFilter(o jsonObj, t RBAllCategoryType, avrIdx int) bool {
	f := &avrConfigAt(avrIdx).RBFilter
	cfg := avrConfigAt(avrIdx)
	tags, country, language := orEmpty(o.Get(APIAttrTags)), orEmpty(o.Get(APIAttrCountry)), orEmpty(o.Get(APIAttrLanguage))
	allowedTags := func() bool { return len(f.AllowedTags) == 0 || HaveCommonElements(tags, f.AllowedTags) }
	notAllowedTags := func() bool { return len(f.NotAllowedTags) > 0 && HaveCommonElements(tags, f.NotAllowedTags) }
	allowedCountries := func() bool { return len(f.AllowedCountries) == 0 || HaveCommonElements(country, f.AllowedCountries) }
	allowedLanguages := func() bool { return len(f.AllowedLanguages) == 0 || HaveCommonElements(language, f.AllowedLanguages) }

	switch t {
	case RbctGenre:
		if !allowedCountries() || !allowedLanguages() {
			return false
		}
	case RbctCountry:
		if !allowedTags() || notAllowedTags() || !allowedLanguages() {
			return false
		}
	case RbctLanguage:
		if !allowedTags() || notAllowedTags() || !allowedCountries() {
			return false
		}
	case RbctPopular:
		if !allowedTags() || notAllowedTags() || !allowedCountries() || !allowedLanguages() {
			return false
		}
	}
	codec := orEmpty(o.Get(APIAttrCodec))
	if len(f.AllowedCodecs) > 0 && !HaveCommonElements(codec, f.AllowedCodecs) {
		return false
	}
	if len(f.NotAllowedCodecs) > 0 && HaveCommonElements(codec, f.NotAllowedCodecs) {
		return false
	}
	if (cfg.Protocol == AVRProtocolHTTP || cfg.Protocol == AVRProtocolHTTPS) && !strings.Contains(o.Get(APIAttrURL), cfg.Protocol+"://") {
		return false
	}
	if f.BitrateMax > 0 {
		v := o.Get(APIAttrCodec) // sic, see NOTE above
		if o.Get(APIAttrBitrate) == "" {
			v = "0"
		}
		if br, ok := tryStrToInt(v); ok && br > f.BitrateMax {
			return false
		}
	}
	if len(f.NotAllowedInName) > 0 && ContainsIn(o.Get(APIAttrURL), f.NotAllowedInName) { // sic, see NOTE above
		return false
	}
	if len(f.NotAllowedInURL) > 0 && ContainsIn(o.Get(APIAttrURL), f.NotAllowedInURL) {
		return false
	}
	return true
}
