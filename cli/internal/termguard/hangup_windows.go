//go:build windows

package termguard

// watchHangup does nothing on Windows, which has no hangup: a console closing
// under the program is CTRL_CLOSE_EVENT, which the runtime delivers as the
// SIGTERM Bubble Tea already handles.
func watchHangup(func(string)) func() {
	return func() {}
}
