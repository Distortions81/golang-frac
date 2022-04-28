#!/bin/bash
rm wasm/main.wasm.gz
rm wasm/main.wasm

GOGC=10 GOOS=js GOARCH=wasm go build -ldflags="-s -w" -o wasm/main.wasm
gzip -9 wasm/main.wasm
