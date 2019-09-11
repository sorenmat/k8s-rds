// +build tools

package main

// This file contains tool dependency import statements. When used along with
// gomodules this ensures that we use the same versions of tools.
// https://github.com/golang/go/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module

// golangci-lint is used for linting the project
import _ "github.com/golangci/golangci-lint/cmd/golangci-lint"
