#! /usr/bin/env bash
set -e
echo "Build revm..."
rm -rf ./out && mkdir -p out

cp -av ./lib ./out
install_name_tool -id @rpath/libkrunfw.dylib ./out/lib/libkrunfw.dylib
install_name_tool -id @rpath/libkrun.dylib   ./out/lib/libkrun.dylib
codesign --force --deep --sign - "out/lib/libkrun.dylib"
codesign --force --deep --sign - "out/lib/libkrunfw.4.dylib"
GOOS=darwin GOARCH=arm64 go build -v -o "out/bin/revm-arm64" ./cmd/main.go

echo "add rpath"
# TODO any better way to set dylib load path ??
codesign --force --deep --sign -  "out/bin/revm-arm64"
install_name_tool -add_rpath @executable_path/../lib "out/bin/revm-arm64"

echo "Do codesign"
codesign --entitlements revm.entitlements --force -s - "out/bin/revm-arm64"

echo "Build bootstrap for linux"
GOOS=linux GOARCH=arm64 go build -v -o "out/bin/bootstrap-arm64" ./cmd/bootstrap

echo "Packing revm and deps"
tar -cvf revm.tar out/