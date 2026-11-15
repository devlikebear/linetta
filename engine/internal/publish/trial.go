package publish

import (
	"context"
	"log"
	"net/http"
)

// Trial states. The site reports the first four (linetta-artworks
// docs/api.md, "Linetta 체험"); the engine adds the two it can tell on its own.
const (
	TrialNotStarted   = "not_started"
	TrialAvailable    = "available"
	TrialExhausted    = "exhausted"
	TrialPaused       = "paused"
	TrialNotConnected = "not_connected"
	TrialUnreachable  = "unreachable"
)

// TrialStatus is how much of the Linetta trial this device's Works account has
// left. The token counts are zero unless the site answered.
type TrialStatus struct {
	State           string `json:"state"`
	GrantedTokens   int64  `json:"granted_tokens"`
	UsedTokens      int64  `json:"used_tokens"`
	RemainingTokens int64  `json:"remaining_tokens"`
}

// TrialStatus asks the site how much of the trial the token's account has left.
func (c *Client) TrialStatus(ctx context.Context, token string) (TrialStatus, error) {
	var out TrialStatus
	err := c.do(ctx, http.MethodGet, "/api/artworks/trial/me", token, nil, &out)
	return out, err
}

// TrialStatus reports where this device stands in the Linetta trial. It never
// fails: a device with no connection is not_connected without asking the
// site, and a site that cannot be reached is unreachable. A refused token is
// forgotten, as anywhere else the site refuses it, and reads not_connected.
func (s *Service) TrialStatus(ctx context.Context) TrialStatus {
	tok, err := s.token()
	if err != nil {
		return TrialStatus{State: TrialNotConnected}
	}
	st, err := s.client.TrialStatus(ctx, tok)
	if err != nil {
		if isUnauthorized(err) {
			s.connector.Revoked()
			return TrialStatus{State: TrialNotConnected}
		}
		log.Printf("publish: trial status: %v", err)
		return TrialStatus{State: TrialUnreachable}
	}
	switch st.State {
	case TrialNotStarted, TrialAvailable, TrialExhausted, TrialPaused:
		return st
	}
	// A state this release does not know is one it cannot offer the writer.
	log.Printf("publish: trial status: unknown state %q", st.State)
	return TrialStatus{State: TrialUnreachable}
}
