package rpc

import (
	"encoding/json"
	"errors"
)

// Reason codes name a failure the UI is expected to explain in the reader's
// own language. The Message on a MethodError stays English for logs and for
// clients that do not recognise the code; the code is what gets translated.
//
// Engine-side translation was the other option and is the wrong one here: the
// desktop app already carries a ko/en/ja catalogue, and Korean strings baked
// into handlers leak Korean to English and Japanese readers.
const (
	ReasonAIDataSharingConsentRequired = "ai_data_sharing_consent_required"
	ReasonMCPPortInUse                 = "mcp_port_in_use"
	ReasonMCPConsentRequired           = "mcp_consent_required"

	// The "not found" family. These are the failures a writer actually meets:
	// a scene deleted in another window, a character an agent removed, a
	// thread that went with the work it belonged to. The rest of the engine's
	// InvalidParams messages ("id required") only fire on a malformed call and
	// stay English for logs.
	ReasonNodeNotFound         = "node_not_found"
	ReasonProjectNotFound      = "project_not_found"
	ReasonImageNotFound        = "image_not_found"
	ReasonImageTooLarge        = "image_too_large"
	ReasonUnsupportedImageType = "unsupported_image_type"
	ReasonTooManyImages        = "too_many_images"
	ReasonEntityNotFound       = "entity_not_found"

	// Style rules the writer typed that the engine will not store (#162).
	// It refuses rather than trims, so these are things a writer meets and
	// has to be told in their own language.
	ReasonStyleTooManyPhrases = "style_too_many_phrases"
	ReasonStylePhraseTooLong  = "style_phrase_too_long"
	ReasonStyleSentenceLimit  = "style_sentence_limit"
	// Approving a style profile draft (#163): the draft another window
	// already dealt with, an approval the writer emptied, and notes that
	// would outgrow what a story brief carries.
	ReasonStyleNoDraft      = "style_no_draft"
	ReasonStyleProfileEmpty = "style_profile_empty"
	ReasonStyleNotesTooLong = "style_notes_too_long"

	// The writer's own direction (story-assist Phase 1): limits are refused
	// rather than trimmed, the same reason style's are above.
	ReasonDirectionFocusTooLong = "direction_focus_too_long"
	ReasonDirectionGoalTooLong  = "direction_goal_too_long"
	ReasonDirectionTooManyItems = "direction_too_many_items"
	ReasonDirectionItemTooLong  = "direction_item_too_long"
	ReasonDirectionNotScene     = "direction_not_scene"

	ReasonThreadNotFound       = "thread_not_found"
	ReasonBeatNotFound         = "beat_not_found"
	ReasonRelationshipNotFound = "relationship_not_found"
	ReasonNoteNotFound         = "note_not_found"
	ReasonFactCardNotFound     = "fact_card_not_found"

	// The built-in agent's provider layer (#90). The first two are states the
	// settings pane fixes; the rest are what the provider said, reduced to
	// something the reader can act on. The provider's raw body stays in the
	// English Message for logs and never becomes UI text.
	//
	// Unreachable means the network never got an answer. A timeout, a request
	// the provider refused (4xx) and an outage on its side (5xx) each have
	// their own code: all of them used to read "check your internet
	// connection", which sent a writer with a working connection looking for
	// a fault they did not have.
	ReasonProviderNotConfigured   = "provider_not_configured"
	ReasonProviderConsentRequired = "provider_consent_required"
	ReasonProviderAuthFailed      = "provider_auth_failed"
	ReasonProviderRateLimited     = "provider_rate_limited"
	ReasonProviderUnreachable     = "provider_unreachable"
	ReasonProviderTimeout         = "provider_timeout"
	ReasonProviderRejected        = "provider_rejected"
	ReasonProviderUnavailable     = "provider_unavailable"
	// A rejection that names the model: this account or plan is not served
	// the one asked for. Its own code because the fix is a different model,
	// not a retry.
	ReasonProviderModelUnsupported = "provider_model_unsupported"

	// Illustration drawing. The provider's credential and the failures above
	// are shared with the agent; these are what only drawing has. Off: no
	// image provider chosen. Model required: Gemini has no default image
	// model. Refused: a policy block, where the same prompt will not pass on
	// retry. Billing required: a Gemini key from a free-tier project, which
	// has no image quota at all — a rate limit it is not.
	ReasonImageGenerationOff     = "image_generation_off"
	ReasonImageConsentRequired   = "image_consent_required"
	ReasonImageModelRequired     = "image_model_required"
	ReasonImageRefused           = "image_refused"
	ReasonImageBillingRequired   = "image_billing_required"
	ReasonIllustrationNotFound   = "illustration_not_found"
	ReasonIllustrationTooLarge   = "illustration_too_large"
	ReasonTooManyIllustrations   = "too_many_illustrations"
	ReasonIllustrationBadContent = "illustration_bad_content"
	ReasonIllustrationInUse      = "illustration_in_use"

	// The Codex login (#92). The message has to name the two ports, because
	// the fix is closing whatever holds them — usually a Codex CLI login. A
	// failed login attempt is not an RPC error at all — codex.login_status
	// reports it as a Status field instead (see codexauth.Status.LoginFailed)
	// — so this is the only Codex reason code.
	ReasonCodexPortInUse = "codex_port_in_use"

	// The built-in agent's loop (#93). Busy is a state the writer resolves by
	// waiting or pressing stop; the iteration limit means the agent was cut
	// off mid-task and the reply says how far it got.
	ReasonAgentBusy           = "agent_busy"
	ReasonAgentIterationLimit = "agent_iteration_limit"

	// A panic inside the turn goroutine (#93 fix round 1). The turn ends
	// instead of taking the engine process down with it; the raw panic
	// value stays in the English Message for logs.
	ReasonAgentInternalError = "agent_internal_error"

	// A turn that ended without the model finishing. Truncated: the provider
	// cut the response off at its output ceiling. Empty: the last response
	// carried neither text nor a tool call. Both used to end as agent.done,
	// which the panel drew as nothing at all.
	ReasonAgentOutputTruncated = "agent_output_truncated"
	ReasonAgentEmptyReply      = "agent_empty_reply"

	// agent.undo asking to revert a batch that has fallen out of the
	// in-memory undo window (#93 fix round 1). This is not "not found" in the
	// writer-did-something-wrong sense — storyops keeps only the last few
	// batches, so a restart or a handful of later turns is enough to age one
	// out on its own. The writer can still reach the same change through
	// snapshot history.
	ReasonAgentUndoUnavailable = "agent_undo_unavailable"

	// Publishing to Linetta Works (Phase 3). Most are the site's own error
	// codes (docs/api.md in linetta-artworks) carried through; the site's
	// Korean message stays in the English Message for logs. The rest are
	// refusals the engine makes before anything is sent.
	//
	// Unauthorized is the site saying the token no longer works — the
	// writer disconnected this device from their profile — and the engine
	// has already forgotten it by the time the pane hears this. Not
	// connected is the engine never having had one.
	ReasonPublishNotConnected      = "publish_not_connected"
	ReasonPublishConsentRequired   = "publish_consent_required"
	ReasonPublishUnauthorized      = "publish_unauthorized"
	ReasonPublishNotEnabled        = "publish_not_enabled"
	ReasonPublishQuotaExceeded     = "publish_quota_exceeded"
	ReasonPublishTooLarge          = "publish_too_large"
	ReasonPublishInvalidSlug       = "publish_invalid_slug"
	ReasonPublishSlugTaken         = "publish_slug_taken"
	ReasonPublishWorkNotRegistered = "publish_work_not_registered"
	ReasonPublishEpisodeNotFound   = "publish_episode_not_found"
	ReasonPublishRateLimited       = "publish_rate_limited"
	ReasonPublishUnreachable       = "publish_unreachable"
	ReasonPublishTimeout           = "publish_timeout"
	ReasonPublishUnavailable       = "publish_unavailable"
	ReasonPublishRejected          = "publish_rejected"
	ReasonPublishNotEpisode        = "publish_not_episode"
	ReasonPublishEmptyEpisode      = "publish_empty_episode"
	ReasonPublishBlurbRequired     = "publish_blurb_required"
	// A picture in the episode is bigger than the site takes.
	ReasonPublishImageTooLarge = "publish_image_too_large"
	// The episode shows more pictures than the site takes (per episode or
	// per work).
	ReasonPublishTooManyImages = "publish_too_many_images"
)

// NotFound builds the error for a record the caller asked for and the engine
// could not find. Message stays English for logs; reason is what gets
// translated (see apps/desktop/src/lib/rpcMessage.ts).
func NotFound(reason, message string) *MethodError {
	return &MethodError{Code: CodeInvalidParams, Message: message, Data: ReasonData(reason)}
}

// ReasonData builds the MethodError Data payload carrying a reason code.
func ReasonData(reason string) json.RawMessage {
	encoded, err := json.Marshal(struct {
		Reason string `json:"reason"`
	}{Reason: reason})
	if err != nil {
		return nil
	}
	return encoded
}

// ReasonError carries a reason code out of a package that does not build
// MethodErrors itself (internal/provider must not know about JSON-RPC).
// Handlers turn it into one with MethodErrorFrom.
type ReasonError struct {
	Reason string
	Err    error
}

func (e *ReasonError) Error() string {
	if e.Err == nil {
		return e.Reason
	}
	return e.Reason + ": " + e.Err.Error()
}

func (e *ReasonError) Unwrap() error { return e.Err }

// MethodErrorFrom maps any error to a MethodError: a ReasonError anywhere in
// the chain becomes an InvalidParams error carrying its reason; anything else
// is an internal error with the message alone.
func MethodErrorFrom(err error) *MethodError {
	if err == nil {
		return &MethodError{Code: CodeInternalError}
	}
	var re *ReasonError
	if errors.As(err, &re) {
		return &MethodError{Code: CodeInvalidParams, Message: err.Error(), Data: ReasonData(re.Reason)}
	}
	return &MethodError{Code: CodeInternalError, Message: err.Error()}
}
