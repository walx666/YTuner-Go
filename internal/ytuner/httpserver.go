package ytuner

// Web server: routes and handlers (src/httpserver.pas).

import (
	"net/http"
	"strconv"
	"strings"
)

const (
	WebService      = "Web Service"
	PathHTTP        = "http://"
	PathSetupApp    = "setupapp"
	PathStatXMLAsp  = "statxml.asp"
	PathLoginXMLAsp = "loginxml.asp"
	PathFavXMLAsp   = "favxml.asp"
	PathNavXMLAsp   = "navxml.asp"
	PathSearchAsp   = "search.asp"
	PathRoot        = "ytuner"
	PathPlay        = "play"
	PathStation     = "station"
	PathSearch      = "search"
	PathIcon        = "icon"
	PathAbout       = "about"
	PathEmpty       = "empty"
	PathVTunerAsp   = "func/dynamOD.asp"

	PathRBGenre      = "genre"
	PathRBCountry    = "country"
	PathRBLanguage   = "language"
	PathRBPopular    = "popular"
	PathCategoryType = "categorytype"
	PathCategory     = "category"

	MsgQueryTooShort   = "Search query too short."
	MsgNoStationsFound = "No station(s) found"
	MsgForThisCategory = " for this category"
	MsgUnknownRoute    = "Unknown route"
	MsgEmptyFolder     = "This folder is intentionally empty..."

	VTunerHost = "radio567.vtuner.com"

	DefWebServerPort = 80
)

// PathRBCategories mirrors PATH_RADIOBROWSER_CATEGORIES.
var PathRBCategories = []string{PathRBGenre, PathRBCountry, PathRBLanguage, PathRBPopular}

var (
	WebServerIPAddress  string
	WebServerPort       = DefWebServerPort
	IconSize            = DefIconSize
	IconCache           = DefIconCache
	IconExtension       = ""
	HTTPCodeRedirectCfg = HTTPCodeRedirect
)

// ---------------------------------------------------------------------------
// Request model (TRequest subset)

// Fields mirrors TStrings used as QueryFields/RouteParams: ordered, names are
// case-insensitive, Get returns the first match.
type Fields struct {
	names, values []string
}

// Get mirrors TStrings.Values[name].
func (f *Fields) Get(name string) string {
	if i := f.IndexOfName(name); i >= 0 {
		return f.values[i]
	}
	return ""
}

// IndexOfName mirrors TStrings.IndexOfName.
func (f *Fields) IndexOfName(name string) int {
	for i, n := range f.names {
		if strings.EqualFold(n, name) {
			return i
		}
	}
	return -1
}

// AddPair mirrors TStrings.AddPair.
func (f *Fields) AddPair(name, value string) {
	f.names = append(f.names, name)
	f.values = append(f.values, value)
}

// ParseQuery parses a raw query string like fpweb does (HTTPDecode of names and values).
func ParseQuery(raw string) *Fields {
	f := &Fields{}
	if raw == "" {
		return f
	}
	for _, part := range strings.Split(raw, "&") {
		if part == "" {
			continue
		}
		name, value, _ := strings.Cut(part, "=")
		f.AddPair(HTTPDecode(name), HTTPDecode(value))
	}
	return f
}

// Request is the request data used by the handlers.
type Request struct {
	Method string
	URI    string // raw request URI (path + query)
	Host   string
	Query  *Fields
	Route  *Fields
}

// NewRequestForTest builds a Request from a raw URI.
func NewRequestForTest(method, uri, host string) *Request {
	path, q, _ := strings.Cut(uri, "?")
	_ = path
	return &Request{Method: method, URI: uri, Host: host, Query: ParseQuery(q), Route: &Fields{}}
}

// Response collects what a handler sends (TResponse subset).
type Response struct {
	Code        int
	ContentType string
	Headers     http.Header
	Body        []byte
	Sent        bool
	server      bool // add the Server header
}

func newResponse() *Response { return &Response{Headers: http.Header{}} }

// ---------------------------------------------------------------------------
// Router (fpweb httproute semantics: case-insensitive, ':name' = one segment,
// '*' = one or more segments, registration order wins, default route otherwise)

type routeHandler func(*Request, *Response)

type route struct {
	pattern []string
	handler routeHandler
}

// Router dispatches requests.
type Router struct {
	routes []route
	def    routeHandler
}

func splitPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// Register adds a route.
func (r *Router) Register(pattern string, h routeHandler) {
	r.routes = append(r.routes, route{pattern: splitPath(pattern), handler: h})
}

func matchSegments(pat, val []string, params *Fields) bool {
	for i, p := range pat {
		if strings.HasPrefix(p, "*") {
			// the rest of the pattern must match the tail of the path, '*' takes >= 1 segment
			rest := pat[i+1:]
			if len(val)-i < len(rest)+1 {
				return false
			}
			tail := val[len(val)-len(rest):]
			if !matchSegments(rest, tail, params) {
				return false
			}
			if name := p[1:]; name != "" {
				params.AddPair(name, strings.Join(val[i:len(val)-len(rest)], "/"))
			}
			return true
		}
		if i >= len(val) {
			return false
		}
		if strings.HasPrefix(p, ":") {
			params.AddPair(p[1:], val[i])
			continue
		}
		if !strings.EqualFold(p, val[i]) {
			return false
		}
	}
	return len(pat) == len(val)
}

// Match finds the handler for a raw path.
func (r *Router) Match(rawPath string) (routeHandler, *Fields) {
	segs := splitPath(rawPath)
	for _, rt := range r.routes {
		params := &Fields{}
		if matchSegments(rt.pattern, segs, params) {
			return rt.handler, params
		}
	}
	return r.def, &Fields{}
}

// RegisterServerRoutes mirrors RegisterServerRoutes.
func RegisterServerRoutes() *Router {
	r := &Router{def: DefaultPage}
	r.Register("/", DefaultPage)
	r.Register("/"+PathSetupApp+"/*/"+PathLoginXMLAsp, SetupAppLoginXMLPage)
	r.Register("/"+PathSetupApp+"/*/"+PathStatXMLAsp, GetStation)
	r.Register("/"+PathSetupApp+"/"+PathFavXMLAsp, BookmarkService)
	r.Register("/"+PathRoot+"/"+PathMyStations, GetMyStationsCategories)
	r.Register("/"+PathRoot+"/"+PathMyStations+"/:"+PathCategory, GetMyStationsOfCategory)
	r.Register("/"+PathRoot+"/"+PathIcon+IconExtension, GetIcon)
	r.Register("/"+PathRoot+"/"+PathRadioBrowser, GetRadioBrowserRootDirectory)
	r.Register("/"+PathRoot+"/"+PathRadioBrowser+"/:"+PathCategoryType, GetRadioBrowserCategoryType)
	r.Register("/"+PathRoot+"/"+PathRadioBrowser+"/:"+PathCategoryType+"/:"+PathCategory, GetRadioBrowserCategoryStations)
	r.Register("/"+PathRoot+"/"+PathBookmark, GetBookmarkStations)
	r.Register("/"+PathRoot+"/"+PathAbout, GetAbout)
	r.Register("/"+PathRoot+"/"+PathEmpty, GetEmpty)
	// redirect for vTuner radio station links
	r.Register("/"+PathSetupApp+"/*/"+PathVTunerAsp, VTunerRedirect)
	// routes not tested with real AVRs (see comments in httpserver.pas)
	r.Register("/"+PathRoot+"/"+PathPlay, PlayStation)
	r.Register("/"+PathRoot+"/"+PathSearch, GetSearchedStations)
	r.Register("/"+PathSetupApp+"/*/"+PathNavXMLAsp, GetRadioBrowserRootDirectory)
	r.Register("/"+PathSetupApp+"/*/"+PathSearchAsp, GetSearchedStations)
	return r
}

// ServeHTTP adapts the router to net/http.
func (r *Router) ServeHTTP(w http.ResponseWriter, hr *http.Request) {
	uri := hr.RequestURI
	if uri == "" {
		uri = hr.URL.RequestURI()
	}
	rawPath, rawQuery, _ := strings.Cut(uri, "?")
	if i := strings.Index(rawPath, "://"); i >= 0 { // absolute-form request target
		rawPath = rawPath[i+3:]
		if j := strings.IndexByte(rawPath, '/'); j >= 0 {
			rawPath = rawPath[j:]
		} else {
			rawPath = "/"
		}
	}
	h, params := r.Match(rawPath)
	req := &Request{Method: hr.Method, URI: uri, Host: hr.Host, Query: ParseQuery(rawQuery), Route: params}
	res := newResponse()
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				Logf(LtError, "Request %s %s panicked: %v", req.Method, req.URI, rec)
				if !res.Sent {
					ServerResponseString(500, CtNone, res, "")
				}
			}
		}()
		h(req, res)
	}()
	writeResponse(w, res)
}

func writeResponse(w http.ResponseWriter, res *Response) {
	if !res.Sent {
		// fpweb sends an empty 200 text/html answer if a handler sent nothing
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Length", "0")
		w.WriteHeader(http.StatusOK)
		return
	}
	for k, v := range res.Headers {
		w.Header()[k] = v
	}
	if res.server {
		w.Header().Set(HTTPHeaderServer, YTunerUserAgent+"/"+AppVersion)
	}
	w.Header().Set("Content-Type", res.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(res.Body)))
	w.WriteHeader(res.Code)
	_, _ = w.Write(res.Body)
}

// ---------------------------------------------------------------------------
// Server response routines

// ServerResponseBytes mirrors ServerResponse(..., AData: TStream).
func ServerResponseBytes(code int, ct ResponseContentType, res *Response, data []byte) {
	res.server = true
	res.Code = code
	res.ContentType = HTTPResponseContentType[ct]
	res.Body = data
	res.Sent = true
}

// ServerResponseString mirrors ServerResponse(..., AData: string): fpweb
// stores string content as TStrings, so non-empty text gets a line ending.
func ServerResponseString(code int, ct ResponseContentType, res *Response, data string) {
	if data != "" {
		data += LineEnding
	}
	ServerResponseBytes(code, ct, res, []byte(data))
}

// SendPageResponse mirrors SendPageResponse.
func SendPageResponse(code int, ct ResponseContentType, res *Response, page *VTunerPage) {
	ServerResponseBytes(code, ct, res, page.ToXML())
}

// DisplayMessage mirrors DisplayMessage.
func DisplayMessage(msg string, res *Response) {
	page := &VTunerPage{TotalItemsCount: 1}
	page.Add(&VTunerDisplay{Message: msg})
	SendPageResponse(HTTPCodeOK, CtXML, res, page)
}

func logReq(req *Request) { Logging(LtDebug, req.Method+" "+req.URI) }

// ---------------------------------------------------------------------------
// Service routines

// GetRange mirrors GetRange (0-based start; howMany -1 = all).
func GetRange(q *Fields) (start, howMany int, ok bool) {
	s, ok1 := tryStrToInt(q.Get("start"))
	start = s
	if ok1 {
		howMany, ok1 = tryStrToInt(q.Get("howmany"))
	}
	if ok1 {
		ok = true
	} else {
		var ok2 bool
		start, ok2 = tryStrToInt(q.Get("startitems"))
		if ok2 {
			howMany, ok2 = tryStrToInt(q.Get("enditems"))
		}
		if ok2 {
			ok = true
			if howMany < start {
				Logging(LtDebug, "Index of first element ("+strconv.Itoa(start)+") greater than the index of the last one ("+strconv.Itoa(howMany)+")?")
				howMany = 1
			} else {
				howMany = howMany - start + 1
			}
		} else {
			howMany = -1
		}
	}
	start--
	if start < 0 {
		start = 0
	}
	return
}

// GetPageRange mirrors GetPageRange (0-based inclusive indexes, clamped to arrayLen).
func GetPageRange(arrayLen int, q *Fields) (first, last int) {
	var ok bool
	first, ok = tryStrToInt(q.Get("start"))
	if ok {
		last, ok = tryStrToInt(q.Get("howmany"))
	}
	if ok {
		last = first + last - 1
	} else {
		first, _ = tryStrToInt(q.Get("startitems"))
		last, _ = tryStrToInt(q.Get("enditems"))
		if last < first {
			Logging(LtDebug, "Index of first element ("+strconv.Itoa(first)+") greater than the index of the last one ("+strconv.Itoa(last)+")?")
			last = first
		}
	}
	first--
	last--
	if first < 0 {
		Logging(LtDebug, "First element of page with index: \""+strconv.Itoa(first)+"\"?")
		first = 0
	}
	if last > arrayLen-1 {
		last = arrayLen - 1
	} else if last < 0 {
		Logging(LtDebug, "Last element of page with index: \""+strconv.Itoa(last)+"\"")
		last = arrayLen - 1
	}
	return
}

// GetStationInfo mirrors GetStationInfo: adds the station of the "id" query
// field to the page and returns its ID (” if not found).
func GetStationInfo(req *Request, page *VTunerPage) string {
	id := req.Query.Get(PathParamID)
	switch pasSubstring(id, 0, 2) {
	case MyStationsPrefix:
		ms := GetMyStationByID(id)
		if ms.Station.ID != "" {
			page.TotalItemsCount = 1
			page.Add(SetVTunerStationMS(ms, req))
			return ms.Station.ID
		}
	case RadioBrowserPrefix:
		avrIdx := GetAVRConfigIdx(req)
		st := GetRBStationByID(pasSubstring(id, 3, 12), avrIdx)
		if st.ID != "" {
			page.TotalItemsCount = 1
			page.Add(SetVTunerStationRB(st, req, avrIdx))
			return st.ID
		}
	}
	return ""
}

// GetRadioBrowserStations mirrors GetRadioBrowserStations.
func GetRadioBrowserStations(stations []RBStation, total int, req *Request, res *Response) {
	if total > 0 {
		avrIdx := GetAVRConfigIdx(req)
		page := &VTunerPage{TotalItemsCount: total}
		for _, st := range stations {
			page.Add(SetVTunerStationRB(st, req, avrIdx))
		}
		SendPageResponse(HTTPCodeOK, CtXML, res, page)
		return
	}
	cat := req.Route.Get(PathCategory)
	if cat != "" {
		cat = " : " + cat
	}
	Logging(LtWarning, MsgNoStationsFound+cat)
	DisplayMessage(MsgNoStationsFound, res)
}

// ---------------------------------------------------------------------------
// vTuner structures routines

func iconURL(id string) string {
	return PathHTTP + URLHost + "/" + PathRoot + "/" + PathIcon + IconExtension + "?" + PathParamID + "=" + id
}

func bookmarkURL(id string) string {
	return PathHTTP + URLHost + "/" + PathSetupApp + "/" + PathFavXMLAsp + "?" + PathParamID + "=" + id + "&" + PathFavAction + "=" + PathFavActionAdd
}

// SetVTunerStationMS mirrors SetVTunerStation(TMSStation, ...).
func SetVTunerStationMS(s MSStation, req *Request) *VTunerStation {
	return &VTunerStation{
		UID:         s.Station.ID,
		Name:        s.Station.Name,
		Description: "My favorite \"" + s.Station.Name + "\"",
		URL:         StripHttps(s.Station.URL, req),
		Icon:        iconURL(s.Station.ID),
		Genre:       s.Category,
		Bookmark:    bookmarkURL(s.Station.ID),
	}
}

// SetVTunerStationRB mirrors SetVTunerStation(TRBStation, ...).
func SetVTunerStationRB(st RBStation, req *Request, avrIdx int) *VTunerStation {
	tr := &avrConfigAt(avrIdx).Translator
	uid := RadioBrowserPrefix + "_" + strings.ToUpper(pasSubstring(strings.ReplaceAll(st.ID, "-", ""), 0, 12))
	v := &VTunerStation{UID: uid, Name: st.Name, Location: st.Country}
	if tr.Enabled {
		v.Location = Translate(v.Location, avrIdx)
	}
	if tr.ReplaceAnyDiacritics() {
		v.Name = ReplaceDiacritics(v.Name, avrIdx)
		v.Location = ReplaceDiacritics(v.Location, avrIdx)
	}
	v.Description = v.Name + " : " + st.HomePageURL
	v.URL = StripHttps(st.URL, req)
	if st.Tags == "" {
		v.Genre = req.Route.Get(PathCategory)
	} else {
		v.Genre = st.Tags
	}
	v.Mime = strings.ToUpper(st.Codec)
	v.Bitrate = st.Bitrate
	v.Icon = iconURL(uid)
	v.Bookmark = bookmarkURL(uid)
	return v
}

// SetVTunerDirectory mirrors SetVTunerDirectory.
func SetVTunerDirectory(title, destination string, itemCount, translatorIdx int) *VTunerDirectory {
	d := &VTunerDirectory{Title: title, Destination: PathHTTP + URLHost + "/" + destination, ItemCount: itemCount}
	if translatorIdx >= 0 {
		tr := &avrConfigAt(translatorIdx).Translator
		if tr.Enabled {
			d.Title = Translate(d.Title, translatorIdx)
		}
		if tr.ReplaceAnyDiacritics() {
			d.Title = ReplaceDiacritics(d.Title, translatorIdx)
		}
	}
	return d
}

// ---------------------------------------------------------------------------
// Registered route handlers

// DefaultPage mirrors DefaultPage.
func DefaultPage(req *Request, res *Response) {
	Logging(LtDebug, MsgUnknownRoute+" : "+req.Method+" "+req.URI)
	ServerResponseString(HTTPCodeNotFound, CtNone, res, "")
}

// SetupAppLoginXMLPage mirrors SetupAppLoginXMLPage (main menu / token).
func SetupAppLoginXMLPage(req *Request, res *Response) {
	logReq(req)
	if req.Query.Get(PathParamToken) == "0" {
		ServerResponseString(HTTPCodeOK, CtXML, res, strings.Replace(VTXMLEncryptedToken, "><", ">"+MyToken+"<", 1))
		return
	}
	page := &VTunerPage{}
	cfg := avrConfigAt(GetAVRConfigIdx(req))
	n := 0
	for _, mi := range cfg.MainMenuItems {
		switch mi.Identifier {
		case AVRMainMenuIdentMyStations:
			if MyStationsEnabled {
				page.Add(SetVTunerDirectory(mi.Label, PathRoot+"/"+PathMyStations, len(MyStations()), -1))
				n++
			}
		case AVRMainMenuIdentRadioBrowser:
			if RadioBrowserEnabled {
				page.Add(SetVTunerDirectory(mi.Label, PathRoot+"/"+PathRadioBrowser, 4, -1))
				n++
			}
		case AVRMainMenuIdentBookmarks:
			if BookmarkEnabled {
				page.Add(SetVTunerDirectory(mi.Label, PathRoot+"/"+PathBookmark, GetBookmarkItemsCount(req.Query.Get(PathParamMAC)), -1))
				n++
			}
		case AVRMainMenuIdentAbout:
			page.Add(SetVTunerDirectory(mi.Label, PathRoot+"/"+PathAbout, 2, -1))
			n++
		}
	}
	page.TotalItemsCount = n
	SendPageResponse(HTTPCodeOK, CtXML, res, page)
}

// GetStation mirrors GetStation (statxml.asp).
func GetStation(req *Request, res *Response) {
	logReq(req)
	id := req.Query.Get(PathParamID)
	switch pasSubstring(id, 2, 1) {
	case "_":
		page := &VTunerPage{}
		if GetStationInfo(req, page) == "" {
			Logging(LtError, MsgNoStationsFound+" with ID: "+id)
			DisplayMessage(MsgNoStationsFound, res)
		} else {
			SendPageResponse(HTTPCodeOK, CtXML, res, page)
		}
	case "B":
		data, ok := GetBookmarkStationInfo(req.Query.Get(PathParamMAC), id)
		code := HTTPCodeOK
		if !ok {
			code = HTTPCodeNotFound
		}
		ServerResponseBytes(code, CtXML, res, data)
	}
}

// BookmarkService mirrors BookmarkService (favxml.asp add/del).
func BookmarkService(req *Request, res *Response) {
	logReq(req)
	page := &VTunerPage{}
	if GetStationInfo(req, page) == "" {
		Logging(LtError, MsgNoStationsFound+" with ID: "+req.Query.Get(PathParamID))
		DisplayMessage(MsgNoStationsFound, res)
		return
	}
	st := page.Items[0].(*VTunerStation)
	st.UID = replaceIgnoreCaseFirst(st.UID, "_", "B")
	root := page.MakeXML()
	if len(root.Children) > 1 {
		SetBookmark(req.Query.Get(PathParamMAC), req.Query.Get(PathParamFav), root.Children[1])
	}
}

// GetMyStationsCategories mirrors GetMyStationsCategories.
func GetMyStationsCategories(req *Request, res *Response) {
	logReq(req)
	ms := MyStations()
	first, last := GetPageRange(len(ms), req.Query)
	if len(ms) == 0 {
		Logging(LtError, MsgNoStationsFound)
		DisplayMessage(MsgNoStationsFound, res)
		return
	}
	page := &VTunerPage{TotalItemsCount: len(ms)}
	for i := first; i <= last && i < len(ms); i++ {
		page.Add(SetVTunerDirectory(ms[i].Category, PathRoot+"/"+PathMyStations+"/"+URLEncode(ms[i].Category), len(ms[i].Stations), -1))
	}
	SendPageResponse(HTTPCodeOK, CtXML, res, page)
}

// GetMyStationsOfCategory mirrors GetMyStationsOfCategory.
func GetMyStationsOfCategory(req *Request, res *Response) {
	logReq(req)
	cat := HTTPDecode(req.Route.Get(PathCategory))
	ms := MyStations()
	idx := -1
	for i := range ms {
		if strings.EqualFold(ms[i].Category, cat) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return // Pascal sends nothing (fpweb answers an empty 200)
	}
	stations := ms[idx].Stations
	first, last := GetPageRange(len(stations), req.Query)
	if len(stations) == 0 {
		Logging(LtError, MsgNoStationsFound+" : "+cat)
		DisplayMessage(MsgNoStationsFound+" : "+cat, res)
		return
	}
	page := &VTunerPage{TotalItemsCount: len(stations)}
	for i := first; i <= last && i < len(stations); i++ {
		page.Add(SetVTunerStationMS(MSStation{Category: cat, Station: stations[i]}, req))
	}
	SendPageResponse(HTTPCodeOK, CtXML, res, page)
}

// GetRadioBrowserRootDirectory mirrors GetRadioBrowserRootDirectory.
func GetRadioBrowserRootDirectory(req *Request, res *Response) {
	logReq(req)
	avrIdx := GetAVRConfigIdx(req)
	c := GetRootItems(avrIdx)
	page := &VTunerPage{TotalItemsCount: 4}
	base := PathRoot + "/" + PathRadioBrowser + "/"
	page.Add(SetVTunerDirectory("Genres", base+PathRBGenre, c[RbctGenre], avrIdx))
	page.Add(SetVTunerDirectory("Countries", base+PathRBCountry, c[RbctCountry], avrIdx))
	page.Add(SetVTunerDirectory("Languages", base+PathRBLanguage, c[RbctLanguage], avrIdx))
	page.Add(SetVTunerDirectory("Most popular", base+PathRBPopular, c[RbctPopular], avrIdx))
	SendPageResponse(HTTPCodeOK, CtXML, res, page)
}

// GetRadioBrowserCategoryType mirrors GetRadioBrowserCategoryType.
func GetRadioBrowserCategoryType(req *Request, res *Response) {
	idx := indexStr(req.Route.Get(PathCategoryType), PathRBCategories)
	switch {
	case idx >= 0 && idx <= 2:
		logReq(req)
		cats, total := GetCategoryItemsReq(RBAllCategoryType(idx), req)
		if total <= 0 {
			Logging(LtError, MsgNoStationsFound+MsgForThisCategory)
			DisplayMessage(MsgNoStationsFound+MsgForThisCategory, res)
			return
		}
		avrIdx := GetAVRConfigIdx(req)
		page := &VTunerPage{TotalItemsCount: total}
		for _, c := range cats {
			page.Add(SetVTunerDirectory(c.Name, PathRoot+"/"+PathRadioBrowser+"/"+PathRBCategories[idx]+"/"+URLEncode(c.Name), c.MaxCount, avrIdx))
		}
		SendPageResponse(HTTPCodeOK, CtXML, res, page)
	case idx == 3:
		GetRadioBrowserCategoryStations(req, res)
	default:
		Logging(LtError, "Radiobrowser category type not supported ?!")
	}
}

// GetRadioBrowserCategoryStations mirrors GetRadioBrowserCategoryStations.
func GetRadioBrowserCategoryStations(req *Request, res *Response) {
	logReq(req)
	idx := indexStr(req.Route.Get(PathCategoryType), PathRBCategories)
	if idx < 0 || idx > 3 {
		ServerResponseString(HTTPCodeNotFound, CtNone, res, "")
		return
	}
	stations, total := GetStationsByCategoryReq(RBAllCategoryType(idx), req.Route.Get(PathCategory), req)
	GetRadioBrowserStations(stations, total, req, res)
}

// GetBookmarkStations mirrors GetBookmarkStations.
func GetBookmarkStations(req *Request, res *Response) {
	logReq(req)
	first, last := GetPageRange(GetBookmarkItemsCount(req.Query.Get(PathParamMAC)), req.Query)
	data, ok := GetBookmark(req, first, last)
	code := HTTPCodeOK
	if !ok {
		code = HTTPCodeNotFound
	}
	ServerResponseBytes(code, CtXML, res, data)
}

// PlayStation mirrors PlayStation (redirect to the stream URL).
func PlayStation(req *Request, res *Response) {
	logReq(req)
	id := req.Query.Get(PathParamID)
	u := ""
	switch pasSubstring(id, 0, 2) {
	case MyStationsPrefix:
		if ms := GetMyStationByID(id); ms.Station.ID != "" {
			u = StripHttps(ms.Station.URL, req)
		}
	case RadioBrowserPrefix:
		u = GetRBStationByID(pasSubstring(id, 3, 12), GetAVRConfigIdx(req)).URL
	}
	code := HTTPCodeNotFound
	if u != "" {
		res.Headers.Set(HTTPHeaderLocation, u)
		code = HTTPCodeRedirectCfg
	}
	ServerResponseString(code, CtNone, res, "")
}

// GetSearchedStations mirrors GetSearchedStations (search.asp; sSearchtype=3 = play).
func GetSearchedStations(req *Request, res *Response) {
	q := req.Query
	if q.Get(PathParamSSearchType) == "3" {
		search := q.Get(PathParamSearch)
		switch pasSubstring(search, 0, 2) {
		case MyStationsPrefix, RadioBrowserPrefix:
			q.AddPair(PathParamID, search)
		default:
			deviceID := "?"
			if q.IndexOfName(PathParamMAC) >= 0 {
				deviceID = q.Get(PathParamMAC)
			}
			Logging(LtDebug, "Device : "+deviceID+" -> "+req.Host)
			if deviceID == "?" {
				deviceID = req.Host
			}
			ms := MyStations()
			if fileExists(pathJoin(ConfigPath, deviceID+".xml")) {
				if q.IndexOfName(PathParamMAC) < 0 {
					q.AddPair(PathParamMAC, deviceID)
				}
				q.AddPair(PathParamID, "UNB"+search)
			} else if MyStationsEnabled && len(ms) > 0 && len(ms[0].Stations) > 0 {
				q.AddPair(PathParamID, ms[0].Stations[0].ID)
			} else {
				Logging(LtDebug, MsgFirstStationNeeded)
			}
		}
		GetStation(req, res)
		return
	}
	logReq(req)
	search := q.Get(PathParamSearch)
	if len(search) < 3 {
		DisplayMessage(MsgQueryTooShort, res)
		return
	}
	stations, total := GetStationsByCategoryReq(RbctSearch, search, req)
	GetRadioBrowserStations(stations, total, req, res)
}

// GetAbout mirrors GetAbout.
func GetAbout(req *Request, res *Response) {
	logReq(req)
	page := &VTunerPage{TotalItemsCount: 2}
	page.Add(SetVTunerDirectory("Welcome to "+AppName+" v"+AppVersion, PathRoot+"/"+PathEmpty, 1, -1))
	page.Add(SetVTunerDirectory(AppCopyright, PathRoot+"/"+PathEmpty, 1, -1))
	SendPageResponse(HTTPCodeOK, CtXML, res, page)
}

// GetEmpty mirrors GetEmpty.
func GetEmpty(req *Request, res *Response) {
	logReq(req)
	DisplayMessage(MsgEmptyFolder, res)
}

// vtunerHTTPClient does not follow redirects (we want the Location header).
var vtunerHTTPClient = &http.Client{
	Timeout:       15e9,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// VTunerRedirect mirrors VTunerRedirect: resolves obsolete vTuner station
// links via the real vTuner host or falls back to the first own station.
func VTunerRedirect(req *Request, res *Response) {
	logReq(req)
	code := HTTPCodeNotFound
	hr, err := http.NewRequest(http.MethodGet, "http://"+VTunerHost+req.URI, nil)
	status := 0
	location := ""
	if err == nil {
		for k, v := range map[string]string{
			HTTPHeaderAccept:                  WebBrowserAccept,
			HTTPHeaderAcceptLanguage:          WebBrowserAcceptLanguage,
			HTTPHeaderCacheControl:            WebBrowserCacheControl,
			HTTPHeaderPragma:                  WebBrowserPragma,
			HTTPHeaderUpgradeInsecureRequests: "1",
			HTTPHeaderUserAgent:               WebBrowserUserAgent,
		} {
			hr.Header.Set(k, v)
		}
		if resp, err := vtunerHTTPClient.Do(hr); err == nil {
			status = resp.StatusCode
			location = strings.TrimSpace(resp.Header.Get("Location"))
			resp.Body.Close()
		}
	}
	switch status {
	case 301, 302, 303, 307, 308:
		if location != "" {
			code = HTTPCodeRedirectCfg
			res.Headers.Set(HTTPHeaderLocation, location)
			Logging(LtDebug, MsgVTunerErrorLink1+location)
		}
	default:
		Logging(LtDebug, MsgVTunerErrorLink2)
		Logging(LtDebug, MsgFirstStationNeeded)
		ms := MyStations()
		if MyStationsEnabled && len(ms) > 0 && len(ms[0].Stations) > 0 {
			code = HTTPCodeRedirectCfg
			res.Headers.Set(HTTPHeaderLocation, StripHttps(ms[0].Stations[0].URL, req))
		} else {
			Logging(LtDebug, MsgFirstStationNeeded)
		}
	}
	Logging(LtDebug, MsgRedirectSupportNeeded)
	ServerResponseString(code, CtNone, res, "")
}
