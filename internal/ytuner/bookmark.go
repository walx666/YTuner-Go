package ytuner

// AVR bookmark feature (src/bookmark.pas).

import (
	"os"
	"strconv"
	"strings"
	"sync"
)

const (
	PathBookmark             = "bookmark"
	PathFavAction            = "fav"
	PathFavActionAdd         = "add"
	PathFavActionDel         = "del"
	DefBookmarkStationsLimit = 100
)

var (
	BookmarkEnabled       = true
	CommonBookmark        = false
	BookmarkStationsLimit = DefBookmarkStationsLimit

	bookmarkMu sync.Mutex // serializes bookmark file read-modify-write
)

// GetBookmarkFileName mirrors GetBookmarkFileName.
func GetBookmarkFileName(mac string) string {
	if CommonBookmark {
		mac = PathBookmark
	}
	return pathJoin(ConfigPath, mac+".xml")
}

// replaceIgnoreCaseFirst mirrors StringReplace(S, Old, New, [rfIgnoreCase]).
func replaceIgnoreCaseFirst(s, old, repl string) string {
	if old == "" {
		return s
	}
	i := strings.Index(strings.ToLower(s), strings.ToLower(old))
	if i < 0 {
		return s
	}
	return s[:i] + repl + s[i+len(old):]
}

func bookmarkItemCount(root *XNode) (int, bool) {
	if root.Name != VTXMLListOfItems || len(root.Children) == 0 || root.Children[0].Name != VTXMLItemCount {
		return 0, false
	}
	return tryStrToInt(root.Children[0].Text)
}

// GetBookmarkItemsCount mirrors GetBookmarkItemsCount.
func GetBookmarkItemsCount(mac string) int {
	fn := GetBookmarkFileName(mac)
	if !fileExists(fn) {
		return 0
	}
	bookmarkMu.Lock()
	root, err := ReadXMLFile(fn)
	bookmarkMu.Unlock()
	if err != nil {
		Logging(LtError, JoinMsg(MsgBookmark, MsgError, "[GetBookmarkItemsCount]", "("+err.Error()+") "))
		return 0
	}
	n, _ := bookmarkItemCount(root)
	return n
}

// setChildText replaces the text of a child, if present (FirstChild.NodeValue := ...).
func setChildText(n *XNode, name string, f func(string) string) {
	if c := n.Find(name); c != nil && c.HasText {
		c.Text = f(c.Text)
	}
}

// GetBookmark mirrors GetBookmark: returns the XML of the items [first..last].
func GetBookmark(req *Request, first, last int) ([]byte, bool) {
	fn := GetBookmarkFileName(req.Query.Get(PathParamMAC))
	avrIdx := GetAVRConfigIdx(req)
	if !fileExists(fn) {
		return nil, false
	}
	bookmarkMu.Lock()
	root, err := ReadXMLFile(fn)
	bookmarkMu.Unlock()
	if err == nil && len(root.Children) == 0 {
		err = errBadBookmark
	}
	if err != nil {
		Logging(LtError, JoinMsg(MsgBookmark, MsgError, "[GetBookmark]", "("+err.Error()+") "))
		return nil, false
	}
	part := NewElem(VTXMLListOfItems)
	part.Append(root.Children[0].Clone())
	tr := &avrConfigAt(avrIdx).Translator
	for i := first; i <= last; i++ {
		if i+1 >= len(root.Children) || i+1 < 1 {
			Logging(LtError, JoinMsg(MsgBookmark, MsgError, "[GetBookmark]", "(List index ("+strconv.Itoa(i+1)+") out of bounds) "))
			return nil, false
		}
		it := part.Append(root.Children[i+1].Clone())
		setChildText(it, VTXMLLogo, func(s string) string { return replaceIgnoreCaseFirst(s, YTunerHost, URLHost) })
		setChildText(it, VTXMLBookmark, func(s string) string { return replaceIgnoreCaseFirst(s, YTunerHost, URLHost) })
		if avrIdx >= 0 && tr.ReplaceAnyDiacritics() {
			setChildText(it, VTXMLStationName, func(s string) string { return ReplaceDiacritics(s, avrIdx) })
		}
	}
	return XMLDocBytes(part), true
}

type bookmarkError string

func (e bookmarkError) Error() string { return string(e) }

const errBadBookmark = bookmarkError("invalid bookmark file")

// GetBookmarkStationInfo mirrors GetBookmarkStationInfo.
func GetBookmarkStationInfo(mac, sid string) ([]byte, bool) {
	fn := GetBookmarkFileName(mac)
	if !fileExists(fn) {
		return nil, false
	}
	bookmarkMu.Lock()
	root, err := ReadXMLFile(fn)
	bookmarkMu.Unlock()
	if err != nil {
		Logging(LtError, JoinMsg(MsgBookmark, MsgError, "[GetBookmarkStationInfo]", "("+err.Error()+") "))
		return nil, false
	}
	part := NewElem(VTXMLListOfItems)
	part.Append(NewTextElem(VTXMLItemCount, "1"))
	var found *XNode
	for i := 1; i < len(root.Children) && found == nil; i++ {
		c := root.Children[i]
		if c.Name == VTXMLItem && c.TextOf(VTXMLStationID) == sid {
			found = c
		}
	}
	if found == nil {
		// Pascal returns True with an empty stream in this case.
		return nil, true
	}
	it := part.Append(found.Clone())
	setChildText(it, VTXMLLogo, func(s string) string { return replaceIgnoreCaseFirst(s, YTunerHost, URLHost) })
	setChildText(it, VTXMLBookmark, func(s string) string { return replaceIgnoreCaseFirst(s, YTunerHost, URLHost) })
	return XMLDocBytes(part), true
}

// SetBookmark mirrors SetBookmark (add/del of a station item node).
func SetBookmark(mac, action string, node *XNode) {
	fn := GetBookmarkFileName(mac)
	bookmarkMu.Lock()
	defer bookmarkMu.Unlock()
	var root *XNode
	if fileExists(fn) {
		var err error
		if root, err = ReadXMLFile(fn); err != nil {
			Logging(LtError, JoinMsg(MsgBookmark, MsgError, "[SetBookmark]", "("+err.Error()+") "))
			return
		}
	} else {
		switch action {
		case PathFavActionAdd:
			root = NewElem(VTXMLListOfItems)
			root.Append(NewTextElem(VTXMLItemCount, "0"))
		default:
			return
		}
	}
	count, ok := bookmarkItemCount(root)
	if !ok || count >= BookmarkStationsLimit {
		return
	}
	sid := node.TextOf(VTXMLStationID)
	idx := -1
	for i := 1; i < len(root.Children) && idx < 0; i++ {
		if c := root.Children[i]; c.Name == VTXMLItem && c.TextOf(VTXMLStationID) == sid {
			idx = i
		}
	}
	switch action {
	case PathFavActionAdd:
		if idx >= 0 {
			return
		}
		root.Children[0].Text = strconv.Itoa(count + 1)
		n := node.Clone()
		setChildText(n, VTXMLBookmark, func(s string) string {
			return replaceIgnoreCaseFirst(s, PathFavAction+"="+PathFavActionAdd, PathFavAction+"="+PathFavActionDel)
		})
		setChildText(n, VTXMLLogo, func(s string) string { return replaceIgnoreCaseFirst(s, URLHost, YTunerHost) })
		setChildText(n, VTXMLBookmark, func(s string) string { return replaceIgnoreCaseFirst(s, URLHost, YTunerHost) })
		// a freshly built node has empty text children: FPC's reader would drop them
		root.Append(n)
		if err := writeBookmark(root, fn); err != nil {
			Logging(LtError, JoinMsg(MsgBookmark, MsgError, "[SetBookmark]", "("+err.Error()+") "))
		}
	case PathFavActionDel:
		if idx < 0 {
			return
		}
		if count > 1 {
			root.Children[0].Text = strconv.Itoa(count - 1)
			root.Children = append(root.Children[:idx], root.Children[idx+1:]...)
			if err := writeBookmark(root, fn); err != nil {
				Logging(LtError, JoinMsg(MsgBookmark, MsgError, "[SetBookmark]", "("+err.Error()+") "))
			}
		} else {
			_ = os.Remove(fn)
		}
	}
}

func writeBookmark(root *XNode, fn string) error {
	// Write atomically to avoid a broken file on crash.
	tmp := fn + ".tmp"
	if err := os.WriteFile(tmp, XMLDocBytes(root), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, fn)
}
