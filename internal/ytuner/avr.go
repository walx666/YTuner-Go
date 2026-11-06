package ytuner

// AVR configuration (src/avr.pas).

import (
	"strings"
	"sync"
)

// RBFilter mirrors TRBFilter.
type RBFilter struct {
	AllowedTags, AllowedCountries, AllowedLanguages, AllowedCodecs      []string
	BitrateMax                                                          int
	NotAllowedTags, NotAllowedCodecs, NotAllowedInName, NotAllowedInURL []string
}

// RBSort mirrors TRBSort.
type RBSort struct {
	Order   string
	Reverse bool
}

// MainMenuItem mirrors TMainMenuItem.
type MainMenuItem struct {
	Identifier string
	Label      string
}

// Translator mirrors TTranslator (the lock flags are replaced by a mutex).
type Translator struct {
	ReplaceUTF8Latin1Supplement bool
	ReplaceUTF8Latin1ExtendedA  bool
	ReplaceUTF8Latin1ExtendedB  bool
	Enabled                     bool
	AutoTranslate               bool
	LanguageCode                string

	mu          sync.Mutex
	ToTranslate string
	Items       *NameValueList
	fileMu      sync.Mutex
}

// ReplaceAnyDiacritics reports whether any diacritics replacement is enabled.
func (t *Translator) ReplaceAnyDiacritics() bool {
	return t.ReplaceUTF8Latin1Supplement || t.ReplaceUTF8Latin1ExtendedA || t.ReplaceUTF8Latin1ExtendedB
}

// AVRConfig mirrors TAVRConfig.
type AVRConfig struct {
	CRC32         uint32
	Protocol      string
	Translator    Translator
	MainMenuItems []MainMenuItem
	RBFilter      RBFilter
	RBSort        RBSort
}

const (
	AVRIniVersion                    = "1.0.2"
	AVRIniSectionConfiguration       = "Configuration"
	AVRIniProtocol                   = "Protocol"
	AVRIniIniVersion                 = "INIVersion"
	AVRIniSectionRadioBrowserFilter  = "RadioBrowser Filtering"
	AVRIniAllowedTags                = "AllowedTags"
	AVRIniNotAllowedTags             = "NotAllowedTags"
	AVRIniAllowedCountries           = "AllowedCountries"
	AVRIniAllowedLanguages           = "AllowedLanguages"
	AVRIniAllowedCodecs              = "AllowedCodecs"
	AVRIniNotAllowedCodecs           = "NotAllowedCodecs"
	AVRIniBitrateMax                 = "BitrateMax"
	AVRIniNotAllowedInName           = "NotAllowedInName"
	AVRIniNotAllowedInURL            = "NotAllowedInURL"
	AVRIniSectionRadioBrowserSorting = "RadioBrowser Sorting"
	AVRIniOrder                      = "Order"
	AVRIniReverse                    = "Reverse"
	AVRIniSectionMainMenu            = "MainMenu Items"
	AVRIniMainMenuIdentifiers        = "IdentifiersList"
	AVRIniMainMenuLabels             = "LabelsList"
	AVRIniSectionTranslator          = "Translator"
	AVRIniEnable                     = "Enable"
	AVRIniAutoTranslate              = "Auto"
	AVRIniLanguageCode               = "LanguageCode"
	AVRIniReplaceLatin1Supplement    = "ReplaceUTF8Latin1Supplement"
	AVRIniReplaceLatin1ExtendedA     = "ReplaceUTF8Latin1ExtendedA"
	AVRIniReplaceLatin1ExtendedB     = "ReplaceUTF8Latin1ExtendedB"

	AVRFilterEmpty = "{empty}"

	AVRAVR                        = "avr"
	AVRMAC                        = "mac"
	AVRProtocolHTTP               = "http"
	AVRProtocolHTTPS              = "https"
	AVRProtocolAllAsHTTP          = "all-as-http"
	AVRMainMenuIdentBookmarks     = "bookmarks"
	AVRMainMenuIdentMyStations    = "mystations"
	AVRMainMenuIdentRadioBrowser  = "radiobrowser"
	AVRMainMenuIdentAbout         = "about"
	AVRMainMenuIdentifiers        = AVRMainMenuIdentAbout + ";" + AVRMainMenuIdentBookmarks + ";" + AVRMainMenuIdentMyStations + ";" + AVRMainMenuIdentRadioBrowser
	AVRMainMenuLabels             = "*** YTuner ***;Bookmarks;My Stations;Radio Browser"
	MsgAVRMainMenuIncorrectValues = "Incorrect values in avr.ini [MainMenu Items] group!"
	MsgAVRMainMenuRestoringValues = "Restoring default values..."
)

var (
	avrMu         sync.RWMutex
	avrMACs       []string
	avrConfigs    []*AVRConfig
	CommonAVRini  = true
	avrRegisterMu sync.Mutex // serializes ReadAVRINIConfiguration of new AVRs
)

// AVRMAC returns the MAC (config name) of an AVR config index.
func avrMACAt(idx int) string {
	avrMu.RLock()
	defer avrMu.RUnlock()
	if idx < 0 || idx >= len(avrMACs) {
		return ""
	}
	return avrMACs[idx]
}

// avrConfigAt returns the AVR config of an index (index 0 if out of range).
func avrConfigAt(idx int) *AVRConfig {
	avrMu.RLock()
	defer avrMu.RUnlock()
	if idx < 0 || idx >= len(avrConfigs) {
		if len(avrConfigs) == 0 {
			return &AVRConfig{Translator: Translator{Items: NewNameValueList()}}
		}
		return avrConfigs[0]
	}
	return avrConfigs[idx]
}

func avrCount() int {
	avrMu.RLock()
	defer avrMu.RUnlock()
	return len(avrConfigs)
}

func avrIndexOfMAC(mac string) int {
	avrMu.RLock()
	defer avrMu.RUnlock()
	for i, m := range avrMACs {
		if m == mac { // IndexStr is case-sensitive
			return i
		}
	}
	return -1
}

// avrViewName returns the DB view name of an AVR ('avr'+MAC+'_'+CRC32).
func avrViewName(idx int) string {
	return AVRAVR + avrMACAt(idx) + "_" + uintToStr(avrConfigAt(idx).CRC32)
}

// StripHttps mirrors StripHttps.
func StripHttps(u string, req *Request) string {
	idx := 0
	if !CommonAVRini && strings.TrimSpace(req.Query.Get(AVRMAC)) != "" {
		idx = avrIndexOfMAC(strings.TrimSpace(req.Query.Get(AVRMAC)))
		if idx < 0 {
			idx = GetAVRConfigIdx(req)
		}
	}
	if idx >= 0 && avrConfigAt(idx).Protocol == AVRProtocolAllAsHTTP {
		// StringReplace with [rfIgnoreCase] (first occurrence only)
		if i := strings.Index(strings.ToLower(u), AVRProtocolHTTPS+"://"); i >= 0 {
			return u[:i] + AVRProtocolHTTP + "://" + u[i+len(AVRProtocolHTTPS)+3:]
		}
	}
	return u
}

// GetAVRConfigIdx mirrors GetAVRConfigIdx.
func GetAVRConfigIdx(req *Request) int {
	if CommonAVRini {
		return 0
	}
	mac := strings.TrimSpace(req.Query.Get(AVRMAC))
	if mac == "" {
		return 0
	}
	if idx := avrIndexOfMAC(mac); idx >= 0 {
		return idx
	}
	avrRegisterMu.Lock()
	defer avrRegisterMu.Unlock()
	if idx := avrIndexOfMAC(mac); idx >= 0 { // registered meanwhile
		return idx
	}
	Logging(LtInfo, "New AVR connected ("+mac+")")
	if !fileExists(pathJoin(ConfigPath, mac+".ini")) {
		Logging(LtInfo, "Preparing new config ini file ("+mac+".ini)..")
	}
	idx := ReadAVRINIConfiguration(mac)
	if idx > 0 {
		switch RBCacheType {
		case CatFile:
			LoadRBCacheFilesInfo(idx)
		case CatDB, CatMemDB, CatPermMemDB:
			DBRBCheckAVRView(idx)
		}
	}
	if idx < 0 {
		idx = 0
	}
	return idx
}

func rbFilteringReadStrings(ini *IniFile, ident string) []string {
	if !ini.ValueExists(AVRIniSectionRadioBrowserFilter, ident) {
		ini.WriteString(AVRIniSectionRadioBrowserFilter, ident, "")
	}
	v := StripChars(ini.ReadString(AVRIniSectionRadioBrowserFilter, ident, ""), RBFilterStripChars)
	var res []string
	for _, s := range strings.Split(v, ";") {
		if s != "" { // TStringSplitOptions.ExcludeEmpty
			res = append(res, s)
		}
	}
	return res
}

// ReadAVRINIConfiguration mirrors ReadAVRINIConfiguration and returns the new index.
func ReadAVRINIConfiguration(mac string) int {
	cfg := &AVRConfig{}
	fileName := pathJoin(ConfigPath, mac+".ini")
	ini := NewIniFile(fileName)

	if !ini.ValueExists(AVRIniSectionConfiguration, AVRIniIniVersion) {
		ini.WriteString(AVRIniSectionConfiguration, AVRIniIniVersion, AVRIniVersion)
	}
	if ini.ReadString(AVRIniSectionConfiguration, AVRIniIniVersion, "") != AVRIniVersion {
		Logging(LtWarning, "Your "+mac+".ini file is outdated! Some features may not work properly!")
		Logging(LtWarning, "Your "+mac+".ini file will be updated with new options right now. Check https://github.com/coffeegreg/YTuner/cfg for the description of appropriate avr.ini file for your version of YTuner")
		ini.WriteString(AVRIniSectionConfiguration, AVRIniIniVersion, AVRIniVersion)
	}

	if !ini.ValueExists(AVRIniSectionConfiguration, AVRIniProtocol) {
		ini.WriteString(AVRIniSectionConfiguration, AVRIniProtocol, AVRProtocolAllAsHTTP)
	}
	cfg.Protocol = strings.ToLower(ini.ReadString(AVRIniSectionConfiguration, AVRIniProtocol, AVRProtocolAllAsHTTP))

	s1 := ini.ReadString(AVRIniSectionMainMenu, AVRIniMainMenuIdentifiers, "")
	s2 := ini.ReadString(AVRIniSectionMainMenu, AVRIniMainMenuLabels, "")
	if strings.Count(s1, ";") != strings.Count(s2, ";") || s1 == "" || s2 == "" {
		Logging(LtWarning, MsgAVRMainMenuIncorrectValues+" "+MsgAVRMainMenuRestoringValues)
		ini.WriteString(AVRIniSectionMainMenu, AVRIniMainMenuIdentifiers, AVRMainMenuIdentifiers)
		ini.WriteString(AVRIniSectionMainMenu, AVRIniMainMenuLabels, AVRMainMenuLabels)
		s1, s2 = AVRMainMenuIdentifiers, AVRMainMenuLabels
	}
	ids, labels := strings.Split(s1, ";"), strings.Split(s2, ";")
	for i := range ids {
		cfg.MainMenuItems = append(cfg.MainMenuItems, MainMenuItem{Identifier: ids[i], Label: labels[i]})
	}

	f := &cfg.RBFilter
	f.AllowedTags = rbFilteringReadStrings(ini, AVRIniAllowedTags)
	f.NotAllowedTags = rbFilteringReadStrings(ini, AVRIniNotAllowedTags)
	f.AllowedCountries = rbFilteringReadStrings(ini, AVRIniAllowedCountries)
	f.AllowedLanguages = rbFilteringReadStrings(ini, AVRIniAllowedLanguages)
	f.AllowedCodecs = rbFilteringReadStrings(ini, AVRIniAllowedCodecs)
	f.NotAllowedCodecs = rbFilteringReadStrings(ini, AVRIniNotAllowedCodecs)
	f.NotAllowedInName = rbFilteringReadStrings(ini, AVRIniNotAllowedInName)
	f.NotAllowedInURL = rbFilteringReadStrings(ini, AVRIniNotAllowedInURL)
	if !ini.ValueExists(AVRIniSectionRadioBrowserFilter, AVRIniBitrateMax) {
		ini.WriteString(AVRIniSectionRadioBrowserFilter, AVRIniBitrateMax, "")
	}
	f.BitrateMax = ini.ReadInteger(AVRIniSectionRadioBrowserFilter, AVRIniBitrateMax, 0)

	if !ini.ValueExists(AVRIniSectionRadioBrowserSorting, AVRIniOrder) {
		ini.WriteString(AVRIniSectionRadioBrowserSorting, AVRIniOrder, "name")
	}
	cfg.RBSort.Order = ini.ReadString(AVRIniSectionRadioBrowserSorting, AVRIniOrder, "name")
	if !ini.ValueExists(AVRIniSectionRadioBrowserSorting, AVRIniReverse) {
		ini.WriteString(AVRIniSectionRadioBrowserSorting, AVRIniReverse, "0")
	}
	cfg.RBSort.Reverse = ini.ReadBool(AVRIniSectionRadioBrowserSorting, AVRIniReverse, false)

	readTrBool := func(ident string) bool {
		if !ini.ValueExists(AVRIniSectionTranslator, ident) {
			ini.WriteString(AVRIniSectionTranslator, ident, "0")
		}
		return ini.ReadBool(AVRIniSectionTranslator, ident, false)
	}
	t := &cfg.Translator
	t.Enabled = readTrBool(AVRIniEnable)
	t.AutoTranslate = readTrBool(AVRIniAutoTranslate)
	if !ini.ValueExists(AVRIniSectionTranslator, AVRIniLanguageCode) {
		ini.WriteString(AVRIniSectionTranslator, AVRIniLanguageCode, "")
	}
	t.LanguageCode = strings.ToLower(ini.ReadString(AVRIniSectionTranslator, AVRIniLanguageCode, ""))
	t.ReplaceUTF8Latin1Supplement = readTrBool(AVRIniReplaceLatin1Supplement)
	t.ReplaceUTF8Latin1ExtendedA = readTrBool(AVRIniReplaceLatin1ExtendedA)
	t.ReplaceUTF8Latin1ExtendedB = readTrBool(AVRIniReplaceLatin1ExtendedB)

	if err := ini.Save(); err != nil {
		Logging(LtError, mac+".ini"+MsgFileSaveError+" ("+err.Error()+")")
	}
	if crc, err := CalcFileCRC32(fileName); err == nil {
		cfg.CRC32 = crc
	}
	t.Items = NewNameValueList()

	avrMu.Lock()
	idx := len(avrConfigs)
	avrMACs = append(avrMACs, mac)
	avrConfigs = append(avrConfigs, cfg)
	avrMu.Unlock()

	if t.Enabled {
		LoadTranslator(idx, mac)
	}
	return idx
}
