//go:build mobile

package engineapp

// iOS keeps the library in the app's private container. No file manager can
// open the data or backup folder there, and the folder picker has no folder to
// sync to — the daily backup already covers that container. The UI hides
// those actions on this flag.
const foldersAvailable = false
