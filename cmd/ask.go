/*
Copyright © 2025 Joseph Goksu josephgoksu@gmail.com
*/
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/josephgoksu/TaskWing/internal/app"
	"github.com/josephgoksu/TaskWing/internal/llm"
	"github.com/josephgoksu/TaskWing/internal/render"
	"github.com/josephgoksu/TaskWing/internal/ui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var askCmd = &cobra.Command{
	Use:          "ask <question>",
	Short:        "Search project knowledge and code symbols",
	SilenceUsage: true,
	Long: `Query the project knowledge base.

Searches architectural knowledge (decisions, patterns, constraints) and
code symbols (functions, types, interfaces). The /taskwing:next slash command
calls this verb under the hood; you can also run it directly.

By default, uses hybrid search (FTS + vector). Use --fts-only to skip
embedding API calls for faster, offline results.

Examples:
  taskwing ask "how does authentication work"
  taskwing ask "SQLite schema design" --limit 10
  taskwing ask "how does the planner work" --answer
  taskwing ask "task state machine" --json
  taskwing ask "API endpoints" --fts-only
  taskwing ask "auth" --workspace=osprey`,
	Args: cobra.ExactArgs(1),
	RunE: runAsk,
}

func init() {
	rootCmd.AddCommand(askCmd)
	askCmd.Flags().BoolP("answer", "a", false, "Generate a RAG answer (uses LLM, slower)")
	askCmd.Flags().StringP("workspace", "w", "", "Filter by workspace (monorepo)")
	askCmd.Flags().IntP("limit", "l", 5, "Max knowledge results")
	askCmd.Flags().Bool("no-symbols", false, "Skip code symbol search")
	askCmd.Flags().Bool("fts-only", false, "Disable vector search (faster, no embedding API call)")
}

func runAsk(cmd *cobra.Command, args []string) error {
	query := args[0]

	repo, err := openRepoOrHandleMissingMemory()
	if err != nil {
		return err
	}
	if repo == nil {
		return nil
	}
	defer func() { _ = repo.Close() }()

	cfg, err := getLLMConfigForRole(cmd, llm.RoleQuery)
	if err != nil {
		return fmt.Errorf("llm config: %w", err)
	}

	askApp := app.NewAskApp(app.NewContextWithConfig(repo, cfg))

	// Build options from flags
	limit, _ := cmd.Flags().GetInt("limit")
	noSymbols, _ := cmd.Flags().GetBool("no-symbols")
	ftsOnly, _ := cmd.Flags().GetBool("fts-only")
	generateAnswer, _ := cmd.Flags().GetBool("answer")
	workspace, _ := cmd.Flags().GetString("workspace")

	if workspace != "" {
		if err := app.ValidateWorkspace(workspace); err != nil {
			return err
		}
	}

	opts := app.DefaultAskOptions()
	opts.Limit = limit
	opts.IncludeSymbols = !noSymbols
	opts.DisableVector = ftsOnly
	opts.GenerateAnswer = generateAnswer
	opts.Workspace = workspace

	// Only stream raw text for JSON mode; for TTY we show spinner then styled output.
	pretty := ui.IsInteractive() && !isJSON()
	if generateAnswer && isJSON() {
		opts.StreamWriter = os.Stdout
	}

	// Spinner only when running in a TTY (avoids polluting AI tool stdin/stdout).
	var spin *ui.Spinner
	if pretty {
		if generateAnswer {
			spin = ui.NewSpinner("Generating answer...")
		} else {
			spin = ui.NewSpinner("Searching knowledge...")
		}
		spin.Start()
	}

	result, err := askApp.Query(cmd.Context(), query, opts)
	if spin != nil {
		spin.Stop()
	}
	if err != nil {
		return fmt.Errorf("query failed: %w", err)
	}

	if isJSON() {
		return printJSON(result)
	}

	if isQuiet() {
		return nil
	}

	if pretty {
		ui.RenderAskResult(result, viper.GetBool("verbose"))
		return nil
	}

	// Compact text for AI consumption: header, optional answer, hits, symbols.
	renderAskCompact(result)
	return nil
}

// renderAskCompact emits the AI-friendly default output for `taskwing ask`.
// Token-light: no boxes, no spinners, no score bars, no ANSI color.
func renderAskCompact(result *app.AskResult) {
	fmt.Printf("query    %s\n", result.Query)
	if result.RewrittenQuery != "" && result.RewrittenQuery != result.Query {
		fmt.Printf("rewrite  %s\n", result.RewrittenQuery)
	}
	fmt.Printf("hits     %d nodes, %d symbols\n", result.Total, result.TotalSymbols)
	if result.Warning != "" {
		fmt.Printf("warning  %s\n", result.Warning)
	}

	if result.Answer != "" {
		fmt.Println()
		fmt.Println("answer")
		for _, line := range strings.Split(strings.TrimSpace(result.Answer), "\n") {
			fmt.Printf("  %s\n", line)
		}
	}

	if len(result.Results) > 0 {
		fmt.Println()
		fmt.Println("nodes")
		hits := make([]render.Hit, 0, len(result.Results))
		for _, n := range result.Results {
			title := n.Summary
			if title == "" {
				title = truncateRunes(n.Content, 80)
			}
			var detail string
			if len(n.Evidence) > 0 {
				detail = n.Evidence[0].File
				if n.Evidence[0].Lines != "" {
					detail += ":" + n.Evidence[0].Lines
				}
			}
			hits = append(hits, render.Hit{
				Type:    typeCode(n.Type),
				Title:   title,
				Path:    detail,
				Summary: "",
			})
		}
		// Indent the hit block by two spaces for visual grouping.
		var sb strings.Builder
		render.RenderHits(&sb, hits)
		for _, line := range strings.Split(strings.TrimRight(sb.String(), "\n"), "\n") {
			fmt.Printf("  %s\n", line)
		}
	}

	if len(result.Symbols) > 0 {
		fmt.Println()
		fmt.Println("symbols")
		for _, sym := range result.Symbols {
			fmt.Printf("  %s  %s\n", sym.Name, sym.Location)
		}
	}

	if len(result.Results) == 0 && len(result.Symbols) == 0 && result.Answer == "" {
		fmt.Println()
		fmt.Println("no results — try a different query, or run `taskwing learn` to populate memory")
	}
}

// typeCode maps a node type to a single-letter or short code used in compact output.
func typeCode(nodeType string) string {
	switch nodeType {
	case "decision":
		return "D"
	case "feature":
		return "F"
	case "constraint":
		return "C"
	case "pattern":
		return "P"
	case "plan":
		return "PL"
	case "note":
		return "N"
	case "metadata":
		return "M"
	case "documentation":
		return "DOC"
	default:
		if nodeType == "" {
			return "?"
		}
		return strings.ToUpper(nodeType[:1])
	}
}

// truncateRunes trims s to width runes, adding an ellipsis if cut.
func truncateRunes(s string, width int) string {
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	if width <= 1 {
		return string(runes[:width])
	}
	return string(runes[:width-1]) + "…"
}
