package main

/*
#include <stdlib.h>
typedef void (*ClioNotifyCallback)(const char* method, const char* params);
static inline void clio_invoke_notify(ClioNotifyCallback cb, const char* m, const char* p) { cb(m, p); }
*/
import "C"

import (
	"context"
	"encoding/json"
	"sync"
	"unsafe"

	"github.com/devlikebear/linetta/engine/internal/clio/app"
	"github.com/devlikebear/linetta/engine/internal/settings"
)

var (
	appMu  sync.RWMutex
	engine *app.App
	ctx    = context.Background()

	notifyMu sync.Mutex
	cNotify  C.ClioNotifyCallback
	goNotify func(method string, params json.RawMessage)
)

func startEngine(home string) error {
	appMu.Lock()
	defer appMu.Unlock()
	if engine != nil {
		return nil
	}
	a, err := app.Open(ctx, app.Options{Home: home, Secrets: settings.PlatformSecretStore("devlikebear.clio")})
	if err != nil {
		return err
	}
	a.SetNotifier(emitNotify)
	engine = a
	return nil
}

func handleRequest(request []byte) []byte {
	appMu.RLock()
	a := engine
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
	if engine == nil {
		return nil
	}
	err := engine.Close()
	engine = nil
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
		C.clio_invoke_notify(cb, cm, cp)
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

//export ClioEngineStart
func ClioEngineStart(home *C.char) C.int {
	if err := startEngine(goString(home)); err != nil {
		return C.int(1)
	}
	return C.int(0)
}

//export ClioEngineCall
func ClioEngineCall(request *C.char) *C.char {
	return C.CString(string(handleRequest([]byte(goString(request)))))
}

//export ClioEngineFreeCString
func ClioEngineFreeCString(s *C.char) {
	if s != nil {
		C.free(unsafe.Pointer(s))
	}
}

//export ClioEngineStop
func ClioEngineStop() C.int {
	if err := stopEngine(); err != nil {
		return C.int(1)
	}
	return C.int(0)
}

//export ClioEngineSetNotifyCallback
func ClioEngineSetNotifyCallback(cb C.ClioNotifyCallback) {
	notifyMu.Lock()
	defer notifyMu.Unlock()
	cNotify = cb
}

func main() {}
