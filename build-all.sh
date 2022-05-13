#!/bin/bash
rm golang-frac.exe
rm golang-frac-linux64.bin


GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o golang-frac-win64.exe
zip -m golang-frac-win64.zip golang-frac-win64.exe


GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o golang-frac-linux64.bin
zip -m golang-frac-linux64.zip golang-frac-linux64.bin

rm wasm/main.wasm.gz
rm wasm/main.wasm

GOGC=10 GOOS=js GOARCH=wasm go build -ldflags="-s -w" -o wasm/main.wasm
gzip -9 wasm/main.wasm
