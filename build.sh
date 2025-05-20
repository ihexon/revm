#! /usr/bin/env bash
echo "Build revm..."
rm -rf ./out && mkdir -p out

cp -av ./lib ./out
codesign --force --deep --sign - "out/lib/libkrun.1.11.2.dylib"
codesign --force --deep --sign - "out/lib/libkrunfw.4.dylib"

go build -v -o "out/bin/rvm" cmd/main.go

echo "Do codesign"
codesign --entitlements revm.entitlements --force -s - out/bin/rvm