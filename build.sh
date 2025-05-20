#! /usr/bin/env bash
echo "Build revm..."
rm -rf ./out && mkdir -p out

cp -av ./lib ./out
codesign --force --deep --sign - "out/lib/libkrun.1.11.2.dylib"
codesign --force --deep --sign - "out/lib/libkrunfw.4.dylib"

GOOS=darwin GOARCH=arm64 go build -v -o "out/bin/rvm-arm64" ./cmd/main.go


echo "add rpath"
# TODO any better way to set dylib load path ??
codesign --force --deep --sign -  "out/bin/rvm-arm64"
install_name_tool -add_rpath @executable_path/../lib "out/bin/rvm-arm64"
install_name_tool -change "@@HOMEBREW_PREFIX@@/opt/libkrunfw/lib/libkrunfw.4.dylib" "@rpath/libkrunfw.4.dylib"  "out/bin/rvm-arm64"
install_name_tool -change "@@HOMEBREW_PREFIX@@/opt/libkrun/lib/libkrun.dylib" "@rpath/libkrun.dylib"  "out/bin/rvm-arm64"

echo "Do codesign"
codesign --entitlements revm.entitlements --force -s - "out/bin/rvm-arm64"

echo "Build dhclient4 for linux"
GOOS=linux GOARCH=arm64 go build -v -o "out/bin/dhclient4-linux-arm64" ./cmd/dhclient4

echo "Packing revm and deps"
tar -cvf revm.tar out/