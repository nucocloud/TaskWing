/*
Copyright © 2025 Joseph Goksu josephgoksu@gmail.com
*/
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/josephgoksu/TaskWing/internal/bootstrap"
	"github.com/josephgoksu/TaskWing/internal/config"
	"github.com/josephgoksu/TaskWing/internal/project"
	"github.com/josephgoksu/TaskWing/internal/ui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Declare this directory a TaskWing project and set up AI tool integration",
	Long: `Initialize a TaskWing project in the current directory.

This is a fast, no-LLM command that:
  1. Writes a .taskwing.yaml marker file (declares this dir as the project root)
  2. Creates the global storage dir at ~/.taskwing/projects/<slug>/
  3. Generates local AI tool integration files (.claude/settings.json,
     .claude/commands/taskwing/, .cursor/rules, etc.) for AIs you select

After init, run 'taskwing learn' to populate project memory with LLM-powered
analysis. Slash commands (/taskwing:plan, /taskwing:next, …) drive their flows
by invoking the taskwing CLI directly - there is no separate MCP server.

The .taskwing.yaml file is the source of truth for project identity. As long as
it exists, every TaskWing command knows where the project is and which storage
slug it owns - even after directory renames.`,
	Example: `  taskwing init                    # auto-detect AI CLIs in PATH
  taskwing init --ai claude        # init only for Claude Code
  taskwing init --ai claude,cursor # multiple AIs
  taskwing init --ai all           # every supported AI
  taskwing init --no-ai            # marker + storage only, no integration files`,
	RunE: runInit,
}

var (
	initAIs   []string
	initNoAI  bool
	initForce bool
)

func init() {
	initCmd.Flags().StringSliceVar(&initAIs, "ai", nil, "Comma-separated AI tools to set up (claude,cursor,gemini,codex,copilot,opencode) or 'all'. If empty, auto-detects AI CLIs in PATH.")
	initCmd.Flags().BoolVar(&initNoAI, "no-ai", false, "Skip AI integration entirely (only write marker + storage dir)")
	initCmd.Flags().BoolVar(&initForce, "force", false, "Overwrite an existing .taskwing.yaml marker")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	// Resolve to absolute, symlink-clean path so the slug is stable.
	if abs, err := filepath.Abs(cwd); err == nil {
		cwd = filepath.Clean(abs)
	}

	ui.SectionHeader("Project")

	markerPath := filepath.Join(cwd, project.MarkerFileName)
	markerExisted := false
	if _, err := os.Stat(markerPath); err == nil {
		markerExisted = true
		if !initForce {
			ui.StatusLine(ui.IconNeutral, fmt.Sprintf("%s already exists (pass --force to rewrite)", project.MarkerFileName))
		}
	}

	if !markerExisted || initForce {
		if err := writeMarkerFile(markerPath, cwd); err != nil {
			return fmt.Errorf("write marker file: %w", err)
		}
		ui.StatusLine(ui.IconOK, fmt.Sprintf("wrote %s", project.MarkerFileName))
	}

	// Refresh project context now that the marker exists. Without this, the
	// context cached during initConfig still reflects the pre-marker state.
	config.ClearProjectContext()
	ctx, err := project.Detect(cwd)
	if err != nil {
		return fmt.Errorf("re-detect project after writing marker: %w", err)
	}
	if err := config.SetProjectContext(ctx); err != nil {
		return fmt.Errorf("set project context: %w", err)
	}

	// Pre-create the global store dir so subsequent commands work immediately.
	storePath, err := config.GetProjectStorePath(cwd)
	if err != nil {
		return fmt.Errorf("create project store: %w", err)
	}
	ui.StatusLine(ui.IconOK, fmt.Sprintf("storage: %s", storePath))

	// Decide which AIs to set up.
	selected, err := resolveInitAIs()
	if err != nil {
		return err
	}

	if len(selected) == 0 {
		ui.StatusLine(ui.IconSkip, "AI integration skipped (re-run with --ai <name>)")
		printPostInitHints(cwd, storePath, false)
		return nil
	}

	ui.SectionHeader("AI integration")

	initializer := bootstrap.NewInitializer(cwd, storePath)
	initializer.Version = version
	if err := initializer.RegenerateConfigs(viper.GetBool("verbose"), selected); err != nil {
		return fmt.Errorf("write AI integration files: %w", err)
	}

	printPostInitHints(cwd, storePath, true)
	return nil
}

// ensureProjectMarker writes a .taskwing.yaml marker at projectRoot if one does
// not already exist. Used by bootstrap and other commands to transparently
// migrate users to the marker-based project identity model. Refreshes the
// cached project context if a marker was written so subsequent calls see it.
func ensureProjectMarker(projectRoot string) error {
	if abs, err := filepath.Abs(projectRoot); err == nil {
		projectRoot = filepath.Clean(abs)
	}
	markerPath := filepath.Join(projectRoot, project.MarkerFileName)
	if _, err := os.Stat(markerPath); err == nil {
		return nil
	}
	if err := writeMarkerFile(markerPath, projectRoot); err != nil {
		return err
	}
	// Refresh project context so the rest of this command sees the marker.
	config.ClearProjectContext()
	if ctx, err := project.Detect(projectRoot); err == nil {
		_ = config.SetProjectContext(ctx)
	}
	return nil
}

// writeMarkerFile writes the .taskwing.yaml marker.
func writeMarkerFile(path, projectRoot string) error {
	slug := config.ProjectSlug(projectRoot)
	name := filepath.Base(projectRoot)
	now := time.Now().UTC().Format(time.RFC3339)

	content := fmt.Sprintf(`# TaskWing project marker - declares this directory as a TaskWing project.
# Storage lives at ~/.taskwing/projects/<slug>/. Memory and knowledge are global;
# this file is the local pointer to it.
version: "1"
project:
  name: %q
  slug: %q
  initialized_at: %q

# Optional per-project overrides (uncomment to use). Global config in
# ~/.taskwing/config.yaml is the default.
# llm:
#   provider: anthropic
#   model: claude-opus-4-7
`, name, slug, now)

	return os.WriteFile(path, []byte(content), 0644)
}

// resolveInitAIs decides which AIs to configure based on flags and TTY state.
func resolveInitAIs() ([]string, error) {
	if initNoAI {
		return nil, nil
	}

	// Explicit flag wins.
	if len(initAIs) > 0 {
		return expandAIList(initAIs)
	}

	// No flag, no TTY: default to detecting installed AI CLIs.
	// Errs on the side of doing useful work without prompting.
	detected := detectInstalledAIClis()
	if len(detected) > 0 {
		fmt.Printf("Detected AI CLIs in PATH: %s\n", strings.Join(detected, ", "))
		return detected, nil
	}

	// Nothing detected, no explicit choice - proceed without AI setup.
	fmt.Println("No AI CLIs detected in PATH. Re-run with --ai <name> to set up integration.")
	return nil, nil
}

// expandAIList resolves "all" and validates each entry against the catalog.
func expandAIList(names []string) ([]string, error) {
	if len(names) == 1 && strings.EqualFold(names[0], "all") {
		return bootstrap.ValidAINames(), nil
	}

	valid := make(map[string]struct{}, len(bootstrap.ValidAINames()))
	for _, n := range bootstrap.ValidAINames() {
		valid[n] = struct{}{}
	}

	out := make([]string, 0, len(names))
	for _, raw := range names {
		n := strings.ToLower(strings.TrimSpace(raw))
		if n == "" {
			continue
		}
		if _, ok := valid[n]; !ok {
			return nil, fmt.Errorf("unknown AI %q (supported: %s)", n, strings.Join(bootstrap.ValidAINames(), ", "))
		}
		out = append(out, n)
	}
	return out, nil
}

// detectInstalledAIClis returns AI catalog names whose CLIs are present in PATH.
// Project-local AIs (cursor, copilot, opencode) are not detectable via PATH;
// they must be requested explicitly.
func detectInstalledAIClis() []string {
	var found []string
	if _, err := exec.LookPath("claude"); err == nil {
		found = append(found, "claude")
	}
	if _, err := exec.LookPath("gemini"); err == nil {
		found = append(found, "gemini")
	}
	if _, err := exec.LookPath("codex"); err == nil {
		found = append(found, "codex")
	}
	return found
}

// printPostInitHints shows the user the most useful next commands.
func printPostInitHints(projectDir, storePath string, didAI bool) {
	_ = projectDir
	_ = storePath
	dim := ui.StyleDim
	fmt.Println()
	fmt.Printf("    %s\n", dim.Render("next:"))
	fmt.Printf("      taskwing learn          %s\n", dim.Render("# analyze codebase with LLM"))
	fmt.Printf("      taskwing doctor         %s\n", dim.Render("# verify integration health"))
	if !didAI {
		fmt.Printf("      taskwing init --ai all  %s\n", dim.Render("# set up AI integration files"))
	}
	fmt.Println()
}
