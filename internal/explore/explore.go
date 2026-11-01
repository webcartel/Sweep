// Package explore measures what takes space under a root, so that a person
// can see the largest folders and files and decide what to remove. It does not
// decide what is junk: it measures, and the caller chooses.
//
// The walk never follows a reparse point (a junction or a symbolic link), so a
// link back up the tree cannot double the count or loop forever. A folder the
// process may not read is counted as unread rather than as an error: on a
// whole drive that is the normal state of a few folders, not a failure.
//
// Directories are read by a pool: a slot guards each ReadDir so that the disk
// is not flooded, and a second, wider permit guards the number of goroutines,
// so a tree with a hundred thousand folders cannot turn into a hundred
// thousand stacks. A parent gives its slot back before it waits for its
// children; holding it would deadlock the pool.
package explore

import (
	"context"
	"errors"
	"runtime"
	"sort"
	"sync"

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
	// Workers is how many directories are read at once. Zero asks for the
	// number of processors.
	Workers int
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

// measured is what one path came to. A path the caller skips is told apart
// from a path that could not be read, because only the second is unread.
type measured struct {
	node    *Node
	skipped bool
}

// Scan measures the tree under root. A cancelled context stops the walk and
// returns what it had measured by then together with the context's error, so a
// caller may show the partial tree rather than nothing.
func Scan(ctx context.Context, fsys platform.FS, root string, opt Options) (Result, error) {
	if opt.Depth < 0 {
		opt.Depth = 0
	}
	if opt.Workers <= 0 {
		opt.Workers = runtime.NumCPU()
	}
	if opt.Skip != nil && opt.Skip(root) {
		return Result{}, errors.New("the root is a path sweep leaves alone: " + root)
	}

	w := &walker{
		ctx: ctx,
		fs:  fsys,
		opt: opt,
		// One slot per directory read.
		slots: make(chan struct{}, opt.Workers),
		// Several goroutines may be blocked on a slot; the permits cap how
		// many there can be.
		permits: make(chan struct{}, opt.Workers*8),
	}
	node := w.measure(root, 0)
	if node != nil {
		w.result.Root = node
	}
	w.mu.Lock()
	w.result.Progress = w.progress
	w.result.Top = w.topLocked()
	w.mu.Unlock()

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
	ctx     context.Context
	fs      platform.FS
	opt     Options
	slots   chan struct{}
	permits chan struct{}

	mu       sync.Mutex
	result   Result
	progress Progress
	// biggest holds the largest files seen so far, unsorted until the end.
	biggest []*Node
}

// measure answers the node of one path, or nil when it could not be read or
// was skipped.
func (w *walker) measure(path string, depth int) *Node {
	return w.measureOne(path, depth).node
}

// measureOne is measure with the reason it came back empty.
func (w *walker) measureOne(path string, depth int) measured {
	if w.ctx.Err() != nil {
		return measured{}
	}
	if w.opt.Skip != nil && w.opt.Skip(path) {
		return measured{skipped: true}
	}
	info, err := w.fs.Lstat(path)
	if err != nil {
		return measured{}
	}

	node := &Node{Path: path, Name: info.Name, IsDir: info.IsDir}
	if node.Name == "" {
		node.Name = lastElement(path)
	}
	if !info.IsDir {
		node.Size = info.Size
		node.Files = 1
		w.countFile(node)
		return measured{node: node}
	}
	if info.Reparse {
		node.Link = true
		w.countDir()
		return measured{node: node}
	}

	// Read the entries under a slot, then give the slot back: a parent that
	// keeps its slot while it waits for its children would deadlock the pool.
	w.slots <- struct{}{}
	entries, err := w.fs.ReadDir(path)
	<-w.slots
	if err != nil {
		node.Denied++
		w.countDir()
		return measured{node: node}
	}
	w.countDir()

	children := make([]measured, len(entries))
	var wg sync.WaitGroup
	for i, entry := range entries {
		if w.ctx.Err() != nil {
			break
		}
		if entry.IsDir && w.spawn(&wg, children, i, entry.Path, depth+1) {
			continue
		}
		children[i] = w.measureOne(entry.Path, depth+1)
	}
	wg.Wait()

	for _, child := range children {
		switch {
		case child.skipped:
		case child.node == nil:
			// The entry is there but could not be read: an unread child is
			// still part of what the folder holds.
			node.Denied++
		default:
			node.Size += child.node.Size
			node.Files += child.node.Files
			node.Denied += child.node.Denied
			if depth < w.opt.Depth {
				node.Kids = append(node.Kids, child.node)
			}
		}
	}
	sortNodes(node.Kids)
	return measured{node: node}
}

// spawn walks one child in its own goroutine while a permit is free, and
// answers false when the pool is full: the caller then walks it inline, so the
// tree never turns into more goroutines than the permits allow.
func (w *walker) spawn(wg *sync.WaitGroup, children []measured, i int, path string, depth int) bool {
	select {
	case w.permits <- struct{}{}:
	default:
		return false
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() { <-w.permits }()
		children[i] = w.measureOne(path, depth)
	}()
	return true
}

// countFile adds one file to the totals and to the largest-files list.
func (w *walker) countFile(node *Node) {
	w.mu.Lock()
	defer w.mu.Unlock()
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

// countDir moves the folder counter and tells the caller how far the walk got.
func (w *walker) countDir() {
	w.mu.Lock()
	w.progress.Dirs++
	progress := w.progress
	w.mu.Unlock()
	if w.opt.Progress != nil {
		w.opt.Progress(progress)
	}
}

// topLocked answers the largest files, largest first, capped at the limit.
// The caller holds the lock.
func (w *walker) topLocked() []*Node {
	sortNodes(w.biggest)
	if len(w.biggest) > w.opt.TopFiles && w.opt.TopFiles > 0 {
		w.biggest = w.biggest[:w.opt.TopFiles]
	}
	return w.biggest
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
