# YTuner – Go port

A Go port of [YTuner](https://github.com/coffeegreg/YTuner), originally written in Free Pascal. It replaces the vTuner internet radio service for AVRs.
It uses the same `ytuner.ini`, `avr.ini`, `stations.ini/.yaml`, bookmark and cache files as the Pascal version, so it can be swapped in directly.

* It is pure Go with `CGO_ENABLED=0` and produces one static binary. SQLite is built in via `modernc.org/sqlite`, so it needs no `sqlite3.dll`, no OpenSSL and no Indy.
* It cross-compiles for Linux (amd64/arm64/arm/386), Windows, macOS and FreeBSD without a C toolchain.

## Build

```sh
./build.sh            # host binary -> bin/ytuner[.exe]   (MSYS2 mingw64 / Linux / macOS)
./build.sh test       # go vet + unit tests
./build.sh cross      # all targets -> bin/<os>-<arch>/
./build.sh dist       # cross + sample config from ../../cfg
build.cmd [build|test|cross|clean]          # Windows cmd (finds MSYS2 Go in C:\dev\msys64)
make [build|test|cross|dist|sql|clean]      # GNU make
```

Requires Go ≥ 1.26 (because of the modernc.org/sqlite and golang.org/x/image dependencies). To install it in MSYS2: `pacman -S mingw-w64-x86_64-go`.

## Run

Put `ytuner(.exe)` and `ytuner.ini` in a folder, and the config files in `config/` below it (see `../../cfg`), then start the binary.
Missing ini options are added automatically, like in the Pascal version. Stop it with Ctrl+C / SIGTERM, or with
`GET http://<MaintenanceServerIPAddress>:<port>/ytuner/down` if the maintenance service is enabled.

## Layout

| Go file (`internal/ytuner/`) | Pascal unit |
|---|---|
| `app.go` | `ytuner.pas` (start-up, `ytuner.ini`) |
| `common.go`, `inifile.go` | `common.pas`, FPC `TIniFile` |
| `httpserver.go`, `icon.go` | `httpserver.pas` (routes, handlers, logo conversion) |
| `vtuner.go`, `xmldom.go` | `vtuner.pas`, FPC XMLRead/XMLWrite layout |
| `avr.go` | `avr.pas` |
| `mystations.go` | `my_stations.pas` |
| `bookmark.go` | `bookmark.pas` |
| `radiobrowser.go`, `rbcache.go` | `radiobrowser.pas` |
| `radiobrowserdb.go`, `sql/*.sql` (embedded) | `radiobrowserdb.pas`, `src/sql` |
| `translator.go` | `translator.pas` |
| `dnsserver.go` | `dnsserver.pas` |
| `maintenance.go` | `maintenance.pas` |

`internal/ytuner/sql` is a copy of `../../src/sql`. Run `make sql` / `./build.sh sql` after changing the originals.

## Comparing with the Pascal build

`scripts/capture.sh <base-url> <out-dir>` sends 33 AVR-like requests and stores the normalized responses.
Run it against both servers and compare the results with `diff -r`.
