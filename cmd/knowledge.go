/*
Copyright © 2025 Joseph Goksu josephgoksu@gmail.com
*/
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/josephgoksu/TaskWing/internal/app"
	"github.com/josephgoksu/TaskWing/internal/config"
	"github.com/josephgoksu/TaskWing/internal/memory"
	"github.com/josephgoksu/TaskWing/internal/render"
	"github.com/josephgoksu/TaskWing/internal/ui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	knowledgeTypeFlag      string
	knowledgeWorkspaceFlag string
	knowledgeAllFlag       bool
)

var knowledgeCmd = &cobra.Command{
	Use:          "knowledge [type]",
	Short:        "View project knowledge nodes",
	SilenceUsage: true,
	Long: `View project knowledge captured in memory.

Without arguments, lists all nodes.
With a type argument or --type flag, filters to that type only.

Types: decision, feature, constraint, pattern, plan, note, metadata, documentation

Workspace Filtering (monorepo support):
  By default, lists nodes from all workspaces.
  Use --workspace to filter by a specific service/workspace.
  Use --all to explicitly show all workspaces.

Examples:
  taskwing knowledge
  taskwing knowledge decision
  taskwing knowledge --type decision
  taskwing knowledge --workspace=osprey`,
	Args: cobra.MaximumNArgs(1),
	RunE: runKnowledge,
}

func init() {
	rootCmd.AddCommand(knowledgeCmd)
	knowledgeCmd.Flags().StringVarP(&knowledgeTypeFlag, "type", "t", "", "Filter by node type (decision, feature, constraint, pattern, plan, note, metadata, documentation)")
	knowledgeCmd.Flags().StringVarP(&knowledgeWorkspaceFlag, "workspace", "w", "", "Filter by workspace name (e.g., 'osprey', 'api'). Includes root nodes by default.")
	knowledgeCmd.Flags().BoolVar(&knowledgeAllFlag, "all", false, "Show all workspaces")
}

func runKnowledge(cmd *cobra.Command, args []string) error {
	// Support both positional arg and --type flag (flag takes precedence)
	var nodeType string
	if knowledgeTypeFlag != "" {
		nodeType = knowledgeTypeFlag
	} else if len(args) > 0 {
		nodeType = args[0]
	}

	repo, err := openRepoOrHandleMissingMemory()
	if err != nil {
		return err
	}
	if repo == nil {
		return nil
	}
	defer func() { _ = repo.Close() }()

	// Resolve workspace: --all overrides --workspace
	var workspace string
	if knowledgeAllFlag {
		workspace = ""
	} else if knowledgeWorkspaceFlag != "" {
		if err := app.ValidateWorkspace(knowledgeWorkspaceFlag); err != nil {
			return err
		}
		workspace = knowledgeWorkspaceFlag
	}

	filter := memory.NodeFilter{
		Type:        nodeType,
		Workspace:   workspace,
		IncludeRoot: true, // Include root/global nodes with workspace filters.
	}

	var nodes []memory.Node
	if workspace != "" {
		nodes, err = repo.ListNodesFiltered(filter)
	} else {
		nodes, err = repo.ListNodes(nodeType)
	}
	if err != nil {
		return fmt.Errorf("list nodes: %w", err)
	}

	if isJSON() {
		return printJSON(nodes)
	}

	if len(nodes) == 0 {
		if nodeType != "" {
			cmd.Printf("no %s nodes found", nodeType)
		} else {
			cmd.Print("no knowledge nodes found")
		}
		if workspace != "" {
			cmd.Printf(" in workspace '%s'", workspace)
		}
		cmd.Println(".")
		cmd.Println("run `taskwing learn` to populate project memory.")
		return nil
	}

	// Pretty TUI output for interactive use; compact text for AI/scripts.
	if ui.IsInteractive() && !isJSON() {
		basePath, _ := config.GetProjectRoot()
		if viper.GetBool("verbose") {
			ui.RenderNodeListVerbose(nodes, basePath)
		} else {
			ui.RenderNodeList(nodes, basePath)
		}
		return nil
	}

	renderKnowledgeCompact(nodes)
	return nil
}

// renderKnowledgeCompact emits the AI-friendly default output for `taskwing knowledge`.
// Same shape as the SessionStart brief: a one-line header, a blank rule, then
// type-grouped sections with one entry per node ([workspace] title).
func renderKnowledgeCompact(nodes []memory.Node) {
	// Group by type, in canonical order.
	typeOrder := []struct {
		kind, code, label string
	}{
		{memory.NodeTypeDecision, "D", "Decisions"},
		{memory.NodeTypeFeature, "F", "Features"},
		{memory.NodeTypeConstraint, "C", "Constraints"},
		{memory.NodeTypePattern, "P", "Patterns"},
		{memory.NodeTypePlan, "PL", "Plans"},
		{memory.NodeTypeNote, "N", "Notes"},
		{memory.NodeTypeMetadata, "M", "Metadata"},
		{memory.NodeTypeDocumentation, "DOC", "Docs"},
	}

	byType := make(map[string][]memory.Node)
	for _, n := range nodes {
		byType[n.Type] = append(byType[n.Type], n)
	}

	// Build a one-line summary like: "88 nodes (D 56 | F 7 | C 5 | P 16 | DOC 4)".
	var counts []string
	for _, t := range typeOrder {
		if c := len(byType[t.kind]); c > 0 {
			counts = append(counts, fmt.Sprintf("%s %d", t.code, c))
		}
	}
	fmt.Printf("%d nodes (%s)\n", len(nodes), strings.Join(counts, " | "))
	fmt.Println(strings.Repeat("-", 50))

	// Build groups for render.RenderGroups.
	groups := make([]render.Group, 0, len(typeOrder))
	for _, t := range typeOrder {
		ns := byType[t.kind]
		if len(ns) == 0 {
			continue
		}
		items := make([]string, 0, len(ns))
		for _, n := range ns {
			items = append(items, formatKnowledgeLine(n))
		}
		groups = append(groups, render.Group{
			Code:  t.code,
			Label: t.label,
			Items: items,
		})
	}

	fmt.Println()
	render.RenderGroups(os.Stdout, groups, false)
}

// formatKnowledgeLine returns the node summary, prefixed with [workspace] only
// when the summary doesn't already start with that prefix (legacy data has it
// embedded in the title).
func formatKnowledgeLine(n memory.Node) string {
	title := n.Summary
	if title == "" {
		title = truncateRunes(n.Content, 100)
	}
	if n.Workspace == "" || n.Workspace == "root" {
		return title
	}
	prefix := fmt.Sprintf("[%s]", n.Workspace)
	if strings.HasPrefix(title, prefix) {
		return title
	}
	return prefix + " " + title
}
