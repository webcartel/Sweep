package guard

import "strings"

// cleanPath gives a Windows path the one spelling Windows itself would use:
// either separator, "." and ".." resolved, a trailing separator gone, and the
// dots and spaces Windows ignores dropped from the end of every name.
//
// It is written here rather than taken from path/filepath because the guard
// has to read a path the same way on every machine that runs it: the tests
// and the fast CI job run on Linux, where filepath reads "/" and leaves a
// backslash alone.
func cleanPath(path string) string {
	path = strings.ReplaceAll(path, "/", `\`)
	volume, rest := splitVolume(path)

	segments := make([]string, 0, 8)
	for _, segment := range strings.Split(rest, `\`) {
		switch segment {
		case "", ".":
			continue
		case "..":
			if len(segments) > 0 {
				segments = segments[:len(segments)-1]
			}
			continue
		}
		segments = append(segments, strings.TrimRight(segment, ". "))
	}

	cleaned := strings.Join(segments, `\`)
	switch {
	case volume == "":
		return cleaned
	case cleaned == "":
		return volume + `\`
	default:
		return volume + `\` + cleaned
	}
}

// stripDevicePrefix takes off the \\?\ that lets a path be longer than
// MAX_PATH, and turns the UNC spelling of one back into a share.
func stripDevicePrefix(path string) string {
	if rest, ok := strings.CutPrefix(path, `\\?\UNC\`); ok {
		return `\\` + rest
	}
	return strings.TrimPrefix(path, `\\?\`)
}

// splitVolume takes off the drive letter or the UNC share a path starts with,
// and answers the rest of the path.
func splitVolume(path string) (volume, rest string) {
	if len(path) >= 2 && path[1] == ':' && isLetter(path[0]) {
		return strings.ToUpper(string(path[0])) + ":", strings.TrimPrefix(path[2:], `\`)
	}
	if strings.HasPrefix(path, `\\`) {
		parts := strings.SplitN(path[2:], `\`, 3)
		if len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
			volume = `\\` + parts[0] + `\` + parts[1]
			if len(parts) == 3 {
				return volume, parts[2]
			}
			return volume, ""
		}
	}
	return "", path
}

// isAbsPath says whether a path starts at a drive root or at a share, which is
// what Windows calls an absolute path.
func isAbsPath(path string) bool {
	volume, _ := splitVolume(path)
	return volume != ""
}

// joinPath puts a name inside a directory.
func joinPath(dir, name string) string {
	if strings.HasSuffix(dir, `\`) {
		return dir + name
	}
	return dir + `\` + name
}

// dirPath is the directory that holds a path.
func dirPath(path string) string {
	volume, rest := splitVolume(path)
	if rest == "" {
		return path
	}
	if index := strings.LastIndex(rest, `\`); index >= 0 {
		return volume + `\` + rest[:index]
	}
	return volume + `\`
}

// pathKey is the key two spellings of one path share: comparisons ignore case,
// the way Windows compares paths.
func pathKey(path string) string { return strings.ToLower(path) }

// quote puts a path in the quotation marks a message uses. The path is not
// escaped the way %q would escape it: a path in a report has to read the way
// Windows writes it.
func quote(path string) string { return `"` + path + `"` }

// below says whether a path is inside a directory: it starts with the
// directory and then a separator, so that C:\WindowsX is not inside C:\Windows.
func below(path, dir string) bool {
	prefix := pathKey(strings.TrimSuffix(dir, `\`)) + `\`
	return strings.HasPrefix(pathKey(path), prefix)
}

// driveRootOf is the root of the drive a path is on ("D:\"), when the path
// starts with one. The drive is read from the path itself: the guard does not
// ask Windows for the list of drives.
func driveRootOf(path string) (string, bool) {
	volume, _ := splitVolume(path)
	if len(volume) == 2 && volume[1] == ':' {
		return volume + `\`, true
	}
	return "", false
}

// isLetter says whether a byte is an ASCII letter.
func isLetter(char byte) bool {
	return (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z')
}
