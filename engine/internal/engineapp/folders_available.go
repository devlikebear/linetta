//go:build !mobile

package engineapp

// The writer can reach folders on this device, so the UI offers folder sync
// and opens the data and backup folders.
const foldersAvailable = true
