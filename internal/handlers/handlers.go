// Package handlers wires CLI verbs (cmd/plan.go) to the underlying app layer.
//
// Historically this package backed the MCP `plan`, `task`, `code`, and `debug`
// tools. After MCP was removed only the plan handler is still used; task
// lifecycle is driven directly by `taskwing task <next|current|start|complete>`
// in cmd/task.go and code/debug were dropped entirely.
package handlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/josephgoksu/TaskWing/internal/app"
	"github.com/josephgoksu/TaskWing/internal/llm"
	"github.com/josephgoksu/TaskWing/internal/memory"
)

// PlanToolResult represents the response from the plan tool.
type PlanToolResult struct {
	Action  string `json:"action"`
	Content string `json:"content"`
	Error   string `json:"error,omitempty"`
}

// HandlePlanTool routes a PlanToolParams to the appropriate plan action and
// returns a JSON-serializable result. Used by cmd/plan.go.
func HandlePlanTool(ctx context.Context, repo *memory.Repository, params PlanToolParams) (*PlanToolResult, error) {
	if !params.Action.IsValid() {
		return &PlanToolResult{
			Action: string(params.Action),
			Error:  fmt.Sprintf("invalid action %q, must be one of: clarify, decompose, expand, generate, finalize, audit", params.Action),
		}, nil
	}

	switch params.Action {
	case PlanActionClarify:
		return handlePlanClarify(ctx, repo, params)
	case PlanActionDecompose:
		return handlePlanDecompose(ctx, repo, params)
	case PlanActionExpand:
		return handlePlanExpand(ctx, repo, params)
	case PlanActionGenerate:
		return handlePlanGenerate(ctx, repo, params)
	case PlanActionFinalize:
		return handlePlanFinalize(ctx, repo, params)
	case PlanActionAudit:
		return handlePlanAudit(ctx, repo, params)
	default:
		return &PlanToolResult{
			Action: string(params.Action),
			Error:  fmt.Sprintf("unsupported action: %s", params.Action),
		}, nil
	}
}

// handlePlanClarify implements the 'clarify' action - refine a goal with questions.
func handlePlanClarify(ctx context.Context, repo *memory.Repository, params PlanToolParams) (*PlanToolResult, error) {
	goal := strings.TrimSpace(params.Goal)
	sessionID := strings.TrimSpace(params.ClarifySessionID)

	if sessionID == "" && goal == "" {
		return &PlanToolResult{
			Action: "clarify",
			Error:  "goal is required for clarify action",
			Content: FormatMultiValidationError(
				"clarify",
				[]string{"goal"},
				"First clarify call requires a goal. Follow-up calls require clarify_session_id and answers.",
			),
		}, nil
	}

	if sessionID != "" && !params.AutoAnswer && len(params.Answers) == 0 {
		return &PlanToolResult{
			Action: "clarify",
			Error:  "answers are required for clarify follow-up action",
			Content: FormatMultiValidationError(
				"clarify",
				[]string{"answers"},
				"Provide answers for pending questions, or set auto_answer=true to let TaskWing continue automatically.",
			),
		}, nil
	}

	answers := make([]app.ClarifyAnswer, 0, len(params.Answers))
	for _, ans := range params.Answers {
		answer := strings.TrimSpace(ans.Answer)
		if answer == "" {
			continue
		}
		answers = append(answers, app.ClarifyAnswer{
			Question: strings.TrimSpace(ans.Question),
			Answer:   answer,
		})
	}

	appCtx := app.NewContextForRole(repo, llm.RoleBootstrap)
	planApp := app.NewPlanApp(appCtx)

	result, err := planApp.Clarify(ctx, app.ClarifyOptions{
		Goal:             goal,
		ClarifySessionID: sessionID,
		Answers:          answers,
		AutoAnswer:       params.AutoAnswer,
	})
	if err != nil {
		return &PlanToolResult{Action: "clarify", Error: err.Error()}, nil
	}

	return &PlanToolResult{Action: "clarify", Content: FormatClarifyResult(result)}, nil
}

// handlePlanGenerate implements the 'generate' action - create a plan with tasks.
func handlePlanGenerate(ctx context.Context, repo *memory.Repository, params PlanToolParams) (*PlanToolResult, error) {
	goal := strings.TrimSpace(params.Goal)
	enrichedGoal := strings.TrimSpace(params.EnrichedGoal)
	clarifySessionID := strings.TrimSpace(params.ClarifySessionID)
	isPassthrough := len(params.Tasks) > 0

	var missingFields []string
	if goal == "" {
		missingFields = append(missingFields, "goal")
	}
	if !isPassthrough {
		if enrichedGoal == "" && clarifySessionID == "" {
			missingFields = append(missingFields, "enriched_goal_or_clarify_session_id")
		}
	}

	if len(missingFields) > 0 {
		hint := "First call `plan clarify` until is_ready_to_plan=true, then pass goal, enriched_goal, and clarify_session_id to generate. Or provide a tasks array for passthrough mode."
		return &PlanToolResult{
			Action:  "generate",
			Error:   fmt.Sprintf("missing required fields: %v", missingFields),
			Content: FormatMultiValidationError("generate", missingFields, hint),
		}, nil
	}

	save := true
	if params.Save != nil {
		save = *params.Save
	}

	appCtx := app.NewContextForRole(repo, llm.RoleBootstrap)
	planApp := app.NewPlanApp(appCtx)

	result, err := planApp.Generate(ctx, app.GenerateOptions{
		Goal:             goal,
		ClarifySessionID: clarifySessionID,
		EnrichedGoal:     enrichedGoal,
		Save:             save,
		ExplicitTasks:    params.Tasks,
	})
	if err != nil {
		return &PlanToolResult{Action: "generate", Error: err.Error()}, nil
	}

	return &PlanToolResult{Action: "generate", Content: FormatGenerateResult(result)}, nil
}

// handlePlanDecompose implements the 'decompose' action - break goal into phases.
func handlePlanDecompose(ctx context.Context, repo *memory.Repository, params PlanToolParams) (*PlanToolResult, error) {
	enrichedGoal := strings.TrimSpace(params.EnrichedGoal)
	if enrichedGoal == "" {
		return &PlanToolResult{
			Action: "decompose",
			Error:  "enriched_goal is required for decompose action",
			Content: FormatMultiValidationError(
				"decompose",
				[]string{"enriched_goal"},
				"First call `plan clarify` to refine your goal into an enriched specification, then pass `enriched_goal` to decompose.",
			),
		}, nil
	}

	appCtx := app.NewContextForRole(repo, llm.RoleBootstrap)
	planApp := app.NewPlanApp(appCtx)

	result, err := planApp.Decompose(ctx, app.DecomposeOptions{
		PlanID:       params.PlanID,
		Goal:         params.Goal,
		EnrichedGoal: enrichedGoal,
		Feedback:     params.Feedback,
	})
	if err != nil {
		return &PlanToolResult{Action: "decompose", Error: err.Error()}, nil
	}

	return &PlanToolResult{Action: "decompose", Content: FormatDecomposeResult(result)}, nil
}

// handlePlanExpand implements the 'expand' action - generate tasks for a phase.
func handlePlanExpand(ctx context.Context, repo *memory.Repository, params PlanToolParams) (*PlanToolResult, error) {
	planID := strings.TrimSpace(params.PlanID)
	if planID == "" {
		return &PlanToolResult{
			Action: "expand",
			Error:  "plan_id is required for expand action",
			Content: FormatMultiValidationError(
				"expand",
				[]string{"plan_id"},
				"First call `plan decompose` to create phases, then pass `plan_id` and `phase_id` or `phase_index` to expand.",
			),
		}, nil
	}

	phaseID := strings.TrimSpace(params.PhaseID)
	if phaseID == "" && params.PhaseIndex == nil {
		return &PlanToolResult{
			Action: "expand",
			Error:  "either phase_id or phase_index is required for expand action",
			Content: FormatMultiValidationError(
				"expand",
				[]string{"phase_id", "phase_index"},
				"Provide the ID or 0-based index of the phase to expand.",
			),
		}, nil
	}

	appCtx := app.NewContextForRole(repo, llm.RoleBootstrap)
	planApp := app.NewPlanApp(appCtx)

	opts := app.ExpandOptions{
		PlanID:   planID,
		PhaseID:  phaseID,
		Feedback: params.Feedback,
	}
	if params.PhaseIndex != nil {
		opts.PhaseIndex = *params.PhaseIndex
	} else {
		opts.PhaseIndex = -1
	}

	result, err := planApp.Expand(ctx, opts)
	if err != nil {
		return &PlanToolResult{Action: "expand", Error: err.Error()}, nil
	}

	return &PlanToolResult{Action: "expand", Content: FormatExpandResult(result)}, nil
}

// handlePlanFinalize implements the 'finalize' action - save completed interactive plan.
func handlePlanFinalize(ctx context.Context, repo *memory.Repository, params PlanToolParams) (*PlanToolResult, error) {
	planID := strings.TrimSpace(params.PlanID)
	if planID == "" {
		return &PlanToolResult{
			Action: "finalize",
			Error:  "plan_id is required for finalize action",
			Content: FormatMultiValidationError(
				"finalize",
				[]string{"plan_id"},
				"Provide the plan_id from the decompose step to finalize.",
			),
		}, nil
	}

	appCtx := app.NewContextForRole(repo, llm.RoleBootstrap)
	planApp := app.NewPlanApp(appCtx)

	result, err := planApp.Finalize(ctx, app.FinalizeOptions{PlanID: planID})
	if err != nil {
		return &PlanToolResult{Action: "finalize", Error: err.Error()}, nil
	}

	return &PlanToolResult{Action: "finalize", Content: FormatFinalizeResult(result)}, nil
}

// handlePlanAudit implements the 'audit' action - verify and fix a completed plan.
func handlePlanAudit(ctx context.Context, repo *memory.Repository, params PlanToolParams) (*PlanToolResult, error) {
	autoFix := true
	if params.AutoFix != nil {
		autoFix = *params.AutoFix
	}

	appCtx := app.NewContextForRole(repo, llm.RoleBootstrap)
	planApp := app.NewPlanApp(appCtx)

	result, err := planApp.Audit(ctx, app.AuditOptions{PlanID: params.PlanID, AutoFix: autoFix})
	if err != nil {
		return &PlanToolResult{Action: "audit", Error: err.Error()}, nil
	}

	return &PlanToolResult{Action: "audit", Content: FormatAuditResult(result)}, nil
}
