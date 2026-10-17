// Package platform isolates every bit of Windows access behind the FS, Env and
// Procs interfaces, so the scanner and the executor run on fakes in tests.
// Implementations that call WinAPI carry a //go:build windows tag.
package platform
