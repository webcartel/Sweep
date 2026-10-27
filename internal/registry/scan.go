package registry

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/webcartel/sweep/internal/platform"
)

// The branches scanning reads. The WOW6432Node spelling is listed literally:
// every key is read through the 64-bit view, so the name is not a second
// view but a path.
const (
	uninstallHKCU = `HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Uninstall`
	uninstallHKLM = `HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`
	uninstallWow  = `HKEY_LOCAL_MACHINE\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`

	runHKCU = `HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Run`
	runHKLM = `HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\Run`
	runWow  = `HKEY_LOCAL_MACHINE\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Run`

	appPathsHKCU = `HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\App Paths`
	appPathsHKLM = `HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths`
	appPathsWow  = `HKEY_LOCAL_MACHINE\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\App Paths`
)

// Scan reads the three branches and answers the findings, plus the notes of
// what it had to skip: an unknown variable, a drive that is not local, a key
// that could not be read.
func Scan(reg platform.Registry, fsys platform.FS, env platform.Env) ([]Finding, []string, error) {
	c := &checker{reg: reg, fs: fsys, env: env}
	for _, parent := range []string{uninstallHKCU, uninstallHKLM, uninstallWow} {
		c.uninstall(parent)
	}
	for _, parent := range []string{runHKCU, runHKLM, runWow} {
		c.run(parent)
	}
	for _, parent := range []string{appPathsHKCU, appPathsHKLM, appPathsWow} {
		c.appPaths(parent)
	}
	return c.findings, c.notes, nil
}

// checker carries what one scan collected.
type checker struct {
	reg      platform.Registry
	fs       platform.FS
	env      platform.Env
	findings []Finding
	notes    []string
}

// note remembers a skip worth telling the user about.
func (c *checker) note(format string, args ...any) {
	c.notes = append(c.notes, fmt.Sprintf(format, args...))
}

// uninstall walks the keys of an Uninstall branch.
func (c *checker) uninstall(parent string) {
	keys, err := c.reg.SubKeys(parent)
	if err != nil {
		return
	}
	for _, key := range keys {
		values, err := c.reg.Values(key)
		if err != nil {
			c.note("cannot read %s: %v", key, err)
			continue
		}
		byName := valueMap(values)
		if skipUninstall(byName) {
			continue
		}
		display := byName["displayname"].Data
		command := byName["uninstallstring"].Data
		if command == "" {
			command = byName["quietuninstallstring"].Data
		}
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(command)), "msiexec") {
			continue
		}
		path, state := c.command(command)
		if state == stateSkip {
			continue
		}
		if state == stateLive {
			continue
		}
		subkeys, _ := c.reg.SubKeys(key)
		c.findings = append(c.findings, Finding{
			Kind:       KindUninstall,
			ID:         findingID(KindUninstall, key, ""),
			Name:       display,
			Key:        key,
			Path:       path,
			Reason:     "uninstall command not found: " + path,
			Admin:      isAdminKey(key),
			HasSubkeys: len(subkeys) != 0,
		})
	}
}

// skipUninstall says whether an Uninstall key is out of scope: an update, an
// MSI entry, a system component, or an entry Windows uses itself.
func skipUninstall(values map[string]platform.Value) bool {
	if values["displayname"].Data == "" {
		return true
	}
	for _, name := range []string{"systemcomponent", "windowsinstaller"} {
		if value := values[name]; value.Data == "1" || strings.EqualFold(value.Data, "true") {
			return true
		}
	}
	for _, name := range []string{"parentkeyname", "releasetype", "isMinorUpdate"} {
		if _, ok := values[strings.ToLower(name)]; ok {
			return true
		}
	}
	return false
}

// run walks the values of a Run branch. Every dead value is deleted alone;
// the key itself is never a target.
func (c *checker) run(parent string) {
	values, err := c.reg.Values(parent)
	if err != nil {
		return
	}
	for _, value := range values {
		if value.Name == "" || strings.TrimSpace(value.Data) == "" {
			continue
		}
		path, state := c.command(value.Data)
		if state == stateSkip || state == stateLive {
			continue
		}
		c.findings = append(c.findings, Finding{
			Kind:        KindRun,
			ID:          findingID(KindRun, parent, value.Name),
			Name:        value.Name,
			Key:         parent,
			Value:       value.Name,
			Path:        path,
			Reason:      "program not found: " + path,
			Admin:       isAdminKey(parent),
			DeleteValue: true,
		})
	}
}

// appPaths walks the keys of an App Paths branch; the default value of each
// key names the program.
func (c *checker) appPaths(parent string) {
	keys, err := c.reg.SubKeys(parent)
	if err != nil {
		return
	}
	for _, key := range keys {
		values, err := c.reg.Values(key)
		if err != nil {
			continue
		}
		byName := valueMap(values)
		command := byName[""].Data
		if command == "" {
			continue
		}
		path, state := c.command(command)
		if state == stateSkip || state == stateLive {
			continue
		}
		subkeys, _ := c.reg.SubKeys(key)
		name := strings.TrimPrefix(key, parent+`\`)
		c.findings = append(c.findings, Finding{
			Kind:       KindAppPath,
			ID:         findingID(KindAppPath, key, ""),
			Name:       name,
			Key:        key,
			Path:       path,
			Reason:     "program not found: " + path,
			Admin:      isAdminKey(key),
			HasSubkeys: len(subkeys) != 0,
		})
	}
}

// valueMap indexes values by their folded names.
func valueMap(values []platform.Value) map[string]platform.Value {
	out := make(map[string]platform.Value, len(values))
	for _, value := range values {
		out[strings.ToLower(value.Name)] = value
	}
	return out
}

// The states of one command: the path exists, the path is gone, or the
// command cannot be judged here.
type pathState int

const (
	stateSkip pathState = iota
	stateLive
	stateDead
)

// command answers the program path of a command line and whether it exists.
// Quotes are taken off; an unquoted path with spaces is tried from the
// longest prefix down, the way CreateProcess reads it.
func (c *checker) command(command string) (string, pathState) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", stateSkip
	}
	expanded := platform.Expand(command, c.env)
	if names := platform.EnvNames(expanded); len(names) != 0 {
		c.note("unknown variable %%%s%% in %q", names[0], command)
		return "", stateSkip
	}
	if strings.HasPrefix(expanded, `"`) {
		return c.judge(quotedPath(expanded))
	}
	// The program may carry spaces without quotes. The longest prefix that
	// exists is the program; Windows tries the same spelling.
	for _, candidate := range commandCandidates(expanded) {
		if !isAbsPath(candidate) {
			continue
		}
		if _, err := c.fs.Lstat(candidate); err == nil && c.fixedDrive(candidate) {
			return candidate, stateLive
		}
	}
	return c.judge(deadPath(expanded))
}

// judge answers whether the path is on a local disk and exists.
func (c *checker) judge(path string) (string, pathState) {
	if path == "" {
		return "", stateSkip
	}
	if !isAbsPath(path) {
		c.note("relative command %q cannot be read as a place on the disk", path)
		return path, stateSkip
	}
	if !c.fixedDrive(path) {
		return path, stateSkip
	}
	_, err := c.fs.Lstat(path)
	if err == nil {
		return path, stateLive
	}
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, os.ErrNotExist) {
		return path, stateDead
	}
	c.note("cannot read %q: %v", path, err)
	return path, stateSkip
}

// fixedDrive says whether the path sits on a local disk. A path on a
// removable or network disk is left alone: the disk may come back.
func (c *checker) fixedDrive(path string) bool {
	kind, err := c.fs.DriveType(path)
	if err != nil {
		c.note("cannot read the drive of %q: %v", path, err)
		return false
	}
	if kind != platform.DriveFixed {
		c.note("%q is not on a local disk; left alone", path)
		return false
	}
	return true
}

// quotedPath answers the first quoted argument.
func quotedPath(command string) string {
	if end := strings.Index(command[1:], `"`); end >= 0 {
		return command[1 : end+1]
	}
	return strings.Trim(command, `"`)
}

// deadPath answers the part of an unquoted command that looks like the
// program: through the .exe suffix when there is one, the first token
// otherwise.
func deadPath(command string) string {
	lower := strings.ToLower(command)
	if i := strings.Index(lower, ".exe"); i >= 0 {
		return strings.Trim(command[:i+4], `" `)
	}
	if i := strings.IndexByte(command, ' '); i >= 0 {
		return strings.Trim(command[:i], `"`)
	}
	return strings.Trim(command, `"`)
}

// commandCandidates answers the prefixes of an unquoted command line that
// may be the program path, longest first: the whole line, then each cut at a
// space.
func commandCandidates(command string) []string {
	command = strings.TrimSpace(strings.Trim(command, `"`))
	var out []string
	for i := len(command); i > 0; i-- {
		if i != len(command) && command[i] != ' ' {
			continue
		}
		out = append(out, command[:i])
	}
	return out
}

// isAbsPath says whether a path starts at a drive or a share.
func isAbsPath(path string) bool {
	if strings.HasPrefix(path, `\\`) {
		return true
	}
	return len(path) >= 3 && path[1] == ':' && path[2] == '\\'
}

// joinKey puts a value under its key for a note or a log line.
func joinKey(key, value string) string {
	if value == "" {
		return key
	}
	return filepath.Join(key, value)
}
