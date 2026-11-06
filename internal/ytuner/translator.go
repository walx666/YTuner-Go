package ytuner

// Translator (src/translator.pas): diacritics replacement and optional
// Google Translate based menu translation.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	utf8Latin1SupplementLow  = 160
	utf8Latin1SupplementHigh = 255
	utf8Latin1ExtendedALow   = 256
	utf8Latin1ExtendedAHigh  = 383
	utf8Latin1ExtendedBLow   = 384
	utf8Latin1ExtendedBHigh  = 639
)

var utf8Latin1Supplement = []string{
	" ", "!", "c", "L", "c", "Y", "I", "S", "\"", "c", "a", "<<", "-", "­", "R", "-", "o", "+/-", "2", "3",
	"\"", "u", " ", ".", ",", "1", "0", ">>", "1/4", "1/2", "3/4", "?", "A", "A", "A", "A", "A", "A",
	"AE", "C", "E", "E", "E", "E", "I", "I", "I", "I", "D", "N", "O", "O", "O", "O", "O", "x", "O",
	"U", "U", "U", "U", "Y", "TH", "ss", "a", "a", "a", "a", "a", "a", "ae", "c", "e", "e", "e", "e",
	"i", "i", "i", "i", "d", "n", "o", "o", "o", "o", "o", "/", "o", "u", "u", "u", "u", "y", "th", "y",
}

var utf8Latin1ExtendedA = []string{
	"A", "a", "A", "a", "A", "a", "C", "c", "C", "c", "C", "c", "C", "c", "D", "d", "D", "d",
	"E", "e", "E", "e", "E", "e", "E", "e", "E", "e", "G", "g", "G", "g", "G", "g", "G", "g",
	"H", "h", "H", "h", "I", "i", "I", "i", "I", "i", "I", "i", "I", "i", "IJ", "ij", "J", "j",
	"K", "k", "k", "L", "l", "L", "l", "L", "l", "L", "l", "l", "l", "N", "n", "N", "n", "N",
	"n", "n", "N", "n", "O", "o", "O", "o", "O", "o", "OE", "oe", "R", "r", "R", "r", "R", "r",
	"S", "s", "S", "s", "S", "s", "S", "s", "T", "t", "T", "t", "T", "t", "U", "u", "U", "u",
	"U", "u", "U", "u", "U", "u", "U", "u", "W", "w", "Y", "y", "Y", "Z", "z", "Z", "z", "Z", "z", "s",
}

var utf8Latin1ExtendedB = []string{
	"b", "B", "B", "b", "b", "b", "C", "C", "c", "D", "D", "D", "d", "d", "E", "E", "E", "F", "f",
	"G", "Y", "h", "I", "I", "K", "k", "l", "l", "M", "N", "n", "O", "O", "o", "OI", "oi", "P", "p",
	"R", "S", "s", "E", "l", "t", "T", "t", "T", "U", "u", "U", "U", "Y", "y", "Z", "z", "E", "E", "e", "e",
	"2", "5", "?", "t", "p", "I", "II", "I", "!", "DZ", "Dz", "dz", "LJ", "Lj", "lj", "NJ", "Nj", "nj",
	"A", "a", "I", "i", "O", "o", "U", "u", "U", "u", "U", "u", "U", "u", "U", "u", "e", "A", "a", "A", "a",
	"AE", "ae", "G", "g", "G", "g", "K", "k", "O", "o", "O", "o", "E", "e", "j", "Dz", "Dz", "dz",
	"G", "g", "H", "P", "N", "n", "A", "a", "AE", "ae", "O", "o", "A", "a", "A", "a", "E", "e", "E", "e",
	"I", "i", "I", "i", "O", "o", "O", "o", "R", "r", "R", "r", "U", "u", "U", "u", "S", "s", "T", "t",
	"3", "3", "H", "h", "N", "d", "8", "8", "Z", "z", "A", "a", "E", "e", "O", "o", "O", "o", "O", "o", "O", "o",
	"Y", "y", "l", "n", "t", "j", "db", "qp", "A", "C", "c", "L", "T", "s", "z", "?", "?", "B", "U", "V",
	"E", "e", "J", "j", "Q", "q", "R", "r", "Y", "y", "a", "a", "a", "b", "c", "c", "d", "d",
	"e", "e", "e", "e", "e", "e", "e", "f", "g", "g", "G", "Y", "h", "h", "h", "h", "i", "i", "I", "l", "l", "l",
	"B", "m", "m", "m", "n", "n", "N", "O", "OE", "O", "o", "r", "r", "r", "r", "r", "r", "r",
}

// ReplaceDiacritics mirrors ReplaceDiacritics: replaces 2-byte UTF-8 code
// points of the enabled Latin-1 ranges with ASCII look-alikes.
func ReplaceDiacritics(s string, avrIdx int) string {
	t := &avrConfigAt(avrIdx).Translator
	var b strings.Builder
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		repl := ""
		if size == 2 {
			switch {
			case r >= utf8Latin1SupplementLow && r <= utf8Latin1SupplementHigh && t.ReplaceUTF8Latin1Supplement:
				if k := int(r) - utf8Latin1SupplementLow; k < len(utf8Latin1Supplement) {
					repl = utf8Latin1Supplement[k]
				}
			case r >= utf8Latin1ExtendedALow && r <= utf8Latin1ExtendedAHigh && t.ReplaceUTF8Latin1ExtendedA:
				if k := int(r) - utf8Latin1ExtendedALow; k < len(utf8Latin1ExtendedA) {
					repl = utf8Latin1ExtendedA[k]
				}
			case r >= utf8Latin1ExtendedBLow && r <= utf8Latin1ExtendedBHigh && t.ReplaceUTF8Latin1ExtendedB:
				if k := int(r) - utf8Latin1ExtendedBLow; k < len(utf8Latin1ExtendedB) {
					repl = utf8Latin1ExtendedB[k]
				}
			}
		}
		if repl != "" {
			b.WriteString(repl)
		} else {
			b.WriteString(s[i : i+size])
		}
		i += size
	}
	return b.String()
}

// NameValueList is a thread safe ordered "Name=Value" list (TStringList with
// IndexOfName/ValueFromIndex, case-insensitive names).
type NameValueList struct {
	mu    sync.Mutex
	names []string
	vals  []string
}

// NewNameValueList creates an empty list.
func NewNameValueList() *NameValueList { return &NameValueList{} }

func (l *NameValueList) indexOf(name string) int {
	for i, n := range l.names {
		if strings.EqualFold(n, name) {
			return i
		}
	}
	return -1
}

// Lookup returns the value of name and whether it exists.
func (l *NameValueList) Lookup(name string) (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if i := l.indexOf(name); i >= 0 {
		return l.vals[i], true
	}
	return "", false
}

// AddIfMissing adds "name=" if not present.
func (l *NameValueList) AddIfMissing(name string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.indexOf(name) < 0 {
		l.names = append(l.names, name)
		l.vals = append(l.vals, "")
	}
}

// Set sets the value of an existing name (no-op if missing).
func (l *NameValueList) Set(name, value string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if i := l.indexOf(name); i >= 0 {
		l.vals[i] = value
	}
}

// Load reads a "name=value" text file.
func (l *NameValueList) Load(fileName string) error {
	data, err := os.ReadFile(fileName)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.names, l.vals = nil, nil
	sc := bufio.NewScanner(bytes.NewReader(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})))
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if line == "" {
			continue
		}
		if i := strings.IndexByte(line, '='); i >= 0 {
			l.names = append(l.names, line[:i])
			l.vals = append(l.vals, line[i+1:])
		} else {
			l.names = append(l.names, line)
			l.vals = append(l.vals, "")
		}
	}
	return nil
}

// Save writes the list as "name=value" lines.
func (l *NameValueList) Save(fileName string) error {
	l.mu.Lock()
	var b strings.Builder
	for i := range l.names {
		b.WriteString(l.names[i] + "=" + l.vals[i] + LineEnding)
	}
	l.mu.Unlock()
	return os.WriteFile(fileName, []byte(b.String()), 0o644)
}

func translatorFileName(avrIdx int) string {
	return pathJoin(ConfigPath, "translator-"+avrMACAt(avrIdx)+".txt")
}

var gtHTTPClient = &http.Client{Timeout: 30 * time.Second}

// GoogleTranslateService mirrors GoogleTranslateService.
func GoogleTranslateService(u, languageCode string) string {
	b, err := httpGetBytes(gtHTTPClient, u, map[string]string{
		HTTPHeaderAccept:         "application/json",
		HTTPHeaderAcceptLanguage: "en-US," + strings.ToLower(languageCode),
		HTTPHeaderCacheControl:   WebBrowserCacheControl,
		HTTPHeaderUserAgent:      WebBrowserUserAgent,
	}, 0)
	if err != nil {
		Logging(LtError, JoinMsg(MsgGoogleTranslate, MsgError, " ("+err.Error()+")"))
		return ""
	}
	return string(b)
}

// GoogleTranslate mirrors GoogleTranslate (returns the translated text of the
// last sentence fragment like the Pascal code does).
func GoogleTranslate(text, languageCode string) string {
	u := "https://translate.googleapis.com/translate_a/single?client=gtx" +
		"&q=" + HTTPEncode(text) + "&sl=auto&tl=" + languageCode + "&dt=t&ie=UTF-8&oe=UTF-8"
	resp := GoogleTranslateService(u, languageCode)
	if resp == "" {
		return ""
	}
	var data []any
	if err := json.Unmarshal([]byte(resp), &data); err != nil || len(data) == 0 {
		return ""
	}
	arr, ok := data[0].([]any)
	if !ok {
		return ""
	}
	result := ""
	for _, e := range arr {
		if ea, ok := e.([]any); ok && len(ea) > 0 {
			if s, ok := ea[0].(string); ok {
				result = strings.TrimSpace(s)
			}
		}
	}
	return result
}

// translateWorker replaces the GTThread timer thread: it is started on demand
// and processes the ToTranslate queues of all AVRs until they are empty.
var (
	gtMu      sync.Mutex
	gtRunning bool
	gtNeeded  bool
	gtStop    = make(chan struct{})
)

func triggerTranslator() {
	gtMu.Lock()
	defer gtMu.Unlock()
	if gtRunning {
		gtNeeded = true
		return
	}
	gtRunning = true
	Logging(LtDebug, JoinMsg(MsgGoogleTranslate, "Starting thread.."))
	go translateLoop()
}

func translateLoop() {
	// a small delay collects several menu items into one request (like the 1s timer)
	select {
	case <-time.After(time.Second):
	case <-gtStop:
		return
	}
	for {
		failed := false
		for idx := 0; idx < avrCount() && !failed; idx++ {
			failed = translateQueue(idx)
		}
		gtMu.Lock()
		again := gtNeeded && !failed
		gtNeeded = false
		if !again {
			gtRunning = false
			gtMu.Unlock()
			return
		}
		gtMu.Unlock()
	}
}

// translateQueue translates the pending names of one AVR. Returns true on failure.
func translateQueue(avrIdx int) bool {
	t := &avrConfigAt(avrIdx).Translator
	for {
		t.mu.Lock()
		if len(t.ToTranslate) <= 1 {
			t.mu.Unlock()
			return false
		}
		if t.ToTranslate[0] != ';' {
			t.ToTranslate = ";" + t.ToTranslate
		}
		toTranslate := t.ToTranslate
		t.mu.Unlock()

		translated := GoogleTranslate(toTranslate, t.LanguageCode)
		ok := translated != "" && strings.Count(toTranslate, ";") == strings.Count(translated, ";")
		names := strings.Split(strings.Trim(toTranslate, ";"), ";")
		if ok {
			values := strings.Split(strings.Trim(translated, ";"), ";")
			for i, n := range names {
				if i < len(values) {
					t.Items.Set(n, values[i])
				}
			}
			t.mu.Lock()
			t.ToTranslate = strings.Replace(t.ToTranslate, toTranslate, "", 1)
			if t.ToTranslate != "" {
				if !strings.HasPrefix(t.ToTranslate, ";") {
					t.ToTranslate = ";" + t.ToTranslate
				}
				if !strings.HasSuffix(t.ToTranslate, ";") {
					t.ToTranslate += ";"
				}
			}
			t.mu.Unlock()
			Logging(LtDebug, JoinMsg(MsgGoogleTranslate, "Translation completed."))
		} else {
			Logging(LtError, JoinMsg(MsgGoogleTranslate, "Wrong translation!"))
			for _, n := range names {
				v, _ := t.Items.Lookup(n)
				attempts, err := strconv.Atoi(strings.ReplaceAll(v, "!", ""))
				if err != nil {
					attempts = 0
				}
				attempts++
				if attempts > 4 {
					t.mu.Lock()
					t.ToTranslate = strings.Replace(t.ToTranslate, ";"+n+";", ";", 1)
					for strings.Contains(t.ToTranslate, ";;") {
						t.ToTranslate = strings.ReplaceAll(t.ToTranslate, ";;", ";")
					}
					if t.ToTranslate != "" && t.ToTranslate[0] != ';' {
						t.ToTranslate = ";" + t.ToTranslate
					}
					t.mu.Unlock()
				}
				if attempts <= 5 {
					t.Items.Set(n, "!"+strconv.Itoa(attempts)+"!")
				}
			}
			Logging(LtDebug, JoinMsg(MsgGoogleTranslate, "Translation failed."))
		}
		t.fileMu.Lock()
		if err := t.Items.Save(translatorFileName(avrIdx)); err != nil {
			Logging(LtError, JoinMsg(MsgTranslator, MsgFile, MsgSave, MsgError, " ("+err.Error()+")"))
		}
		t.fileMu.Unlock()
		if !ok {
			return true
		}
	}
}

// AddNameToTranslatorFile mirrors AddNameToTranslatorFile (appends "name=").
func AddNameToTranslatorFile(name string, avrIdx int) bool {
	fn := translatorFileName(avrIdx)
	t := &avrConfigAt(avrIdx).Translator
	t.fileMu.Lock()
	defer t.fileMu.Unlock()
	prefix := ""
	if data, err := os.ReadFile(fn); err == nil && len(data) > 0 && data[len(data)-1] != '\n' {
		prefix = LineEnding
	}
	f, err := os.OpenFile(fn, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		Logging(LtError, JoinMsg(MsgTranslator, fn, MsgFile, MsgError, " ("+err.Error()+")"))
		return false
	}
	defer f.Close()
	if _, err := f.WriteString(prefix + name + "=" + LineEnding); err != nil {
		Logging(LtError, JoinMsg(MsgTranslator, fn, MsgFile, MsgError, " ("+err.Error()+")"))
		return false
	}
	return true
}

// Translate mirrors Translate: returns the translation of name if known,
// otherwise registers it (translator file and, with Auto=1, the Google queue).
func Translate(name string, avrIdx int) string {
	result := name
	name = strings.TrimSpace(strings.NewReplacer("=", "-", ";", "-").Replace(name))
	t := &avrConfigAt(avrIdx).Translator
	value, exists := t.Items.Lookup(name)
	if !exists {
		if AddNameToTranslatorFile(name, avrIdx) {
			t.Items.AddIfMissing(name)
		}
		value = ""
	} else if t.AutoTranslate && strings.HasPrefix(value, "!") && strings.HasSuffix(value, "!") {
		if n, err := strconv.Atoi(strings.ReplaceAll(value, "!", "")); err == nil {
			if n > 4 {
				Logging(LtDebug, JoinMsg(MsgGoogleTranslate, "Too many failed attempts to translate:", name))
			} else {
				value = ""
			}
		}
	} else if value != "" {
		result = value
	}
	if value == "" && t.AutoTranslate && t.LanguageCode != "" {
		t.mu.Lock()
		if !strings.Contains(t.ToTranslate, ";"+name+";") {
			if t.ToTranslate == "" {
				t.ToTranslate = ";"
			}
			t.ToTranslate += name + ";"
		}
		t.mu.Unlock()
		triggerTranslator()
	}
	return result
}

// LoadTranslator mirrors LoadTranslator.
func LoadTranslator(avrIdx int, mac string) {
	fn := pathJoin(ConfigPath, "translator-"+mac+".txt")
	if !fileExists(fn) {
		return
	}
	if err := avrConfigAt(avrIdx).Translator.Items.Load(fn); err != nil {
		Logging(LtError, JoinMsg("LoadTranslator", MsgTranslator, MsgFileLoadError, " ("+err.Error()+")"))
		return
	}
	Logging(LtInfo, "Translator: loaded")
}

// StopTranslator stops a pending translator start.
func StopTranslator() {
	select {
	case <-gtStop:
	default:
		close(gtStop)
	}
}
