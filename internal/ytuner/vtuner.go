package ytuner

// vTuner page structures (src/vtuner.pas).

import (
	"strconv"
	"strings"
)

const (
	VTXMLListOfItems      = "ListOfItems"
	VTXMLItemCount        = "ItemCount"
	VTXMLItemType         = "ItemType"
	VTXMLItem             = "Item"
	VTXMLDir              = "Dir"
	VTXMLStation          = "Station"
	VTXMLDisplay          = "Display"
	VTXMLTitle            = "Title"
	VTXMLURLDir           = "UrlDir"
	VTXMLURLDirBackUp     = "UrlDirBackUp"
	VTXMLDirCount         = "DirCount"
	VTXMLStationID        = "StationId"
	VTXMLStationName      = "StationName"
	VTXMLStationURL       = "StationUrl"
	VTXMLStationDesc      = "StationDesc"
	VTXMLLogo             = "Logo"
	VTXMLStationFormat    = "StationFormat"
	VTXMLStationLocation  = "StationLocation"
	VTXMLStationBandWidth = "StationBandWidth"
	VTXMLStationMime      = "StationMime"
	VTXMLRelia            = "Relia"
	VTXMLBookmark         = "Bookmark"
	VTXMLEncryptedToken   = "<EncryptedToken></EncryptedToken>"
	PathParamDummy        = "?ytuner=true"

	VTToken = "0123456789ABCDEF"
)

// VTunerMime lists the MIME types an AVR understands.
var VTunerMime = []string{"MP3", "AAC", "WMA"}

// MyToken is the token returned for loginXML.asp?token=0.
var MyToken = VTToken

// VTunerInfo is an item of a vTuner page (TVTunerInfo).
type VTunerInfo interface {
	add2XML(root *XNode)
}

// VTunerPage mirrors TVTunerPage.
type VTunerPage struct {
	TotalItemsCount int
	Items           []VTunerInfo
}

// Add appends an item.
func (p *VTunerPage) Add(i VTunerInfo) { p.Items = append(p.Items, i) }

// MakeXML builds the DOM of the page.
func (p *VTunerPage) MakeXML() *XNode {
	root := NewElem(VTXMLListOfItems)
	root.Append(NewTextElem(VTXMLItemCount, strconv.Itoa(p.TotalItemsCount)))
	for _, it := range p.Items {
		it.add2XML(root)
	}
	return root
}

// ToXML serializes the page.
func (p *VTunerPage) ToXML() []byte { return XMLDocBytes(p.MakeXML()) }

// VTunerDirectory mirrors TVTunerDirectory.
type VTunerDirectory struct {
	Title       string
	Destination string
	ItemCount   int
}

func (d *VTunerDirectory) add2XML(root *XNode) {
	n := root.Append(NewElem(VTXMLItem))
	n.Append(NewTextElem(VTXMLItemType, VTXMLDir))
	n.Append(NewTextElem(VTXMLTitle, d.Title))
	n.Append(NewTextElem(VTXMLURLDir, AddYTunerParameter(d.Destination)))
	n.Append(NewTextElem(VTXMLURLDirBackUp, AddYTunerParameter(d.Destination)))
	n.Append(NewTextElem(VTXMLDirCount, strconv.Itoa(d.ItemCount)))
}

// VTunerStation mirrors TVTunerStation.
type VTunerStation struct {
	UID, Name, Description, URL, Icon, Genre, Location, Mime, Bitrate, Bookmark string
}

func (s *VTunerStation) add2XML(root *XNode) {
	n := root.Append(NewElem(VTXMLItem))
	n.Append(NewTextElem(VTXMLItemType, VTXMLStation))
	n.Append(NewTextElem(VTXMLStationID, s.UID))
	n.Append(NewTextElem(VTXMLStationName, s.Name))
	n.Append(NewTextElem(VTXMLStationURL, s.URL))
	n.Append(NewTextElem(VTXMLStationDesc, s.Description))
	n.Append(NewTextElem(VTXMLLogo, s.Icon))
	n.Append(NewTextElem(VTXMLStationFormat, s.Genre))
	n.Append(NewTextElem(VTXMLStationLocation, s.Location))
	n.Append(NewTextElem(VTXMLStationBandWidth, s.Bitrate))
	codec := ""
	for _, m := range VTunerMime {
		if strings.EqualFold(m, s.Mime) {
			codec = s.Mime
			break
		}
	}
	n.Append(NewTextElem(VTXMLStationMime, codec))
	n.Append(NewTextElem(VTXMLRelia, "3"))
	n.Append(NewTextElem(VTXMLBookmark, s.Bookmark))
}

// VTunerDisplay mirrors TVTunerDisplay.
type VTunerDisplay struct{ Message string }

func (d *VTunerDisplay) add2XML(root *XNode) {
	n := root.Append(NewElem(VTXMLItem))
	n.Append(NewTextElem(VTXMLItemType, VTXMLDisplay))
	n.Append(NewTextElem(VTXMLDisplay, d.Message))
}

// AddYTunerParameter mirrors AddYTunerParameter.
func AddYTunerParameter(u string) string { return u + PathParamDummy }
