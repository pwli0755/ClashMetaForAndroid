package main

/*
#cgo LDFLAGS: -llog

#include "bridge.h"
*/
import "C"

import (
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"

	"cfa/native/config"
	"cfa/native/delegate"
	"cfa/native/tailnet"
	"cfa/native/tunnel"

	"github.com/metacubex/mihomo/log"
)

func main() {
	panic("Stub!")
}

//export coreInit
func coreInit(home, versionName, gitVersion C.c_string, sdkVersion C.int) {
	h := C.GoString(home)
	v := C.GoString(versionName)
	g := C.GoString(gitVersion)
	s := int(sdkVersion)

	delegate.Init(h, v, g, s)
	// Keep native crash evidence private; Android's stderr is not retained by logcat.
	if output, err := os.OpenFile(filepath.Join(h, "cmfa-crash.txt"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600); err == nil {
		_ = debug.SetCrashOutput(output, debug.CrashOptions{})
		_ = output.Close()
	}

	reset()
}

//export reset
func reset() {
	tailnet.Pause()
	config.LoadDefault()
	tunnel.ResetStatistic()
	tunnel.CloseAllConnections()

	runtime.GC()
	debug.FreeOSMemory()
}

//export forceGc
func forceGc() {
	go func() {
		log.Infoln("[APP] request force GC")

		runtime.GC()
		debug.FreeOSMemory()
	}()
}
