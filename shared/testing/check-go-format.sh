#!/bin/sh
# Maintained by RepSync; adopted as the go-format-check shared resource.
set -eu

usage() {
  printf 'Usage: sh check-go-format.sh PATH [PATH ...]\n'
}

if [ "$#" -eq 1 ] && [ "$1" = '--help' ]; then
  usage
  exit 0
fi

if [ "$#" -eq 0 ]; then
  usage >&2
  exit 2
fi

for path do
  case "$path" in
    -*)
      printf 'Expected a path, got option: %s (prefix such paths with ./)\n' "$path" >&2
      exit 2
      ;;
  esac
done

if ! command -v gofmt >/dev/null 2>&1; then
  printf 'gofmt is required; install Go and add it to PATH.\n' >&2
  exit 127
fi

files=$(gofmt -l "$@")
if [ -n "$files" ]; then
  printf 'Go formatting required:\n%s\n' "$files" >&2
  exit 1
fi

printf 'Go formatting passed: %s input path(s)\n' "$#"
