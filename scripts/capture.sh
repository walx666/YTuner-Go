#!/usr/bin/env bash
# Capture responses of a running YTuner instance (Pascal or Go) for a fixed set
# of AVR-like requests, so two implementations can be diffed.
#
# Usage: capture.sh <base-url> <out-dir>
#   e.g. capture.sh http://127.0.0.1:18081 /tmp/ref/pas
#        capture.sh http://127.0.0.1:18082 /tmp/ref/go
#        diff -r /tmp/ref/pas /tmp/ref/go
#
# Notes:
# - Radio-browser.info answers are live data; capture both servers close in time.
# - Date/Content-Length headers are dropped, only status/Content-Type/Location/Server are kept.
set -u
BASE="${1:?base url}"
OUT="${2:?out dir}"
mkdir -p "$OUT"

n=0
get() { # get <name> <path>
  local name="$1" path="$2"
  n=$((n+1))
  local f
  f="$OUT/$(printf '%02d' $n)_$name"
  curl -s -m 60 -D "$f.hdr.raw" -o "$f.body" "$BASE$path"
  # keep only stable headers, normalized
  { head -1 "$f.hdr.raw" | tr -d '\r' | sed -E 's#HTTP/[0-9.]+ ([0-9]+).*#status: \1#';
    grep -iE '^(content-type|location|server):' "$f.hdr.raw" | tr -d '\r' | tr 'A-Z' 'a-z' | sort; } > "$f.hdr"
  rm -f "$f.hdr.raw"
  # normalize line endings in XML bodies (Pascal uses CRLF on Windows)
  if grep -q 'xml' "$f.hdr"; then sed -i 's/\r$//' "$f.body"; fi
  echo "$path" > "$f.req"
}

first_id() { # first StationId in a captured body
  grep -o '<StationId>[^<]*</StationId>' "$1" | head -1 | sed -E 's#</?StationId>##g'
}

get login_token   "/setupapp/Yamaha/asp/BrowseXML/loginXML.asp?token=0"
get login_menu    "/setupapp/Yamaha/asp/BrowseXML/loginXML.asp?mac=0011223344&dlang=eng"
get login_case    "/SETUPAPP/yamaha/ASP/browsexml/LOGINXML.ASP?mac=0011223344"
get ms_root       "/ytuner/mystations"
get ms_root_page  "/ytuner/mystations?start=2&howmany=2"
get ms_cat        "/ytuner/mystations/Pop%20Music"
get ms_cat_items  "/ytuner/mystations/Dance?startitems=2&enditems=3"
get ms_cat_bad    "/ytuner/mystations/NoSuchCategory"
MSID=$(first_id "$OUT/06_ms_cat.body")
get ms_stat       "/setupapp/Yamaha/asp/BrowseXML/statxml.asp?id=$MSID&mac=0011223344"
get ms_stat_bad   "/setupapp/Yamaha/asp/BrowseXML/statxml.asp?id=MS_000000000000"
get ms_play       "/ytuner/play?id=$MSID"
get play_bad      "/ytuner/play?id=XX_123"
get about         "/ytuner/about"
get empty         "/ytuner/empty"
get unknown       "/no/such/route"
get bm_empty      "/ytuner/bookmark?mac=0011223344"
get bm_add        "/setupapp/favxml.asp?id=$MSID&fav=add&mac=0011223344"
get bm_list       "/ytuner/bookmark?mac=0011223344"
BMID=$(first_id "$OUT/18_bm_list.body")
get bm_stat       "/setupapp/Yamaha/asp/BrowseXML/statxml.asp?id=$BMID&mac=0011223344"
get bm_del        "/setupapp/favxml.asp?id=$MSID&fav=del&mac=0011223344"
get bm_after_del  "/ytuner/bookmark?mac=0011223344"
get search_short  "/setupapp/Yamaha/asp/BrowseXML/search.asp?sSearchtype=2&search=ab"
get search_play   "/setupapp/Yamaha/asp/BrowseXML/search.asp?sSearchtype=3&search=$MSID"
get rb_root       "/ytuner/radiobrowser"
get rb_genre      "/ytuner/radiobrowser/genre?start=1&howmany=5"
get rb_country    "/ytuner/radiobrowser/country?start=1&howmany=5"
get rb_language   "/ytuner/radiobrowser/language?start=1&howmany=5"
get rb_bad_type   "/ytuner/radiobrowser/nosuchtype"
get rb_cat_cty    "/ytuner/radiobrowser/country/Germany?start=1&howmany=3"
get rb_popular    "/ytuner/radiobrowser/popular?start=1&howmany=3"
RBID=$(first_id "$OUT/30_rb_popular.body")
get rb_stat       "/setupapp/Yamaha/asp/BrowseXML/statxml.asp?id=$RBID"
get rb_search     "/setupapp/Yamaha/asp/BrowseXML/search.asp?sSearchtype=2&search=rock&start=1&howmany=3"
get icon_ms       "/ytuner/icon?id=$MSID"
echo "captured $n requests into $OUT"
