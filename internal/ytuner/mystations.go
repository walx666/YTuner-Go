package ytuner

// Custom stations list files support (src/my_stations.pas).

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MyStationsExt lists supported extensions (index 0 = INI, 1,2 = YAML).
var MyStationsExt = []string{".ini", ".yaml", ".yml"}

const MyStationsFileNameDefault = "stations.ini"

// MyStation mirrors TMyStation.
type MyStation struct {
	ID, Name, URL, LogoURL string
}

// MyStationsCategory is one element of TMyStationsGroup.
type MyStationsCategory struct {
	Category string
	Stations []MyStation
}

// MSStation mirrors TMSStation.
type MSStation struct {
	Category string
	Station  MyStation
}

var (
	MyStationsEnabled           = true
	MyStationsFileName          = MyStationsFileNameDefault
	MyStationsAutoRefreshPeriod = 0

	myStationsMu      sync.RWMutex
	myStations        []MyStationsCategory
	myStationsFileAge time.Time
	myStationsFileCRC uint32
)

// MyStations returns a snapshot of the station groups.
func MyStations() []MyStationsCategory {
	myStationsMu.RLock()
	defer myStationsMu.RUnlock()
	return myStations
}

func setMyStations(s []MyStationsCategory) {
	myStationsMu.Lock()
	myStations = s
	myStationsMu.Unlock()
}

// MyStationID mirrors MY_STATIONS_PREFIX+'_'+MD5Print(MD5String(Name+URL)).Substring(0,12).ToUpper.
func MyStationID(name, url string) string {
	sum := md5.Sum([]byte(name + url))
	return MyStationsPrefix + "_" + strings.ToUpper(hex.EncodeToString(sum[:])[:12])
}

// GetMyStationByID mirrors GetMyStationByID (zero value if not found).
func GetMyStationByID(id string) MSStation {
	for _, c := range MyStations() {
		for _, s := range c.Stations {
			if s.ID == id {
				return MSStation{Category: c.Category, Station: s}
			}
		}
	}
	return MSStation{}
}

// ReadMyStationsINIFile mirrors ReadMyStationsINIFile.
func ReadMyStationsINIFile(fileName string) bool {
	count := 0
	var groups []MyStationsCategory
	ok := func() bool {
		if !fileExists(fileName) {
			return false
		}
		ini := NewIniFile(fileName)
		for _, sec := range ini.ReadSections() {
			g := MyStationsCategory{Category: sec}
			lines := ini.ReadSectionRaw(sec)
			for _, l := range lines {
				eq := strings.IndexByte(l, '=')
				if eq < 0 {
					continue
				}
				v := l[eq+1:]
				parts := strings.Split(v, "|")
				st := MyStation{Name: strings.TrimSpace(l[:eq]), URL: strings.TrimSpace(parts[0])}
				if len(parts) > 1 {
					st.LogoURL = strings.TrimSpace(parts[1])
				}
				st.ID = MyStationID(st.Name, st.URL)
				g.Stations = append(g.Stations, st)
			}
			count += len(lines)
			groups = append(groups, g)
		}
		return count > 0
	}()
	if ok {
		setMyStations(groups)
		Logging(LtInfo, MsgSuccessfullyLoaded+strconv.Itoa(count)+" my "+MsgStations)
	} else {
		Logging(LtError, "INI"+MsgFileLoadError)
	}
	return ok
}

// ReadMyStationsYAMLFile mirrors ReadMyStationsYAMLFile (a YCast compatible,
// line based "Category:" / "  Name: URL|Logo" format, not a full YAML parser).
func ReadMyStationsYAMLFile(fileName string) bool {
	count := 0
	var groups []MyStationsCategory
	ok := func() bool {
		data, err := os.ReadFile(fileName)
		if err != nil {
			return false
		}
		data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
		sc := bufio.NewScanner(bytes.NewReader(data))
		sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
		for sc.Scan() {
			line := strings.TrimRight(sc.Text(), "\r")
			if strings.HasSuffix(line, ":") {
				groups = append(groups, MyStationsCategory{Category: strings.TrimRight(strings.TrimSpace(line), ":")})
				continue
			}
			if len(groups) > 0 && groups[len(groups)-1].Category != "" && strings.Contains(line, ": ") {
				// Split(['': '','|''])
				parts := splitMulti(line, []string{": ", "|"})
				st := MyStation{Name: strings.TrimSpace(parts[0])}
				if len(parts) > 1 {
					st.URL = strings.TrimSpace(parts[1])
				}
				if len(parts) > 2 {
					st.LogoURL = strings.TrimSpace(parts[2])
				}
				st.ID = MyStationID(st.Name, st.URL)
				g := &groups[len(groups)-1]
				g.Stations = append(g.Stations, st)
				count++
			}
		}
		return count > 0
	}()
	if ok {
		setMyStations(groups)
		Logging(LtInfo, MsgSuccessfullyLoaded+strconv.Itoa(count)+" my "+MsgStations)
	} else {
		Logging(LtError, "YAML"+MsgFileLoadError)
	}
	return ok
}

// splitMulti splits s at any of the given separators (keeps empty parts).
func splitMulti(s string, seps []string) []string {
	var res []string
	for {
		best, bestSep := -1, ""
		for _, sep := range seps {
			if i := strings.Index(s, sep); i >= 0 && (best < 0 || i < best) {
				best, bestSep = i, sep
			}
		}
		if best < 0 {
			return append(res, s)
		}
		res = append(res, s[:best])
		s = s[best+len(bestSep):]
	}
}

// ReadMyStations mirrors ReadMyStations (dispatch by file extension).
func ReadMyStations() bool {
	fn := pathJoin(ConfigPath, MyStationsFileName)
	switch indexText(fileExt(fn), MyStationsExt) {
	case 0:
		return ReadMyStationsINIFile(fn)
	case 1, 2:
		return ReadMyStationsYAMLFile(fn)
	}
	return false
}

// CheckMyStations mirrors CheckMyStationsThread: reloads the stations file
// if its modification time and CRC32 changed.
func CheckMyStations() {
	fn := pathJoin(ConfigPath, MyStationsFileName)
	if indexText(fileExt(fn), MyStationsExt) < 0 {
		Logging(LtError, "Unsupported \"My Stations\" file format.")
		return
	}
	age, _ := fileAge(fn)
	myStationsMu.RLock()
	oldAge, oldCRC := myStationsFileAge, myStationsFileCRC
	myStationsMu.RUnlock()
	if age.Equal(oldAge) {
		return
	}
	crc, _ := CalcFileCRC32(fn)
	if crc == oldCRC {
		return
	}
	if !oldAge.IsZero() && oldCRC != 0 {
		Logging(LtInfo, "\"My Stations\" file has been changed. Refreshing..")
	}
	if ReadMyStations() {
		myStationsMu.Lock()
		myStationsFileAge, myStationsFileCRC = age, crc
		myStationsMu.Unlock()
	}
}

// fileExt mirrors ExtractFileExt.
func fileExt(name string) string {
	for i := len(name) - 1; i >= 0 && name[i] != '/' && name[i] != '\\'; i-- {
		if name[i] == '.' {
			return name[i:]
		}
	}
	return ""
}

// indexText mirrors IndexText (case-insensitive).
func indexText(s string, list []string) int {
	for i, x := range list {
		if strings.EqualFold(s, x) {
			return i
		}
	}
	return -1
}

// indexStr mirrors IndexStr (case-sensitive).
func indexStr(s string, list []string) int {
	for i, x := range list {
		if s == x {
			return i
		}
	}
	return -1
}

func uintToStr(v uint32) string { return strconv.FormatUint(uint64(v), 10) }
