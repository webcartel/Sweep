// Package explore measures what takes space under a root, so that a person
// can see the largest folders and files and decide what to remove. It does not
// decide what is junk: it measures, and the caller chooses.
//
// The walk never follows a reparse point (a junction or a symbolic link), so a
// link back up the tree cannot double the count or loop forever. A folder the
// process may not read is counted as unread rather than as an error: on a
// whole drive that is the normal state of a few folders, not a failure.
package explore

import (
	"context"
	"errors"
	"sort"

	"github.com/webcartel/sweep/internal/platform"
)

// Node is one directory or file in the measured tree.
type Node struct {
	Path string
	Name string
	// Size is the whole subtree of a directory, and the file itself for a
	// file.
	Size int64
	// Files counts the files under the node, itself included for a file.
	Files int
	IsDir bool
	// Link marks a junction or a symbolic link: it was measured as nothing
	// and was not entered.
	Link bool
	// Denied counts the entries under the node that could not be read.
	Denied int
	// Kids are the children kept for the screen, largest first. A directory
	// deeper than the display depth has none, though its Size counts it all.
	Kids []*Node
}

// Options tunes one walk.
type Options struct {
	// Depth is how many levels below the root carry children for the screen.
	// The walk itself always goes all the way down: a folder's size is its
	// whole subtree.
	Depth int
	// TopFiles is how many of the largest files to keep. Zero keeps none.
	TopFiles int
	// Skip, when set, says whether a path is left alone entirely. The CLI
	// passes the guard's list here.
	Skip func(path string) bool
	// Progress, when set, is called with the running totals.
	Progress func(Progress)
}

// Progress is what a walk has seen so far.
type Progress struct {
	Dirs  int
	Files int
	Bytes int64
}

// Result is what one walk found.
type Result struct {
	Root *Node
	// Top holds the largest files, largest first, at most Options.TopFiles
	// of them.
	Top []*Node
	// Progress is the last total the walk reached.
	Progress Progress
}

// Scan measures the tree under root. A cancelled context stops the walk and
// returns what it had measured by then together with the context's error, so a
// caller may show the partial tree rather than nothing.
func Scan(ctx context.Context, fsys platform.FS, root string, opt Options) (Result, error) {
	if opt.Depth < 0 {
		opt.Depth = 0
	}
	if opt.Skip != nil && opt.Skip(root) {
		return Result{}, errors.New("the root is a path sweep leaves alone: " + root)
	}
	w := &walker{ctx: ctx, fs: fsys, opt: opt}
	node := w.measure(root, 0)
	if node != nil {
		w.result.Root = node
	}
	w.result.Progress = w.progress
	w.result.Top = w.top()
	if err := ctx.Err(); err != nil {
		return w.result, err
	}
	if node == nil {
		// The walk could not even name the root. The reason matters: a path
		// that is not there and a folder without access send the user to
		// different fixes.
		if _, statErr := fsys.Lstat(root); statErr != nil {
			return w.result, statErr
		}
		return w.result, errors.New("nothing could be read under " + root)
	}
	return w.result, nil
}

// walker carries what one walk collected.
type walker struct {
	ctx      context.Context
	fs       platform.FS
	opt      Options
	result   Result
	progress Progress
	// biggest holds the largest files seen so far, unsorted until the end.
	biggest []*Node
}

// measure answers the node of one path, or nil when it could not be read at
// all. A path below the display depth still gets a node, just without kids.
func (w *walker) measure(path string, depth int) *Node {
	if w.ctx.Err() != nil {
		return nil
	}
	info, err := w.fs.Lstat(path)
	if err != nil {
		return nil
	}

	node := &Node{Path: path, Name: info.Name, IsDir: info.IsDir}
	if node.Name == "" {
		node.Name = lastElement(path)
	}
	if !info.IsDir {
		node.Size = info.Size
		node.Files = 1
		w.countFile(node)
		return node
	}
	if info.Reparse {
		node.Link = true
		w.progress.Dirs++
		w.report()
		return node
	}

	entries, err := w.fs.ReadDir(path)
	if err != nil {
		node.Denied++
		w.progress.Dirs++
		w.report()
		return node
	}
	w.progress.Dirs++

	for _, entry := range entries {
		if w.ctx.Err() != nil {
			break
		}
		if w.opt.Skip != nil && w.opt.Skip(entry.Path) {
			continue
		}
		child := w.measure(entry.Path, depth+1)
		if child == nil {
			// The entry is there but could not be read: an unread child is
			// still part of what the folder holds.
			node.Denied++
			continue
		}
		node.Size += child.Size
		node.Files += child.Files
		node.Denied += child.Denied
		if depth < w.opt.Depth {
			node.Kids = append(node.Kids, child)
		}
	}
	sortNodes(node.Kids)
	w.report()
	return node
}

// countFile adds one file to the totals and to the largest-files list.
func (w *walker) countFile(node *Node) {
	w.progress.Files++
	w.progress.Bytes += node.Size
	if w.opt.TopFiles <= 0 {
		return
	}
	w.biggest = append(w.biggest, node)
	if len(w.biggest) > w.opt.TopFiles*2 {
		sortNodes(w.biggest)
		w.biggest = w.biggest[:w.opt.TopFiles]
	}
}

// top answers the largest files, largest first, capped at the limit.
func (w *walker) top() []*Node {
	sortNodes(w.biggest)
	if len(w.biggest) > w.opt.TopFiles && w.opt.TopFiles > 0 {
		w.biggest = w.biggest[:w.opt.TopFiles]
	}
	return w.biggest
}

// report calls the progress hook, if the caller gave one.
func (w *walker) report() {
	if w.opt.Progress != nil {
		w.opt.Progress(w.progress)
	}
}

// sortNodes puts the largest first, and the name orders the equal sizes so a
// screen does not reshuffle between two walks.
func sortNodes(nodes []*Node) {
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].Size != nodes[j].Size {
			return nodes[i].Size > nodes[j].Size
		}
		return nodes[i].Name < nodes[j].Name
	})
}

// lastElement is the name of the last segment of a path, for the systems that
// do not answer one.
func lastElement(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '\\' || path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}
