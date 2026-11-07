package vault

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/vaultbundle"
)

func (d *device) say(t *testing.T, id, messageID, text string) {
	t.Helper()
	exec(t, d.db, `INSERT INTO companion_messages (id, project_id, role, intent, content, created_at) VALUES (?, ?, 'user', 'agent', ?, 1)`,
		messageID, id, text)
}

func (d *device) messages(t *testing.T, id string) []string {
	t.Helper()
	rows, err := d.db.Query(`SELECT id FROM companion_messages WHERE project_id = ? ORDER BY id`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var messageID string
		if err := rows.Scan(&messageID); err != nil {
			t.Fatal(err)
		}
		out = append(out, messageID)
	}
	return out
}

// lastSyncedBeforeConversations makes the work's link look the way a device
// that synced it before conversations were carried left it.
func (d *device) lastSyncedBeforeConversations(t *testing.T, id string) {
	t.Helper()
	bundle, err := vaultbundle.Export(context.Background(), d.db, d.home, id, time.UnixMilli(1), []string{vaultbundle.CategoryConversations})
	if err != nil {
		t.Fatal(err)
	}
	exec(t, d.db, `UPDATE vault_items SET last_hash = ? WHERE project_id = ?`, bundle.Manifest.ContentHash(), id)
}

func TestConversationTravelsAndItsClearingToo(t *testing.T) {
	ctx := context.Background()
	_, a, b := devices(t)
	a.say(t, workID, "m-a", "첫 대화")
	if got := a.state(t, workID); got != StateLocalChanges {
		t.Fatalf("a = %s", got)
	}
	if _, err := a.svc.Push(ctx, workID); err != nil {
		t.Fatal(err)
	}
	if _, err := b.svc.Pull(ctx, PullInput{ProjectID: workID}); err != nil {
		t.Fatal(err)
	}
	if got := b.messages(t, workID); !slices.Equal(got, []string{"m-a"}) {
		t.Fatalf("b's conversation = %v", got)
	}

	exec(t, b.db, `DELETE FROM companion_messages WHERE project_id = ?`, workID)
	if _, err := b.svc.Push(ctx, workID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.svc.Pull(ctx, PullInput{ProjectID: workID}); err != nil {
		t.Fatal(err)
	}
	if got := a.messages(t, workID); len(got) != 0 {
		t.Fatalf("a cleared conversation came back: %v", got)
	}
}

// Both devices kept their own conversation while it stayed on the device. The
// first sync that carries it must not call that a conflict, nor lose either.
func TestFirstSyncWithConversationsKeepsWhatEachDeviceHad(t *testing.T) {
	ctx := context.Background()
	_, a, b := devices(t)
	a.lastSyncedBeforeConversations(t, workID)
	b.lastSyncedBeforeConversations(t, workID)
	a.say(t, workID, "m-a", "에이에서 나눈 말")
	b.say(t, workID, "m-b", "비에서 나눈 말")

	if _, err := a.svc.Push(ctx, workID); err != nil {
		t.Fatal(err)
	}
	if got := b.state(t, workID); got != StateRemoteChanges {
		t.Fatalf("b = %s, want remote changes", got)
	}
	if _, err := b.svc.Pull(ctx, PullInput{ProjectID: workID}); err != nil {
		t.Fatal(err)
	}
	if got := b.messages(t, workID); !slices.Equal(got, []string{"m-a", "m-b"}) {
		t.Fatalf("b = %v", got)
	}
	if got := b.state(t, workID); got != StateLocalChanges {
		t.Fatalf("b after the merge = %s, want local changes", got)
	}
	if _, err := b.svc.Push(ctx, workID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.svc.Pull(ctx, PullInput{ProjectID: workID}); err != nil {
		t.Fatal(err)
	}
	if got := a.messages(t, workID); !slices.Equal(got, []string{"m-a", "m-b"}) {
		t.Fatalf("a = %v", got)
	}
	var found int
	if err := a.db.QueryRow(`SELECT COUNT(*) FROM companion_messages_fts WHERE message_id = 'm-b'`).Scan(&found); err != nil || found != 1 {
		t.Fatalf("the agent cannot search the merged message: %d %v", found, err)
	}
	if a.state(t, workID) != StateSynced || b.state(t, workID) != StateSynced {
		t.Fatalf("states: %s, %s", a.state(t, workID), b.state(t, workID))
	}
}

// An edit made here is still an edit: carrying conversations does not let a
// pull run over it.
func TestEditBesideTheConversationIsStillAConflict(t *testing.T) {
	ctx := context.Background()
	_, a, b := devices(t)
	b.lastSyncedBeforeConversations(t, workID)
	a.edit(t, workID, "에이의 고침")
	if _, err := a.svc.Push(ctx, workID); err != nil {
		t.Fatal(err)
	}
	b.say(t, workID, "m-b", "비에서 나눈 말")
	b.edit(t, workID, "비의 고침")
	if got := b.state(t, workID); got != StateConflict {
		t.Fatalf("b = %s, want conflict", got)
	}
	_, err := b.svc.Pull(ctx, PullInput{ProjectID: workID})
	wantReason(t, err, "vault_local_changes")
}
