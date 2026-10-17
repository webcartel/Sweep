// Package rules publishes the built-in rule files. The binary embeds the same
// files the repository publishes, so a rule and the binary that ships it cannot
// drift apart.
package rules

import "embed"

// FS holds every built-in rule file under its path below the rules directory:
// browsers/chrome.yaml, system/temp.yaml, dev/npm.yaml and so on.
//
// The pattern names the files rather than the directories, so a category that
// holds no rule yet (apps/) does not break the build and a new rule file is
// picked up without an edit here.
//
//go:embed */*.yaml
var FS embed.FS
