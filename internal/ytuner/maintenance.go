package ytuner

// Maintenance service (src/maintenance.pas): currently only "shut down".

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	MaintenanceService            = "Maintenance Service"
	DefMaintenanceServerIPAddress = "127.0.0.1"
	DefMaintenanceServerPort      = 8080
	MsgService                    = "service"
	MsgShuttingDown               = "Shutting down.."
	MsgDone                       = "Done"
)

var (
	MaintenanceServiceEnabled  = false
	MaintenanceServerIPAddress = DefMaintenanceServerIPAddress
	MaintenanceServerPort      = DefMaintenanceServerPort
)

// StartMaintenanceServer mirrors MaintenaceHTTPServerThread. GET /ytuner/down
// shuts YTuner down (the request must target MaintenanceServerIPAddress).
func StartMaintenanceServer(shutdown func()) *http.Server {
	addr := net.JoinHostPort(MaintenanceServerIPAddress, strconv.Itoa(MaintenanceServerPort))
	srv := &http.Server{Addr: addr, ReadHeaderTimeout: 10 * time.Second}
	srv.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		res := newResponse()
		if strings.HasPrefix(r.Host, MaintenanceServerIPAddress) {
			ServerResponseString(HTTPCodeOK, CtNone, res, AppName+" "+MsgService+": "+MsgShuttingDown+"...")
			writeResponse(w, res)
			if strings.ToLower(r.URL.RequestURI()) == "/"+PathRoot+"/down" {
				go shutdown()
			}
			return
		}
		ServerResponseString(HTTPCodeUnavailable, CtNone, res, "Service unavailable.")
		writeResponse(w, res)
	})
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		Logging(LtError, MaintenanceService+": "+err.Error())
		return nil
	}
	Logging(LtInfo, MaintenanceService+": listening on: "+addr)
	go func() { _ = srv.Serve(ln) }()
	return srv
}
