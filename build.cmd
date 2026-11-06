@echo off
rem ---------------------------------------------------------------------------
rem YTuner (Go port) - Windows build script.
rem
rem Usage: build.cmd [build^|test^|cross^|clean]   (default: build)
rem   build   host binary          -> bin\ytuner.exe
rem   test    go vet + unit tests
rem   cross   all release targets  -> bin\<os>-<arch>\ytuner[.exe]
rem   clean   remove bin\
rem
rem go.exe is taken from PATH, otherwise from MSYS2 (MSYS2_ROOT, default
rem C:\dev\msys64): %MSYS2_ROOT%\mingw64\bin\go.exe
rem ---------------------------------------------------------------------------
setlocal EnableExtensions EnableDelayedExpansion
cd /d "%~dp0"

set "CMD=%~1"
if "%CMD%"=="" set "CMD=build"
if "%MSYS2_ROOT%"=="" set "MSYS2_ROOT=C:\dev\msys64"

set "GO=go"
where go >nul 2>nul
if errorlevel 1 (
  if exist "%MSYS2_ROOT%\mingw64\bin\go.exe" (
    set "GO=%MSYS2_ROOT%\mingw64\bin\go.exe"
    if "%GOROOT%"=="" set "GOROOT=%MSYS2_ROOT%\mingw64\lib\go"
  ) else (
    echo ERROR: go.exe not found. Install it in MSYS2: pacman -S mingw-w64-x86_64-go
    exit /b 1
  )
)
set CGO_ENABLED=0
set "LDF=-s -w"

if /I "%CMD%"=="build" (
  "%GO%" build -trimpath -ldflags="%LDF%" -o bin\ytuner.exe .\cmd\ytuner || goto fail
  echo OK: bin\ytuner.exe
  exit /b 0
)
if /I "%CMD%"=="test" (
  "%GO%" vet ./... || goto fail
  "%GO%" test -count=1 ./... || goto fail
  exit /b 0
)
if /I "%CMD%"=="cross" (
  for %%T in (linux/amd64 linux/arm64 linux/arm linux/386 windows/amd64 windows/386 windows/arm64 darwin/amd64 darwin/arm64 freebsd/amd64) do (
    for /f "tokens=1,2 delims=/" %%A in ("%%T") do (
      set "EXT="
      if "%%A"=="windows" set "EXT=.exe"
      echo building %%A/%%B
      set "GOOS=%%A"
      set "GOARCH=%%B"
      set "GOARM=7"
      "%GO%" build -trimpath -ldflags="%LDF%" -o bin\%%A-%%B\ytuner!EXT! .\cmd\ytuner || goto fail
    )
  )
  exit /b 0
)
if /I "%CMD%"=="clean" (
  if exist bin rmdir /s /q bin
  exit /b 0
)
echo Unknown command: %CMD%
exit /b 2

:fail
echo BUILD FAILED.
exit /b 1
