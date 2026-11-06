package ytuner

// Radio-browser catFile / catMemStr cache (second half of src/radiobrowser.pas).
//
// The serialized format is byte compatible with the Pascal build:
//   'S' | 'C'                 1 byte type (stations / categories)
//   count                     uint32 little endian
//   stations:   12 x string   (uint16 length + bytes) per station
//   categories: string + int32 station count

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type rbCacheEntry struct {
	name    string
	records []byte    // catMemStr only
	eol     time.Time // catMemStr only (zero = no TTL)
}

// RBCacheList mirrors TRBCache (a TStringList of cache names with optional objects).
type RBCacheList struct {
	mu      sync.Mutex
	entries []*rbCacheEntry
}

func (c *RBCacheList) indexOf(name string) int {
	for i, e := range c.entries {
		if strings.EqualFold(e.name, name) { // TStringList.IndexOf, case-insensitive
			return i
		}
	}
	return -1
}

func (c *RBCacheList) delete(i int) {
	c.entries = append(c.entries[:i], c.entries[i+1:]...)
}

// Count returns the number of cache entries.
func (c *RBCacheList) Count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// Add adds a name (used by LoadRBCacheFilesInfo).
func (c *RBCacheList) Add(name string) {
	c.mu.Lock()
	c.entries = append(c.entries, &rbCacheEntry{name: name})
	c.mu.Unlock()
}

func rbCacheFileName(cacheName string, avrIdx int) string {
	return pathJoin(CachePath, cacheName+"-"+uintToStr(avrConfigAt(avrIdx).CRC32)+CacheExt)
}

// Get mirrors GetRBCache. Exactly one of stations/cats must be non-nil.
// Returns the total number of cached objects (0 = cache miss).
func (c *RBCacheList) Get(stations *[]RBStation, cats *[]RBCategory, cacheName string, avrIdx, start, howMany int) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	idx := c.indexOf(cacheName)
	if idx < 0 {
		return 0
	}
	result := 0
	switch RBCacheType {
	case CatFile:
		fn := rbCacheFileName(cacheName, avrIdx)
		age, ok := fileAge(fn)
		if !ok {
			Logging(LtError, JoinMsg("GetRBCache", MsgRBCacheFileType, "no file", MsgCache, cacheName))
			break
		}
		if RBCacheTTL > 0 && expiredAfterHours(age, RBCacheTTL) {
			Logging(LtInfo, JoinMsg("GetRBCache", MsgRBCacheFileType, MsgCache, MsgRBCacheTTLExpReload, cacheName))
			return 0
		}
		data, err := os.ReadFile(fn)
		if err != nil {
			Logging(LtError, JoinMsg("LoadRBObjects", MsgRBCacheFileType, MsgStream, MsgCache, MsgError, " ("+err.Error()+")"))
			break
		}
		result = deserializeInto(data, stations, cats, start, howMany, "")
	case CatMemStr:
		e := c.entries[idx]
		if !e.eol.IsZero() && e.eol.Before(time.Now()) {
			Logging(LtInfo, JoinMsg("GetRBCache", MsgRBCacheMemStrType, MsgCache, MsgRBCacheTTLExpReload, cacheName))
			return 0
		}
		result = deserializeInto(e.records, stations, cats, start, howMany, "")
	}
	if result > 0 {
		Logging(LtDebug, JoinMsg(MsgCache, MsgLoaded, cacheName))
	} else {
		Logging(LtError, JoinMsg("GetRBCache", MsgCache, MsgEmpty, cacheName))
		c.delete(idx)
	}
	return result
}

func deserializeInto(data []byte, stations *[]RBStation, cats *[]RBCategory, start, howMany int, objectName string) int {
	st, ct, n, err := DeserializeRBObjects(data, start, howMany, objectName)
	if err != nil {
		Logging(LtError, JoinMsg("DeSerializeRBObjects", MsgCache, MsgError, " ("+err.Error()+")"))
		return 0
	}
	if stations != nil {
		*stations = st
	}
	if cats != nil {
		*cats = ct
	}
	return n
}

// SetStations mirrors SetRBCache for a station list.
func (c *RBCacheList) SetStations(list []RBStation, cacheName string, avrIdx int) bool {
	if len(list) == 0 {
		Logging(LtError, JoinMsg("SetRBCache", MsgEmpty, MsgObjects, cacheName))
		return false
	}
	return c.set(SerializeRBStations(list), cacheName, avrIdx)
}

// SetCategories mirrors SetRBCache for a category list.
func (c *RBCacheList) SetCategories(list []RBCategory, cacheName string, avrIdx int) bool {
	if len(list) == 0 {
		Logging(LtError, JoinMsg("SetRBCache", MsgEmpty, MsgObjects, cacheName))
		return false
	}
	return c.set(SerializeRBCategories(list), cacheName, avrIdx)
}

func (c *RBCacheList) set(data []byte, cacheName string, avrIdx int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	idx := c.indexOf(cacheName)
	ok := false
	switch RBCacheType {
	case CatFile:
		fn := rbCacheFileName(cacheName, avrIdx)
		if err := os.WriteFile(fn, data, 0o644); err != nil {
			Logging(LtError, JoinMsg("SetRBCache", MsgRBCacheFileType, MsgCache, MsgSave, MsgError, cacheName, " ("+err.Error()+")"))
		} else {
			ok = true
			if idx < 0 {
				c.entries = append(c.entries, &rbCacheEntry{name: cacheName})
			}
		}
	case CatMemStr:
		e := &rbCacheEntry{name: cacheName, records: data}
		if RBCacheTTL > 0 {
			e.eol = time.Now().Add(time.Duration(RBCacheTTL) * time.Hour)
		}
		if idx < 0 {
			c.entries = append(c.entries, e)
		} else {
			c.entries[idx] = e
		}
		ok = true
	}
	if ok {
		Logging(LtDebug, JoinMsg(MsgCache, MsgSaved, cacheName))
	} else if idx >= 0 {
		c.delete(idx)
	}
	return ok
}

// GetRBStationCache mirrors GetRBStationCache: looks for a station UUID in
// all station caches of the AVR.
func GetRBStationCache(uuid string, avrIdx int) (RBStation, bool) {
	c := rbCache
	c.mu.Lock()
	defer c.mu.Unlock()
	if RBCacheType == CatNone || len(c.entries) == 0 {
		return RBStation{}, false
	}
	prefix := RBCacheStationsPrefix + avrMACAt(avrIdx) + "-"
	for _, e := range c.entries {
		if !strings.HasPrefix(e.name, prefix) {
			continue
		}
		var data []byte
		typ := MsgRBCacheMemStrType
		switch RBCacheType {
		case CatFile:
			typ = MsgRBCacheFileType
			fn := rbCacheFileName(e.name, avrIdx)
			age, ok := fileAge(fn)
			if !ok || (RBCacheTTL > 0 && expiredAfterHours(age, RBCacheTTL)) {
				continue
			}
			data, _ = os.ReadFile(fn)
		case CatMemStr:
			if !e.eol.IsZero() && !e.eol.After(time.Now()) {
				continue
			}
			data = e.records
		}
		if !bytes.Contains(data, []byte(uuid)) {
			continue
		}
		st, _, n, err := DeserializeRBObjects(data, 0, 0, uuid)
		if err != nil {
			Logging(LtError, JoinMsg("GetRBStationCache", typ, MsgCache, MsgError, e.name, " ("+err.Error()+")"))
			continue
		}
		if n == 1 && len(st) == 1 {
			Logging(LtDebug, JoinMsg("GetRBStationCache", typ, MsgCache, MsgLoaded, e.name))
			return st[0], true
		}
	}
	return RBStation{}, false
}

// RemoveEmptyCategory mirrors RemoveEmptyCategory.
func RemoveEmptyCategory(cacheName, name string, avrIdx int) bool {
	c := rbCache
	c.mu.Lock()
	defer c.mu.Unlock()
	idx := c.indexOf(cacheName)
	if idx < 0 {
		return false
	}
	var data []byte
	fn := rbCacheFileName(cacheName, avrIdx)
	switch RBCacheType {
	case CatFile:
		var err error
		if data, err = os.ReadFile(fn); err != nil {
			Logging(LtError, JoinMsg("RemoveEmptyCategory", MsgRBCacheFileType, "no file", MsgCache, cacheName))
			return false
		}
	case CatMemStr:
		data = c.entries[idx].records
	default:
		return false
	}
	_, cats, n, err := DeserializeRBObjects(data, 0, 0, "")
	if err != nil || n == 0 {
		return false
	}
	li := -1
	for i, ct := range cats {
		if strings.EqualFold(ct.Name, name) {
			li = i
			break
		}
	}
	if li <= 0 { // sic: Pascal checks "LIdx>0"
		return false
	}
	cats = append(cats[:li], cats[li+1:]...)
	if len(cats) == 0 {
		return false
	}
	out := SerializeRBCategories(cats)
	switch RBCacheType {
	case CatFile:
		if err := os.WriteFile(fn, out, 0o644); err != nil {
			return false
		}
	case CatMemStr:
		c.entries[idx].records = out
	}
	return true
}

func writeStr(b *bytes.Buffer, s string) {
	if len(s) > 0xFFFF {
		s = s[:0xFFFF]
	}
	_ = binary.Write(b, binary.LittleEndian, uint16(len(s)))
	b.WriteString(s)
}

// SerializeRBStations mirrors SerializeRBObjects for stations.
func SerializeRBStations(list []RBStation) []byte {
	var b bytes.Buffer
	b.WriteByte('S')
	_ = binary.Write(&b, binary.LittleEndian, uint32(len(list)))
	for _, s := range list {
		for _, v := range []string{s.ID, s.Name, s.HomePageURL, s.URL, s.Icon, s.Tags, s.Country, s.CountryCode, s.Language, s.Votes, s.Codec, s.Bitrate} {
			writeStr(&b, v)
		}
	}
	return b.Bytes()
}

// SerializeRBCategories mirrors SerializeRBObjects for categories.
func SerializeRBCategories(list []RBCategory) []byte {
	var b bytes.Buffer
	b.WriteByte('C')
	_ = binary.Write(&b, binary.LittleEndian, uint32(len(list)))
	for _, c := range list {
		writeStr(&b, c.Name)
		_ = binary.Write(&b, binary.LittleEndian, int32(c.MaxCount))
	}
	return b.Bytes()
}

var errShortCache = errors.New("truncated cache stream")

type cacheReader struct {
	d   []byte
	pos int
}

func (r *cacheReader) u16() (int, error) {
	if r.pos+2 > len(r.d) {
		return 0, errShortCache
	}
	v := binary.LittleEndian.Uint16(r.d[r.pos:])
	r.pos += 2
	return int(v), nil
}

func (r *cacheReader) str() (string, error) {
	n, err := r.u16()
	if err != nil {
		return "", err
	}
	if r.pos+n > len(r.d) {
		return "", errShortCache
	}
	s := string(r.d[r.pos : r.pos+n])
	r.pos += n
	return s, nil
}

func (r *cacheReader) i32() (int, error) {
	if r.pos+4 > len(r.d) {
		return 0, errShortCache
	}
	v := int32(binary.LittleEndian.Uint32(r.d[r.pos:]))
	r.pos += 4
	return int(v), nil
}

// DeserializeRBObjects mirrors DeSerializeRBObjects. With objectName set it
// returns at most the one object with that ID/name and result 1 (0 if not found);
// otherwise it returns the page [start, start+howMany) and the total count.
func DeserializeRBObjects(data []byte, start, howMany int, objectName string) ([]RBStation, []RBCategory, int, error) {
	if len(data) < 5 {
		return nil, nil, 0, errShortCache
	}
	typ := data[0]
	result := int(binary.LittleEndian.Uint32(data[1:5]))
	r := &cacheReader{d: data, pos: 5}
	if start == 0 && howMany <= 0 {
		howMany = result
	} else if start+howMany > result {
		howMany = result - start
	}
	var stations []RBStation
	var cats []RBCategory
	found := false
	for i := 0; i < start+howMany && !found; i++ {
		switch typ {
		case 'S':
			var f [12]string
			for k := range f {
				s, err := r.str()
				if err != nil {
					return nil, nil, 0, err
				}
				f[k] = s
			}
			if i < start {
				continue
			}
			st := RBStation{ID: f[0], Name: f[1], HomePageURL: f[2], URL: f[3], Icon: f[4], Tags: f[5], Country: f[6], CountryCode: f[7], Language: f[8], Votes: f[9], Codec: f[10], Bitrate: f[11]}
			if objectName == "" || objectName == st.ID {
				stations = append(stations, st)
				if objectName != "" {
					found, result = true, 1
				}
			}
		case 'C':
			name, err := r.str()
			if err != nil {
				return nil, nil, 0, err
			}
			cnt, err := r.i32()
			if err != nil {
				return nil, nil, 0, err
			}
			if i < start {
				continue
			}
			if objectName == "" || objectName == name {
				cats = append(cats, RBCategory{Name: name, MaxCount: cnt})
				if objectName != "" {
					found, result = true, 1
				}
			}
		default:
			return nil, nil, 0, errors.New("unknown cache type")
		}
	}
	if objectName != "" && !found {
		result = 0
	}
	return stations, cats, result, nil
}

// LoadRBCacheFilesInfo mirrors LoadRBCacheFilesInfo: registers the existing
// (still valid) cache files of an AVR, deletes expired ones.
func LoadRBCacheFilesInfo(avrIdx int) {
	suffix := "-" + uintToStr(avrConfigAt(avrIdx).CRC32) + CacheExt
	pattern := pathJoin(CachePath, "RB*-"+avrMACAt(avrIdx)+"-*"+suffix)
	files, _ := filepath.Glob(pattern)
	if len(files) == 0 {
		return
	}
	Logging(LtInfo, JoinMsg(MsgLoading, avrMACAt(avrIdx), MsgCache))
	for _, f := range files {
		age, _ := fileAge(f)
		if RBCacheTTL <= 0 || !expiredAfterHours(age, RBCacheTTL) {
			rbCache.Add(strings.Replace(filepath.Base(f), suffix, "", 1))
		} else {
			_ = os.Remove(f)
		}
	}
}

// RemoveOldRBCacheFiles mirrors RemoveOldRBCacheFiles.
func RemoveOldRBCacheFiles() {
	if RBCacheTTL <= 0 {
		return
	}
	files, _ := filepath.Glob(pathJoin(CachePath, "RB*-*"+CacheExt))
	removed := false
	for _, f := range files {
		if age, ok := fileAge(f); ok && expiredAfterHours(age, RBCacheTTL) {
			_ = os.Remove(f)
			removed = true
		}
	}
	if removed {
		Logging(LtDebug, JoinMsg("old", MsgCache, MsgRemoved))
	}
}
