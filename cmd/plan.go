/*
Copyright © 2025 Joseph Goksu josephgoksu@gmail.com
*/
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	handlers "github.com/josephgoksu/TaskWing/internal/handlers"
	"github.com/spf13/cobra"
)

// planCmd is the entry point for the plan tool, exposed as a CLI verb.
//
// It accepts a single JSON params blob via --params (or stdin) and prints the
// JSON-encoded result. This thin shape mirrors the tool contract so slash
// commands and other agents can invoke planning without a long-lived server.
var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Plan management: clarify, decompose, expand, generate, finalize, audit",
	Long: `Plan management for an active development goal.

The plan command takes a JSON params object describing one of the actions in
the plan lifecycle and returns a JSON result. It is the CLI surface used by
the /taskwing:plan slash command and by any agent that wants to drive a
clarify → decompose → expand → finalize flow programmatically.

Actions:
  clarify    Refine a goal with questions until the planner is ready to plan
  decompose  Break the enriched goal into 3-5 phases
  expand     Generate detailed tasks for a single phase
  generate   Batch-generate all tasks at once (skips decompose/expand)
  finalize   Persist a fully-expanded interactive plan
  audit      Verify plan implementation against reality`,
	Example: `  # Start clarification
  taskwing plan --params '{"action":"clarify","goal":"add stripe billing"}'

  # Pipe a params object from stdin (recommended for multi-line answers)
  echo '{"action":"clarify","clarify_session_id":"...","answers":[...]}' \
    | taskwing plan --params -

  # Decompose
  taskwing plan --params '{"action":"decompose","clarify_session_id":"...","enriched_goal":"..."}'

  # Expand a phase
  taskwing plan --params '{"action":"expand","plan_id":"...","phase_id":"..."}'

  # Finalize
  taskwing plan --params '{"action":"finalize","plan_id":"..."}'`,
	RunE: runPlan,
}

var planParamsRaw string

func init() {
	planCmd.Flags().StringVar(&planParamsRaw, "params", "", "JSON params blob; use '-' to read from stdin")
	_ = planCmd.MarkFlagRequired("params")
	rootCmd.AddCommand(planCmd)
}

func runPlan(cmd *cobra.Command, args []string) error {
	raw, err := readParamsBlob(planParamsRaw)
	if err != nil {
		return err
	}

	var params handlers.PlanToolParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return fmt.Errorf("parse --params JSON: %w", err)
	}

	repo, err := openRepoOrHandleMissingMemory()
	if err != nil || repo == nil {
		return err
	}
	defer func() { _ = repo.Close() }()

	ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
	defer cancel()

	result, err := handlers.HandlePlanTool(ctx, repo, params)
	if err != nil {
		return err
	}

	return printJSON(result)
}

// readParamsBlob reads a JSON params blob from a flag value. If the value is
// "-" or empty (with stdin piped), it reads from stdin instead. Returns the
// raw bytes ready for json.Unmarshal.
func readParamsBlob(flagValue string) ([]byte, error) {
	if flagValue == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		return data, nil
	}
	if strings.TrimSpace(flagValue) == "" {
		return nil, fmt.Errorf("--params is required (or pass '-' to read from stdin)")
	}
	return []byte(flagValue), nil
}
