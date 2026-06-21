package main

/*
#include <stdlib.h>
typedef void (*LinettaNotifyCallback)(const char* method, const char* params);
static inline void linetta_invoke_notify(LinettaNotifyCallback cb, const char* m, const char* p) { cb(m, p); }
*/
import "C"

import (
	"context"
	"encoding/json"
	"sync"
	"unsafe"

	"github.com/devlikebear/linetta/engine/internal/engineapp"
)

var (
	appMu sync.RWMutex
	app   *engineapp.App
	ctx   = context.Background()

	notifyMu sync.Mutex
	cNotify  C.LinettaNotifyCallback
	goNotify func(method string, params json.RawMessage)
)

func startEngine(home string) error {
	appMu.Lock()
	defer appMu.Unlock()
	if app != nil {
		return nil
	}
	a, err := engineapp.Open(ctx, engineapp.Options{Home: home})
	if err != nil {
		return err
	}
	a.SetNotifier(emitNotify)
	app = a
	return nil
}

func handleRequest(request []byte) []byte {
	appMu.RLock()
	a := app
	appMu.RUnlock()
	if a == nil {
		return errorEnvelope(request, "engine not started")
	}
	resp, err := a.Handle(ctx, request)
	if err != nil {
		return errorEnvelope(request, err.Error())
	}
	if resp == nil {
		return []byte(`{}`)
	}
	return resp
}

func stopEngine() error {
	appMu.Lock()
	defer appMu.Unlock()
	if app == nil {
		return nil
	}
	err := app.Close()
	app = nil
	return err
}

func setGoNotifier(fn func(method string, params json.RawMessage)) {
	notifyMu.Lock()
	defer notifyMu.Unlock()
	goNotify = fn
}

func emitNotify(method string, params json.RawMessage) {
	if len(params) == 0 {
		params = json.RawMessage("null")
	}
	notifyMu.Lock()
	gn, cb := goNotify, cNotify
	notifyMu.Unlock()

	if gn != nil {
		gn(method, params)
	}
	if cb != nil {
		cm := C.CString(method)
		cp := C.CString(string(params))
		C.linetta_invoke_notify(cb, cm, cp)
		C.free(unsafe.Pointer(cm))
		C.free(unsafe.Pointer(cp))
	}
}

func errorEnvelope(request []byte, msg string) []byte {
	var probe struct {
		ID json.RawMessage `json:"id"`
	}
	_ = json.Unmarshal(request, &probe)
	id := probe.ID
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	out, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    -32603,
			"message": msg,
		},
	})
	return out
}

func goString(s *C.char) string {
	if s == nil {
		return ""
	}
	return C.GoString(s)
}

//export LinettaEngineStart
func LinettaEngineStart(home *C.char) C.int {
	if err := startEngine(goString(home)); err != nil {
		return C.int(1)
	}
	return C.int(0)
}

//export LinettaEngineCall
func LinettaEngineCall(request *C.char) *C.char {
	return C.CString(string(handleRequest([]byte(goString(request)))))
}

//export LinettaEngineFreeCString
func LinettaEngineFreeCString(s *C.char) {
	if s != nil {
		C.free(unsafe.Pointer(s))
	}
}

//export LinettaEngineStop
func LinettaEngineStop() C.int {
	if err := stopEngine(); err != nil {
		return C.int(1)
	}
	return C.int(0)
}

//export LinettaEngineSetNotifyCallback
func LinettaEngineSetNotifyCallback(cb C.LinettaNotifyCallback) {
	notifyMu.Lock()
	defer notifyMu.Unlock()
	cNotify = cb
}

func main() {}
