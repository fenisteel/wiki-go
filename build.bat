@echo off

set ExeName=wiki-go-windows-amd64
echo Building %ExeName%...

set GOOS=windows
set GOFLAGS=-mod=vendor

if not exist .\build (
	mkdir .\build
)

if exist .\build\%ExeName%.exe (
	del .\build\%ExeName%.exe
)

@echo on
go build -v -ldflags "-X 'wiki-go/internal/version.Version=%date% %time%'" -o .\build\%ExeName%.exe

@echo off
if exist .\build\%ExeName%.exe (
	pushd .\build
	.\%ExeName%.exe
	popd
)
