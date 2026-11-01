package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/webcartel/sweep/internal/explore"
	"github.com/webcartel/sweep/internal/i18n"
	"github.com/webcartel/sweep/internal/report"
)

// exploreNode is one node of the JSON the overview prints.
type exploreNode struct {
	Path   string        `json:"path"`
	Name   string        `json:"name"`
	Size   int64         `json:"size"`
	Files  int           `json:"files"`
	IsDir  bool          `json:"is_dir"`
	Link   bool          `json:"link,omitempty"`
	Denied int           `json:"denied,omitempty"`
	Kids   []exploreNode `json:"kids,omitempty"`
}

// exploreAnswer is the whole `sweep explore --json` document.
type exploreAnswer struct {
	Schema int           `json:"schema"`
	Root   exploreNode   `json:"root"`
	Top    []exploreNode `json:"top"`
}

// newExploreCmd reads a folder and prints what takes the most room in it. It
// changes nothing: removing is what the interface is for.
func newExploreCmd(d deps) *cobra.Command {
	var depth, top int
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "explore [path]",
		Short: i18n.T("cli.explore.short"),
		Long:  i18n.T("cli.explore.long"),
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			root := ""
			if len(args) == 1 {
				root = args[0]
			} else if d.Env != nil {
				root, _ = d.Env.Get("USERPROFILE")
			}
			if root == "" {
				return usageError(fmt.Errorf("%s", i18n.T("cli.explore.nopath")))
			}

			result, err := explore.Scan(c.Context(), d.FS, root, explore.Options{
				Depth: depth, TopFiles: top,
			})
			if err != nil {
				return err
			}
			if asJSON {
				return printExploreJSON(c.OutOrStdout(), result)
			}
			printExplore(c.OutOrStdout(), result)
			return nil
		},
	}
	cmd.Flags().IntVar(&depth, "depth", 2, i18n.T("cli.explore.flag.depth"))
	cmd.Flags().IntVar(&top, "top", 20, i18n.T("cli.explore.flag.top"))
	cmd.Flags().BoolVar(&asJSON, "json", false, i18n.T("cli.explore.flag.json"))
	return cmd
}

// printExplore writes the tree, the totals and the largest files.
func printExplore(out io.Writer, result explore.Result) {
	if result.Root == nil {
		_, _ = fmt.Fprintln(out, i18n.T("cli.explore.nothing"))
		return
	}
	_, _ = fmt.Fprintln(out, i18n.T("cli.explore.header"))
	printExploreNode(out, result.Root, 0)

	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, fmt.Sprintf(i18n.T("cli.explore.total"),
		report.FormatBytes(result.Root.Size), result.Root.Files, result.Progress.Dirs))
	if result.Root.Denied != 0 {
		_, _ = fmt.Fprintln(out, fmt.Sprintf(i18n.T("cli.explore.denied"), result.Root.Denied))
	}

	if len(result.Top) != 0 {
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintln(out, i18n.T("cli.explore.top"))
		for _, file := range result.Top {
			_, _ = fmt.Fprintf(out, "  %10s  %s\n", report.FormatBytes(file.Size), file.Path)
		}
	}
}

// printExploreNode writes one row and, indented, what it holds.
func printExploreNode(out io.Writer, node *explore.Node, level int) {
	pad := strings.Repeat("  ", level)
	name := node.Name
	if node.Link {
		name += " " + i18n.T("cli.explore.link")
	}
	_, _ = fmt.Fprintf(out, "%s%-12s %10d  %s\n", pad, report.FormatBytes(node.Size), node.Files, name)
	for _, kid := range node.Kids {
		printExploreNode(out, kid, level+1)
	}
}

// printExploreJSON writes the one document of the run.
func printExploreJSON(out io.Writer, result explore.Result) error {
	answer := exploreAnswer{Schema: 1}
	if result.Root != nil {
		answer.Root = jsonOfNode(result.Root)
	}
	answer.Top = make([]exploreNode, 0, len(result.Top))
	for _, file := range result.Top {
		answer.Top = append(answer.Top, jsonOfNode(file))
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(answer)
}

// jsonOfNode copies a node, and its kept children, into the document's shape.
func jsonOfNode(node *explore.Node) exploreNode {
	out := exploreNode{
		Path: node.Path, Name: node.Name, Size: node.Size,
		Files: node.Files, IsDir: node.IsDir, Link: node.Link, Denied: node.Denied,
	}
	for _, kid := range node.Kids {
		out.Kids = append(out.Kids, jsonOfNode(kid))
	}
	return out
}
