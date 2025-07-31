#! /usr/bin/env bash
set -e

args="$1"
if [[ "$args" == "test" ]]; then
  go test -v linuxvm/test/system
  exit
fi

# Copy libkrun dynamic lib to ./out/lib
{
  echo "copy prebuild libkrun dylib"
  rm -rf ./out
  mkdir -p out
  cp -av ./lib ./out/lib
}

# Download busybox.static from alpine v3.22 mirror, and save the busybox.static to ./out/3rd/busybox.static
{
  echo "get busybox static"
  mkdir -p ./out/3rd
  wget -c https://dl-cdn.alpinelinux.org/alpine/v3.22/main/aarch64/busybox-static-1.37.0-r18.apk --output-document ./out/busybox-static-1.37.0-r18.apk
  tar -C out -xvf out/busybox-static-1.37.0-r18.apk bin/busybox.static
  mv ./out/bin/busybox.static out/3rd
  rm -f ./out/busybox-static-1.37.0-r18.apk
  rm -f ./out/bin/busybox.static
}

echo "codesign out/lib/*.dylib"
codesign --force --deep --sign - "out/lib/libkrun.dylib"
codesign --force --deep --sign - "out/lib/libkrunfw.dylib"
codesign --force --deep --sign - "out/lib/libepoxy.dylib"
codesign --force --deep --sign - "out/lib/libvirglrenderer.dylib"
codesign --force --deep --sign - "out/lib/libMoltenVK.dylib"

echo "build revm from source"
GOOS=darwin GOARCH=arm64 go build -v -o "out/bin/revm-arm64" ./cmd/main.go

echo "codesign revm"
codesign --force --deep --sign -  "out/bin/revm-arm64"

echo "add rpath"
install_name_tool -add_rpath @executable_path/../lib "out/bin/revm-arm64"

echo "codesign revm again with revm.entitlements"
codesign --entitlements revm.entitlements --force -s - "out/bin/revm-arm64"

echo "Build bootstrap for guest"
GOOS=linux GOARCH=arm64 go build -v -o "out/3rd/bootstrap" ./cmd/bootstrap

echo "Packing revm and deps"
tar --zstd -cvf revm.tar out/