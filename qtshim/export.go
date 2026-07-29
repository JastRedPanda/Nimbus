// Package main is built as a c-shared library and is never imported.
//
// It exists so that ONE `go build` produces the Qt shim: cgo compiles shim.cpp
// with the system C++ compiler and links Qt, and -buildmode=c-shared wraps the
// result in a .so. No CMake, no qmake, no second build system in the project -
// the same reason winres/ is a nested module rather than a script.
//
// The Go code here is deliberately empty. The symbols the loader binds are the
// extern "C" ones from shim.cpp, which cgo exports from the shared object as
// plain C symbols; nothing on this side has to wrap them. Verified with
// `nm -D --defined-only`: nimbus_qt_init and its siblings are there as T.
//
// WHERE QT IS is not stated here, and cannot be. cgo's `pkg-config` directive was
// the obvious way to ask, and it does not work: Debian and Ubuntu ship NO .pc
// files for Qt 6 at all - measured on ubuntu:22.04 with qt6-base-dev installed,
// where `find / -name "Qt6*.pc"` comes back empty - because upstream Qt 6 stopped
// generating them and made CMake the supported way to be found. Arch does ship
// them, which is exactly why the mistake survived until this was built on
// anything else.
//
// So the include and library paths come in through CGO_CXXFLAGS and CGO_LDFLAGS,
// which the Makefile fills from `qmake6 -query`. That tool IS in every
// distribution's Qt 6 development package, and it answers for the Qt it belongs
// to rather than for a path someone guessed. Build through `make shim`, not by
// running go build in this directory by hand.
//
// Its own module, like winres/, so that a C++ toolchain and Qt never enter the
// application's dependency graph or its build.
package main

/*
#cgo CXXFLAGS: -std=c++17 -fPIC
#include "shim.h"
*/
import "C"

func main() {}
