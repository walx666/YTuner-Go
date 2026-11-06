package ytuner

// Application start-up and configuration (src/ytuner.pas).

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ---------------------------------------------------------------------------
// Timers (TThreadTimer replacement)

type ticker struct {
	name string
	stop chan struct{}
	busy sync.Mutex
}

func startTicker(name string, interval time.Duration, fn func()) *ticker {
	t := &ticker{name: name, stop: make(chan struct{})}
	go func() {
		tk := time.NewTicker(interval)
		defer tk.Stop()
		for {
			select {
			case <-t.stop:
				return
			case <-tk.C:
				if t.busy.TryLock() { // skip if the previous run is still processing
					go func() {
						defer t.busy.Unlock()
						fn()
					}()
				}
			}
		}
	}()
	return t
}

func (t *ticker) Stop() {
	if t != nil {
		close(t.stop)
	}
}

const (
	RBThread = "RBThread"
	MSThread = "MSThread"
	GTThread = "GTThread"
)

var rbTicker, msTicker *ticker

// ---------------------------------------------------------------------------
// Configuration

func folderLocation(ini *IniFile, ident, sub string) string {
	if !ini.ValueExists(IniConfiguration, ident) {
		ini.WriteString(IniConfiguration, ident, DefaultString)
	}
	loc := ini.ReadString(IniConfiguration, ident, DefaultString)
	if loc == DefaultString {
		loc = MyAppPath
	}
	return filepath.Join(loc, sub)
}

// ReadINIConfiguration mirrors ReadINIConfiguration (ytuner.ini next to the
// executable; missing options are added with their defaults).
func ReadINIConfiguration() {
	ini := NewIniFile(pathJoin(MyAppPath, "ytuner.ini"))
	defer func() {
		if err := ini.Save(); err != nil {
			Logging(LtError, "ytuner.ini"+MsgFileSaveError+" ("+err.Error()+")")
		}
	}()

	if !ini.ValueExists(IniConfiguration, IniMessageInfoLevel) {
		ini.WriteInteger(IniConfiguration, IniMessageInfoLevel, 4)
	}
	if lvl := ini.ReadInteger(IniConfiguration, IniMessageInfoLevel, 4); lvl >= int(LtNone) && lvl <= int(LtDebug) {
		LogLevel = LogType(lvl)
	} else {
		LogLevel = LtError
	}
	if !ini.ValueExists(IniConfiguration, IniIniVersion) {
		Logging(LtWarning, MsgIniWarning1)
		Logging(LtWarning, MsgIniWarning3)
	} else if ini.ReadString(IniConfiguration, IniIniVersion, "") != IniVersion {
		Logging(LtWarning, MsgIniWarning2)
		Logging(LtWarning, MsgIniWarning3)
	}
	ini.WriteString(IniConfiguration, IniIniVersion, IniVersion)

	str := func(section, ident, def string) string {
		if !ini.ValueExists(section, ident) {
			ini.WriteString(section, ident, def)
		}
		return ini.ReadString(section, ident, def)
	}
	integer := func(section, ident string, def int) int {
		if !ini.ValueExists(section, ident) {
			ini.WriteInteger(section, ident, def)
		}
		return ini.ReadInteger(section, ident, def)
	}
	boolean := func(section, ident string, defStr string, def bool) bool {
		if !ini.ValueExists(section, ident) {
			ini.WriteString(section, ident, defStr)
		}
		return ini.ReadBool(section, ident, def)
	}

	str(IniConfiguration, IniIPAddress, DefaultString)
	MyIPAddress = GetLocalIP(ini.ReadString(IniConfiguration, IniIPAddress, MyIPAddress))

	str(IniConfiguration, IniActAsHost, DefaultString)
	URLHost = strings.ReplaceAll(ini.ReadString(IniConfiguration, IniActAsHost, MyIPAddress), DefaultString, MyIPAddress)
	if strings.TrimSpace(URLHost) == "" {
		URLHost = MyIPAddress
	}

	UseSSL = boolean(IniConfiguration, IniUseSSL, "1", true)
	HTTPCodeRedirectCfg = integer(IniConfiguration, IniRedirectHTTPCode, HTTPCodeRedirect)
	IconSize = integer(IniConfiguration, IniIconSize, DefIconSize)
	IconCache = boolean(IniConfiguration, IniIconCache, "1", DefIconCache)
	IconExtension = strings.ToLower(str(IniConfiguration, IniIconExtension, ""))
	if IconExtension != "" {
		IconExtension = "." + pasSubstring(IconExtension, 0, 3)
	}
	token := str(IniConfiguration, IniMyToken, VTToken)
	CommonAVRini = boolean(IniConfiguration, IniCommonAVRIni, "1", DefCommonAVRIni)

	CachePath = folderLocation(ini, IniCacheFolderLocation, PathCache)
	ConfigPath = folderLocation(ini, IniConfigFolderLocation, PathConfig)
	DBPath = folderLocation(ini, IniDBFolderLocation, PathDB)

	DBLibFile = str(IniConfiguration, IniDBLibFile, DefaultString) // unused: SQLite is built in

	if regexp.MustCompile(`^[A-F a-f 0-9]{16,16}$`).MatchString(token) {
		MyToken = token
	}

	MyStationsEnabled = boolean(IniMyStations, IniEnable, "1", true)
	MyStationsFileName = str(IniMyStations, IniMyStationsFile, MyStationsFileNameDefault)
	MyStationsAutoRefreshPeriod = integer(IniMyStations, IniMyStationsAutoRefreshPeriod, 0)

	RadioBrowserEnabled = boolean(IniRadioBrowser, IniEnable, "1", true)
	RBAPIURL = str(IniRadioBrowser, IniRBAPIURL, RBAPIURL)
	RBPopularAndSearchStationsLimit = integer(IniRadioBrowser, IniRBPopularAndSearchStationsLimit, DefRBPopularAndSearchStationsLimit)
	RBMinStationsPerCategory = integer(IniRadioBrowser, IniRBMinStationsPerCategory, DefRBMinStationsPerCategory)

	str(IniRadioBrowser, IniRBUUIDsCacheTTL, "0")
	RBUUIDsCacheTTL = ini.ReadInteger(IniRadioBrowser, IniRBUUIDsCacheTTL, RBUUIDsCacheTTL)
	if RBUUIDsCacheTTL < 0 {
		ini.WriteString(IniRadioBrowser, IniRBUUIDsCacheTTL, "0")
		RBUUIDsCacheTTL = 0
	}
	RBUUIDsCacheAutoRefresh = boolean(IniRadioBrowser, IniRBUUIDsCacheAutoRefresh, "0", false)

	str(IniRadioBrowser, IniRBCacheTTL, "0")
	RBCacheTTL = ini.ReadInteger(IniRadioBrowser, IniRBCacheTTL, RBCacheTTL)
	if RBCacheTTL < 0 {
		ini.WriteString(IniRadioBrowser, IniRBCacheTTL, "0")
		RBCacheTTL = 0
	}

	ct := str(IniRadioBrowser, IniRBCacheType, "catFile")
	if ct != "" {
		if v, ok := ParseCacheType(ct); ok {
			RBCacheType = v
		} else {
			Logging(LtError, "Unexpected value of RBStationsCacheType: \""+ct+"\". Default \"catFile\" will be used")
			RBCacheType = CatFile
		}
	} else {
		Logging(LtWarning, "Radio-browser.info cache is disabled.")
		RBCacheType = CatNone
	}

	BookmarkEnabled = boolean(IniBookmark, IniEnable, "1", true)
	CommonBookmark = boolean(IniBookmark, IniCommonBookmark, "1", true)
	BookmarkStationsLimit = integer(IniBookmark, IniBookmarkStationsLimit, DefBookmarkStationsLimit)

	WebServerIPAddress = GetLocalIP(strings.ReplaceAll(str(IniWebServer, IniWebServerIPAddress, DefaultString), DefaultString, MyIPAddress))
	WebServerPort = integer(IniWebServer, IniWebServerPort, DefWebServerPort)

	DNSServerEnabled = boolean(IniDNSServer, IniEnable, "1", true)
	DNSServerIPAddress = GetLocalIP(strings.ReplaceAll(str(IniDNSServer, IniDNSServerIPAddress, DefaultString), DefaultString, MyIPAddress))
	DNSServerPort = integer(IniDNSServer, IniDNSServerPort, DefDNSServerPort)
	InterceptDNs = str(IniDNSServer, IniInterceptDNs, DefInterceptDNs)
	DNSServers = str(IniDNSServer, IniDNSServers, DefDNSServers)

	MaintenanceServiceEnabled = boolean(IniMaintenanceServer, IniEnable, "0", false)
	MaintenanceServerIPAddress = GetLocalIP(strings.ReplaceAll(str(IniMaintenanceServer, IniMaintenanceServerIPAddress, DefMaintenanceServerIPAddress), DefaultString, MaintenanceServerIPAddress))
	MaintenanceServerPort = integer(IniMaintenanceServer, IniMaintenanceServerPort, DefMaintenanceServerPort)
}

// MoveConfigFiles mirrors MoveConfigFiles: moves stations/bookmark files from
// the application directory to the config directory.
func MoveConfigFiles() {
	seen := map[string]bool{}
	var files []string
	for _, pat := range []string{MyStationsFileName, "stations.*", "*.xml"} {
		m, _ := filepath.Glob(filepath.Join(MyAppPath, pat))
		for _, f := range m {
			if st, err := os.Stat(f); err == nil && !st.IsDir() && !seen[f] {
				seen[f] = true
				files = append(files, f)
			}
		}
	}
	if len(files) == 0 {
		return
	}
	Logging(LtInfo, "Moving files to config directory ..")
	for _, f := range files {
		dst := filepath.Join(ConfigPath, filepath.Base(f))
		data, err := os.ReadFile(f)
		if err == nil {
			err = os.WriteFile(dst, data, 0o644)
		}
		if err == nil {
			_ = os.Remove(f)
		}
	}
}

// ---------------------------------------------------------------------------
// Run

// Run starts YTuner and blocks until it is shut down. Returns the exit code.
func Run() int {
	MyAppPath = GetMyAppPath()
	fmt.Println(AppName + " v" + AppVersion + " (" + AppGoVersion + ") " + AppCopyright)
	ReadINIConfiguration()

	_ = os.MkdirAll(CachePath, 0o755)
	_ = os.MkdirAll(ConfigPath, 0o755)
	if dirExists(ConfigPath) {
		MoveConfigFiles()
	}
	ReadAVRINIConfiguration(AVRAVR)

	Logging(LtInfo, "Starting services..")

	if MyStationsEnabled {
		CheckMyStations()
	}

	RemoveOldRBCacheFiles()

	if RadioBrowserEnabled {
		if RBCacheType.IsDB() {
			if err := os.MkdirAll(DBPath, 0o755); err != nil {
				Logging(LtWarning, JoinMsg(MsgRBDBCannot, MsgRBDBCreate, MsgRBDBDB, MsgDirectory, DBPath))
				RBCacheType = CatFile
			} else if CheckSQLite3LibVer() && DBRBPrepare() {
				if !DBRBCheckAVRView(0) {
					RBCacheType = CatFile
				} else if RBCacheTTL > 0 {
					rbTicker = startTicker(RBThread, time.Duration(RBCacheTTL)*time.Hour, DBRBRefresh)
				}
			} else {
				Logging(LtWarning, JoinMsg(MsgRBDBDB, MsgRBDBPreparing, MsgError))
				RBCacheType = CatFile
			}
			if RBCacheType == CatFile {
				Logging(LtWarning, JoinMsg(MsgChanging, MsgCache, "to \"catFile\""))
			}
		}
		if RBCacheType == CatNone || RBCacheType.IsFileOrMemStr() {
			if !LoadRBStationsUUIDs() {
				go GetRBStationsUUIDs()
			}
			if RBCacheType == CatFile {
				LoadRBCacheFilesInfo(0)
			}
			if RBUUIDsCacheAutoRefresh && RBUUIDsCacheTTL > 0 {
				rbTicker = startTicker(RBThread, time.Duration(RBUUIDsCacheTTL)*time.Hour, GetRBStationsUUIDs)
			}
		}
	}

	if MyStationsEnabled && MyStationsAutoRefreshPeriod > 0 {
		msTicker = startTicker(MSThread, time.Duration(MyStationsAutoRefreshPeriod)*time.Minute, CheckMyStations)
	}

	if DNSServerEnabled && StartDNSServer() {
		Logging(LtInfo, DNSService+": listening on: "+DNSServerIPAddress+":"+strconv.Itoa(DNSServerPort))
	}

	stop := make(chan struct{})
	var stopOnce sync.Once
	shutdown := func() { stopOnce.Do(func() { close(stop) }) }

	var maint *http.Server
	if MaintenanceServiceEnabled {
		maint = StartMaintenanceServer(shutdown)
	}

	router := RegisterServerRoutes()
	addr := net.JoinHostPort(WebServerIPAddress, strconv.Itoa(WebServerPort))
	srv := &http.Server{Addr: addr, Handler: router, ReadHeaderTimeout: 30 * time.Second}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		Logging(LtError, WebService+": "+err.Error())
		return 1
	}
	Logging(LtInfo, WebService+": listening on: "+addr)
	if URLHost != "" && URLHost != WebServerIPAddress {
		Logging(LtInfo, "AVRs HTTP requests host target: "+URLHost)
	} else if MyIPAddress != WebServerIPAddress {
		Logging(LtInfo, "AVRs HTTP requests IP target: "+MyIPAddress)
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	exitCode := 0
	select {
	case <-stop:
	case s := <-sig:
		Logging(LtInfo, "Signal received: "+s.String())
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			Logging(LtError, WebService+": "+err.Error())
			exitCode = 1
		}
	}
	serviceDown(srv, maint)
	return exitCode
}

// serviceDown mirrors YTunerServiceDown.
func serviceDown(srv, maint *http.Server) {
	Logging(LtInfo, AppName+": "+MsgShuttingDown+". Please wait..")
	if RadioBrowserEnabled && RBCacheType.IsDB() && mainDB() != nil {
		Logging(LtInfo, JoinMsg(MsgRBDBClosing, MsgRBDBDB, MsgRBDBConnection))
		DBRBClose()
	}
	if rbTicker != nil {
		Logging(LtInfo, RBThread+": "+MsgShuttingDown)
		rbTicker.Stop()
		Logging(LtInfo, RBThread+": "+MsgDone)
	}
	if msTicker != nil {
		Logging(LtInfo, MSThread+": "+MsgShuttingDown)
		msTicker.Stop()
		Logging(LtInfo, MSThread+": "+MsgDone)
	}
	StopTranslator()
	if DNSServerEnabled {
		Logging(LtInfo, DNSService+": "+MsgShuttingDown)
		StopDNSServer()
		Logging(LtInfo, DNSService+": "+MsgDone)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if maint != nil {
		Logging(LtInfo, MaintenanceService+": "+MsgShuttingDown)
		_ = maint.Shutdown(ctx)
		Logging(LtInfo, MaintenanceService+": "+MsgDone)
	}
	Logging(LtInfo, WebService+": "+MsgShuttingDown)
	_ = srv.Shutdown(ctx)
	Logging(LtInfo, WebService+": "+MsgDone)
}
