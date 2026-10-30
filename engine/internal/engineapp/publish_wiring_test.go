//go:build !mobile

package engineapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/publish"
)

// A writer who never connects is not affected by publishing at all: every
// publish.* method answers, and none of them reaches the site.
func TestPublishMethodsAreRegisteredAndSendNothingUntilConnected(t *testing.T) {
	var hits atomic.Int32
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusTeapot)
	}))
	defer site.Close()
	t.Setenv(publish.BaseURLEnv, site.URL)
	app := openApp(t)

	res, rpcErr := call(t, app, "diagnostics.version", "")
	if rpcErr != nil || !strings.Contains(string(res), `"publish_available":true`) {
		t.Fatalf("diagnostics.version: %s %v", res, rpcErr)
	}

	res, rpcErr = call(t, app, "publish.connect_status", "")
	if rpcErr != nil {
		t.Fatalf("publish.connect_status: %v", rpcErr)
	}
	var st publish.ConnectStatus
	if err := json.Unmarshal(res, &st); err != nil || st.State != publish.StateDisconnected {
		t.Fatalf("connect_status = %s", res)
	}

	res, rpcErr = call(t, app, "projects.create", `{"title":"작품","genres":[],"length_target":"series","default_pov":"first","outline_preset":"webnovel"}`)
	if rpcErr != nil {
		t.Fatalf("projects.create: %v", rpcErr)
	}
	var p struct {
		ID               string `json:"id"`
		LastOpenedNodeID string `json:"last_opened_node_id"`
	}
	_ = json.Unmarshal(res, &p)

	res, rpcErr = call(t, app, "publish.work_status", `{"project_id":"`+p.ID+`"}`)
	if rpcErr != nil {
		t.Fatalf("publish.work_status: %v", rpcErr)
	}
	var ws publish.WorkStatus
	if err := json.Unmarshal(res, &ws); err != nil || ws.Connected || len(ws.Episodes) != 1 {
		t.Fatalf("work_status = %s", res)
	}

	_, rpcErr = call(t, app, "publish.episode_publish", `{"node_id":"`+p.LastOpenedNodeID+`"}`)
	if rpcErr == nil || !strings.Contains(string(rpcErr.Data), "publish_not_connected") {
		t.Fatalf("episode_publish: %+v", rpcErr)
	}
	_, rpcErr = call(t, app, "publish.work_upsert", `{"project_id":"`+p.ID+`","blurb":"소개"}`)
	if rpcErr == nil || !strings.Contains(string(rpcErr.Data), "publish_not_connected") {
		t.Fatalf("work_upsert: %+v", rpcErr)
	}
	_, rpcErr = call(t, app, "publish.episode_set_visibility", `{"project_id":"`+p.ID+`","node_id":"n","visibility":"private"}`)
	if rpcErr == nil || !strings.Contains(string(rpcErr.Data), "publish_not_connected") {
		t.Fatalf("episode_set_visibility: %+v", rpcErr)
	}
	_, rpcErr = call(t, app, "publish.cover_publish", `{"project_id":"`+p.ID+`","illustration_id":"i"}`)
	if rpcErr == nil || !strings.Contains(string(rpcErr.Data), "publish_not_connected") {
		t.Fatalf("cover_publish: %+v", rpcErr)
	}
	_, rpcErr = call(t, app, "publish.cover_publish", `{"project_id":"`+p.ID+`"}`)
	if rpcErr == nil || !strings.Contains(rpcErr.Message, "illustration_id required") {
		t.Fatalf("cover_publish with no picture: %+v", rpcErr)
	}
	// Without the writer's consent the connect flow does not even start.
	_, rpcErr = call(t, app, "publish.connect_start", `{}`)
	if rpcErr == nil || !strings.Contains(string(rpcErr.Data), "publish_consent_required") {
		t.Fatalf("connect_start without consent: %+v", rpcErr)
	}
	if _, rpcErr := call(t, app, "publish.disconnect", ""); rpcErr != nil {
		t.Fatalf("publish.disconnect: %v", rpcErr)
	}

	if n := hits.Load(); n != 0 {
		t.Fatalf("the app sent %d requests to the site before connecting", n)
	}
}
