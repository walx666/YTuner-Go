package ytuner

// Minimal emulation of FPC's TIniFile (fcl-base/inifiles.pp) as used by YTuner:
//   - case-insensitive section and key names
//   - values trimmed, surrounding quotes stripped on read (ifoStripQuotes)
//   - ReadBool: true unless the first char is '0'
//   - comment lines (';') are preserved, the file is rewritten in FPC's
//     normalized layout on Save (sections separated by an empty line).

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type iniKey struct {
	ident string // '' for invalid lines; comment lines keep the full line in ident
	value string
}

type iniSection struct {
	name string // a section whose name starts with ';' is a leading comment block
	keys []iniKey
}

// IniFile is a TIniFile look-alike. Changes are written by Save (or Close).
type IniFile struct {
	fileName string
	sections []*iniSection
	dirty    bool
}

func isIniComment(s string) bool { return strings.HasPrefix(s, ";") }

// NewIniFile loads the file (a missing file yields an empty ini).
func NewIniFile(fileName string) *IniFile {
	ini := &IniFile{fileName: fileName}
	data, err := os.ReadFile(fileName)
	if err == nil {
		ini.parse(data)
	}
	return ini
}

func (f *IniFile) parse(data []byte) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	var cur *iniSection
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if isIniComment(line) && cur == nil {
			// comment at the beginning of the ini file
			cur = &iniSection{name: line}
			f.sections = append(f.sections, cur)
			cur = nil
			continue
		}
		if line[0] == '[' && line[len(line)-1] == ']' {
			cur = &iniSection{name: line[1 : len(line)-1]}
			f.sections = append(f.sections, cur)
			continue
		}
		if cur == nil {
			continue
		}
		if isIniComment(line) {
			cur.keys = append(cur.keys, iniKey{ident: line})
			continue
		}
		if j := strings.IndexByte(line, '='); j >= 0 {
			cur.keys = append(cur.keys, iniKey{ident: strings.TrimSpace(line[:j]), value: strings.TrimSpace(line[j+1:])})
		} else {
			cur.keys = append(cur.keys, iniKey{ident: "", value: line})
		}
	}
}

func (f *IniFile) section(name string) *iniSection {
	for _, s := range f.sections {
		if !isIniComment(s.name) && strings.EqualFold(s.name, name) {
			return s
		}
	}
	return nil
}

func (s *iniSection) key(ident string) *iniKey {
	for i := range s.keys {
		if s.keys[i].ident != "" && !isIniComment(s.keys[i].ident) && strings.EqualFold(s.keys[i].ident, ident) {
			return &s.keys[i]
		}
	}
	return nil
}

// ValueExists mirrors TCustomIniFile.ValueExists.
func (f *IniFile) ValueExists(section, ident string) bool {
	s := f.section(section)
	return s != nil && s.key(ident) != nil
}

// ReadString mirrors TIniFile.ReadString (with ifoStripQuotes).
func (f *IniFile) ReadString(section, ident, def string) string {
	s := f.section(section)
	if s == nil {
		return def
	}
	k := s.key(ident)
	if k == nil {
		return def
	}
	return stripQuotes(k.value)
}

func stripQuotes(v string) string {
	if n := len(v); n > 1 && (v[0] == '"' || v[0] == '\'') && v[n-1] == v[0] {
		return v[1 : n-1]
	}
	return v
}

// ReadInteger mirrors TCustomIniFile.ReadInteger (StrToIntDef).
func (f *IniFile) ReadInteger(section, ident string, def int) int {
	if v, ok := tryStrToInt(f.ReadString(section, ident, "")); ok {
		return v
	}
	return def
}

// ReadBool mirrors TCustomIniFile.ReadBool (CharToBool(s[1])).
func (f *IniFile) ReadBool(section, ident string, def bool) bool {
	s := f.ReadString(section, ident, "")
	if s == "" {
		return def
	}
	return s[0] != '0'
}

// WriteString mirrors TIniFile.WriteString.
func (f *IniFile) WriteString(section, ident, value string) {
	s := f.section(section)
	if s == nil {
		s = &iniSection{name: section}
		f.sections = append(f.sections, s)
	}
	if k := s.key(ident); k != nil {
		if k.value != value {
			k.value = value
			f.dirty = true
		}
		return
	}
	s.keys = append(s.keys, iniKey{ident: ident, value: value})
	f.dirty = true
}

// WriteInteger mirrors TCustomIniFile.WriteInteger.
func (f *IniFile) WriteInteger(section, ident string, value int) {
	f.WriteString(section, ident, strconv.Itoa(value))
}

// ReadSections mirrors TIniFile.ReadSections (comment blocks excluded).
func (f *IniFile) ReadSections() []string {
	var res []string
	for _, s := range f.sections {
		if !isIniComment(s.name) {
			res = append(res, s.name)
		}
	}
	return res
}

// ReadSectionRaw returns the "Ident=Value" pairs of a section (comments and
// invalid lines are skipped; values are returned unquoted-as-is).
func (f *IniFile) ReadSectionRaw(section string) []string {
	s := f.section(section)
	if s == nil {
		return nil
	}
	var res []string
	for _, k := range s.keys {
		if k.ident == "" || isIniComment(k.ident) {
			continue
		}
		res = append(res, k.ident+"="+k.value)
	}
	return res
}

// Dirty reports whether there are unsaved changes.
func (f *IniFile) Dirty() bool { return f.dirty }

// Save writes the file in TIniFile.UpdateFile layout if it was modified.
func (f *IniFile) Save() error {
	if !f.dirty {
		return nil
	}
	var b strings.Builder
	for i, s := range f.sections {
		if isIniComment(s.name) {
			b.WriteString(s.name + LineEnding)
		} else {
			b.WriteString("[" + s.name + "]" + LineEnding)
		}
		for _, k := range s.keys {
			switch {
			case isIniComment(k.ident):
				b.WriteString(k.ident + LineEnding)
			case k.ident == "":
				b.WriteString("=" + k.value + LineEnding)
			default:
				b.WriteString(k.ident + "=" + k.value + LineEnding)
			}
		}
		if i < len(f.sections)-1 && !isIniComment(s.name) {
			b.WriteString(LineEnding)
		}
	}
	if d := filepath.Dir(f.fileName); d != "" {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(f.fileName, []byte(b.String()), 0o644); err != nil {
		return err
	}
	f.dirty = false
	return nil
}
