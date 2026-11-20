#!/bin/sh
# Meldet Go-Dateien, die gofmt anders formatieren würde (make check).
files=$(gofmt -l cmd internal tools)
if [ -n "$files" ]; then
  echo "gofmt fehlt bei:"
  echo "$files"
  exit 1
fi
