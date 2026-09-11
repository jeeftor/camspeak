package api

import "regexp"

// Stream URLs can carry credentials in userinfo, queries, or fragments. Keep
// these in the transport only, not in playback state or diagnostic output.
func streamDisplayURL(raw string) string {
	replay := playbackReplay("", map[string]any{"url": raw}, -1)
	return replay.Body["url"].(string)
}

var streamDiagnosticURL = regexp.MustCompile(`(?i)(?:https?|rtsp)://[^\s'"<>]+`)

func redactStreamDiagnostic(line string) string {
	return streamDiagnosticURL.ReplaceAllStringFunc(line, streamDisplayURL)
}

type streamDiagnosticError struct{ err error }

func (e streamDiagnosticError) Error() string { return redactStreamDiagnostic(e.err.Error()) }
func (e streamDiagnosticError) Unwrap() error { return e.err }
