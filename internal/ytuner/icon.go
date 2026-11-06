package ytuner

// Station logo conversion/resizing (GetIcon of src/httpserver.pas).
// Supported input formats: JPEG, PNG, GIF (like the Pascal default build);
// output: JPEG (WRITERJPG default).

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif" // register GIF decoder
	"image/jpeg"
	_ "image/png" // register PNG decoder
	"math"
	"net/http"
	"os"
	"time"

	xdraw "golang.org/x/image/draw"
)

// IconJPEGQuality matches the FPC TFPWriterJPEG default quality.
const IconJPEGQuality = 75

var iconHTTPClient = &http.Client{Timeout: 30 * time.Second}

// ConvertIcon decodes an image, scales it down to fit into size x size
// (keeping the aspect ratio) and encodes it as JPEG. Transparent areas are
// placed on a white background.
func ConvertIcon(data []byte, size int) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if size > 0 && (w > size || h > size) {
		var scale float64
		if w >= h {
			scale = float64(size) / float64(w)
		} else {
			scale = float64(size) / float64(h)
		}
		w = int(math.Round(float64(w) * scale))
		h = int(math.Round(float64(h) * scale))
		if w < 1 {
			w = 1
		}
		if h < 1 {
			h = 1
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, xdraw.Over, nil)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: IconJPEGQuality}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// GetIcon mirrors GetIcon.
func GetIcon(req *Request, res *Response) {
	logReq(req)
	imageFile := req.Query.Get(PathParamID)
	cacheFile := pathJoin(CachePath, imageFile)
	validName := imageFile != "" && !containsPathChars(imageFile)

	if IconCache && validName && fileExists(cacheFile) {
		if data, err := os.ReadFile(cacheFile); err != nil {
			Logging(LtError, imageFile+MsgFileLoadError+" ("+err.Error()+")")
		} else if len(data) > 0 {
			ServerResponseBytes(HTTPCodeOK, CtJPG, res, data)
			return
		}
	}

	u := ""
	switch pasSubstring(imageFile, 0, 2) {
	case MyStationsPrefix:
		u = GetMyStationByID(imageFile).Station.LogoURL
	case RadioBrowserPrefix:
		u = GetRBStationByID(pasSubstring(imageFile, 3, 12), GetAVRConfigIdx(req)).Icon
	}
	if u != "" {
		Logging(LtDebug, MsgGetting+" "+u)
		data, err := httpGetBytes(iconHTTPClient, u, map[string]string{
			HTTPHeaderAccept:    HTTPResponseContentType[CtJPG] + "," + HTTPResponseContentType[CtPNG] + "," + HTTPResponseContentType[CtGIF],
			HTTPHeaderUserAgent: YTunerUserAgent + "/" + AppVersion,
		}, 16<<20)
		if err != nil {
			Logging(LtError, "Error getting image from: \""+u+"\"? ("+err.Error()+")")
		} else if len(data) > 0 {
			out, err := ConvertIcon(data, IconSize)
			if err != nil {
				Logging(LtError, "Unsupported image type: \""+u+"\" ("+err.Error()+")")
			} else {
				ServerResponseBytes(HTTPCodeOK, CtJPG, res, out)
				if IconCache && validName {
					if !dirExists(CachePath) {
						_ = os.MkdirAll(CachePath, 0o755)
					}
					if err := os.WriteFile(cacheFile, out, 0o644); err != nil {
						Logging(LtError, imageFile+MsgFileSaveError+" ("+err.Error()+")")
					}
				}
			}
		}
	}
	if !res.Sent {
		if u != "" {
			Logging(LtWarning, "Cannot process image from \""+u+"\"")
		}
		ServerResponseString(HTTPCodeNotFound, CtNone, res, "")
	}
}

// containsPathChars protects the icon cache against path traversal via ?id=.
func containsPathChars(s string) bool {
	for _, c := range s {
		switch c {
		case '/', '\\', ':', 0:
			return true
		}
	}
	return s == "." || s == ".."
}
