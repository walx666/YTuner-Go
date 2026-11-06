package ytuner

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func init() { LineEnding = "\n" } // deterministic expectations in tests

func TestMyStationID(t *testing.T) {
	// value produced by the Pascal build for cfg/stations.ini
	got := MyStationID("Antenne Bayern", "http://stream.antenne.de/antenne/stream/mp3")
	if got != "MS_E910A604A380" {
		t.Fatalf("MyStationID = %q", got)
	}
}

func TestPascalStringHash(t *testing.T) {
	// cache file names of the Pascal build: RBS-avr-4-3349775 is the search for "rock"
	if h := AbsInt32(PascalStringHash("rock")); h != 3349775 {
		t.Fatalf("hash(rock) = %d", h)
	}
	if h := AbsInt32(PascalStringHash("germany")); h != 1515021661 {
		t.Fatalf("hash(germany) = %d", h)
	}
}

func TestURLEncodeAndHTTPDecode(t *testing.T) {
	if got := URLEncode("Pop Music1"); got != "Pop%20Music%31" {
		t.Fatalf("URLEncode = %q", got)
	}
	if got := HTTPDecode("Pop%20Music+x%zz"); got != "Pop Music x%zz" {
		t.Fatalf("HTTPDecode = %q", got)
	}
	if got := HTTPEncode("a b&c"); got != "a+b%26c" {
		t.Fatalf("HTTPEncode = %q", got)
	}
}

func TestStripChars(t *testing.T) {
	if got := StripChars("  pop,rock -- (live)!  ", RBJSONStripChars); got != "pop rock live" {
		t.Fatalf("StripChars = %q", got)
	}
}

func TestIsWildAndHaveCommonElements(t *testing.T) {
	cases := []struct {
		in, pat string
		want    bool
	}{
		{"Dance", "*dance*", true},
		{"medieval", "*medieval", true},
		{"medieval rock", "*medieval", false},
		{"United Kingdom of Great Britain", "*Britain*", true},
		{"abc", "a?c", true},
		{"", "*", true},
	}
	for _, c := range cases {
		if got := IsWild(c.in, c.pat, true); got != c.want {
			t.Errorf("IsWild(%q,%q) = %v", c.in, c.pat, got)
		}
	}
	if !HaveCommonElements("pop,eurodance/90s", []string{"*dance*"}) {
		t.Error("HaveCommonElements: expected match")
	}
	if HaveCommonElements("pop,rock", []string{"*dance*"}) {
		t.Error("HaveCommonElements: unexpected match")
	}
}

func TestGetPageRange(t *testing.T) {
	q := func(s string) *Fields { return ParseQuery(s) }
	cases := []struct {
		query       string
		n           int
		first, last int
	}{
		{"", 5, 0, 4},
		{"start=2&howmany=2", 5, 1, 2},
		{"startitems=2&enditems=3", 5, 1, 2},
		{"start=1&howmany=100", 5, 0, 4},
		{"startitems=4&enditems=2", 5, 3, 3},
		{"", 0, 0, -1},
	}
	for _, c := range cases {
		f, l := GetPageRange(c.n, q(c.query))
		if f != c.first || l != c.last {
			t.Errorf("GetPageRange(%d,%q) = %d,%d want %d,%d", c.n, c.query, f, l, c.first, c.last)
		}
	}
}

func TestGetRange(t *testing.T) {
	cases := []struct {
		query        string
		start, count int
	}{
		{"", 0, -1},
		{"start=1&howmany=5", 0, 5},
		{"start=11&howmany=10", 10, 10},
		{"startitems=3&enditems=7", 2, 5},
		{"startitems=7&enditems=3", 6, 1},
	}
	for _, c := range cases {
		s, n, _ := GetRange(ParseQuery(c.query))
		if s != c.start || n != c.count {
			t.Errorf("GetRange(%q) = %d,%d want %d,%d", c.query, s, n, c.start, c.count)
		}
	}
}

func TestRouterMatching(t *testing.T) {
	r := RegisterServerRoutes()
	cases := []struct {
		path string
		want string // function name hint via Route params / handler identity
		cat  string
	}{
		{"/setupapp/Yamaha/asp/BrowseXML/loginXML.asp", "login", ""},
		{"/SETUPAPP/a/b/c/d/LOGINXML.ASP", "login", ""},
		{"/setupapp/loginXML.asp", "default", ""}, // '*' needs >= 1 segment
		{"/ytuner/mystations/Pop%20Music", "mscat", "Pop%20Music"},
		{"/ytuner/radiobrowser/country/Germany", "rbcat", "Germany"},
		{"/ytuner/radiobrowser/country/", "rbtype", ""},
		{"/nothing/here", "default", ""},
	}
	handlers := map[string]routeHandler{
		"login": SetupAppLoginXMLPage, "mscat": GetMyStationsOfCategory, "rbcat": GetRadioBrowserCategoryStations,
		"rbtype": GetRadioBrowserCategoryType, "default": DefaultPage,
	}
	for _, c := range cases {
		h, params := r.Match(c.path)
		if fnName(h) != fnName(handlers[c.want]) {
			t.Errorf("%s matched %s, want %s", c.path, fnName(h), c.want)
		}
		if c.cat != "" && params.Get(PathCategory) != c.cat {
			t.Errorf("%s category = %q", c.path, params.Get(PathCategory))
		}
	}
}

// fnName identifies a handler by its function pointer.
func fnName(h routeHandler) string { return fmtPtr(h) }

func TestXMLLayout(t *testing.T) {
	p := &VTunerPage{TotalItemsCount: 1}
	p.Add(&VTunerStation{UID: "MS_1", Name: "A & B", Bookmark: "http://h/x?id=1&fav=add"})
	got := string(p.ToXML())
	want := `<?xml version="1.0" encoding="utf-8"?>
<ListOfItems>
  <ItemCount>1</ItemCount>
  <Item>
    <ItemType>Station</ItemType>
    <StationId>MS_1</StationId>
    <StationName>A &amp; B</StationName>
    <StationUrl></StationUrl>
    <StationDesc></StationDesc>
    <Logo></Logo>
    <StationFormat></StationFormat>
    <StationLocation></StationLocation>
    <StationBandWidth></StationBandWidth>
    <StationMime></StationMime>
    <Relia>3</Relia>
    <Bookmark>http://h/x?id=1&amp;fav=add</Bookmark>
  </Item>
</ListOfItems>
`
	if got != want {
		t.Fatalf("XML layout mismatch:\n%s", got)
	}
	// re-read: empty elements become <X/> like FPC's XMLRead/XMLWrite
	root, err := ParseXML(strings.NewReader(got))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(XMLDocBytes(root)), "<StationMime/>") {
		t.Fatal("expected <StationMime/> after round trip")
	}
}

func TestIniFile(t *testing.T) {
	dir := t.TempDir()
	fn := filepath.Join(dir, "x.ini")
	src := ";comment\n[Configuration]\n  Key = \"quoted value\" \nFlag=0\n[radiobrowser]\nEnable=yes\n"
	if err := os.WriteFile(fn, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	ini := NewIniFile(fn)
	if v := ini.ReadString("configuration", "KEY", ""); v != "quoted value" {
		t.Errorf("ReadString = %q", v)
	}
	if ini.ReadBool("Configuration", "Flag", true) {
		t.Error("ReadBool(0) should be false")
	}
	if !ini.ReadBool("RadioBrowser", "Enable", false) {
		t.Error("ReadBool(yes) should be true")
	}
	ini.WriteString("RadioBrowser", "New", "1")
	ini.WriteString("Other", "A", "b")
	if err := ini.Save(); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(fn)
	want := ";comment\n[Configuration]\nKey=\"quoted value\"\nFlag=0\n\n[radiobrowser]\nEnable=yes\nNew=1\n\n[Other]\nA=b\n"
	if string(got) != want {
		t.Errorf("Save layout:\n%q\nwant\n%q", got, want)
	}
}

func TestRBCacheSerialization(t *testing.T) {
	st := []RBStation{{ID: "a-1", Name: "One", Bitrate: "128"}, {ID: "b-2", Name: "Two", Tags: "pop"}, {ID: "c-3", Name: "Three"}}
	data := SerializeRBStations(st)
	got, _, n, err := DeserializeRBObjects(data, 1, 1, "")
	if err != nil || n != 3 || len(got) != 1 || got[0] != st[1] {
		t.Fatalf("page: %v %d %v", got, n, err)
	}
	got, _, n, _ = DeserializeRBObjects(data, 0, 0, "c-3")
	if n != 1 || len(got) != 1 || got[0].Name != "Three" {
		t.Fatalf("by name: %v %d", got, n)
	}
	_, _, n, _ = DeserializeRBObjects(data, 0, 0, "zzz")
	if n != 0 {
		t.Fatalf("missing name should give 0, got %d", n)
	}
	cats := []RBCategory{{"Germany", 6320}, {"Spain", 1426}}
	_, gc, n, err := DeserializeRBObjects(SerializeRBCategories(cats), 0, 1, "")
	if err != nil || n != 2 || len(gc) != 1 || gc[0] != cats[0] {
		t.Fatalf("categories: %v %d %v", gc, n, err)
	}
	// byte layout: 'C', uint32 LE count, uint16 LE len + name, int32 LE count
	b := SerializeRBCategories([]RBCategory{{"ab", 1}})
	if !bytes.Equal(b, []byte{'C', 1, 0, 0, 0, 2, 0, 'a', 'b', 1, 0, 0, 0}) {
		t.Fatalf("layout % x", b)
	}
}

func TestDNSAnswer(t *testing.T) {
	// standard query for radio567.vtuner.com, type A, class IN
	q := []byte{0x12, 0x34, 0x01, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}
	for _, l := range []string{"radio567", "vtuner", "com"} {
		q = append(q, byte(len(l)))
		q = append(q, l...)
	}
	q = append(q, 0, 0, 1, 0, 1)
	name, qt, qc, _, err := parseDNSQuestion(q)
	if err != nil || name != "radio567.vtuner.com" || qt != 1 || qc != 1 {
		t.Fatalf("parse: %q %d %d %v", name, qt, qc, err)
	}
	InterceptDNs = "*.vtuner.com,exact.example.org"
	if !dnsIntercepted(name) || !dnsIntercepted("exact.example.org") || dnsIntercepted("example.org") {
		t.Fatal("intercept matching")
	}
	resp, isA := BuildDNSAnswer(q, net.ParseIP("192.168.1.10"))
	if !isA {
		t.Fatal("expected A answer")
	}
	if resp[0] != 0x12 || resp[1] != 0x34 || resp[2] != 0x81 || resp[3] != 0x80 || resp[7] != 1 {
		t.Fatalf("header % x", resp[:12])
	}
	if !bytes.Equal(resp[len(resp)-4:], []byte{192, 168, 1, 10}) {
		t.Fatalf("ip % x", resp[len(resp)-4:])
	}
}

func TestReplaceDiacritics(t *testing.T) {
	avrMu.Lock()
	avrConfigs = []*AVRConfig{{Translator: Translator{ReplaceUTF8Latin1Supplement: true, ReplaceUTF8Latin1ExtendedA: true, Items: NewNameValueList()}}}
	avrMACs = []string{"avr"}
	avrMu.Unlock()
	// Note: the original Pascal table maps 'Ł' (U+0141) to lower case 'l' - kept as is.
	if got := ReplaceDiacritics("Zürich Łódź €", 0); got != "Zurich lodz €" {
		t.Fatalf("ReplaceDiacritics = %q", got)
	}
}
