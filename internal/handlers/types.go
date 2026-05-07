// Package handlers types for the plan tool.
//
// Historically this file held param/result types for every MCP tool (code,
// task, debug, ask, remember, plan). After MCP was removed, only the plan
// types remain - task lifecycle is driven by `cmd/task.go` directly and
// code/debug were dropped entirely.
package handlers

import (
	"encoding/json"
	"fmt"

	"github.com/josephgoksu/TaskWing/internal/task"
)

// PlanAction defines the valid actions for the plan tool.
type PlanAction string

const (
	PlanActionClarify   PlanAction = "clarify"   // Refine goal with questions (Stage 1)
	PlanActionDecompose PlanAction = "decompose" // Break goal into phases (Stage 2 - interactive)
	PlanActionExpand    PlanAction = "expand"    // Generate tasks for a phase (Stage 3 - interactive)
	PlanActionGenerate  PlanAction = "generate"  // Generate all tasks at once (batch mode)
	PlanActionFinalize  PlanAction = "finalize"  // Save completed interactive plan (Stage 4)
	PlanActionAudit     PlanAction = "audit"     // Verify plan implementation
)

// IsValid checks if the action is a valid plan action.
func (a PlanAction) IsValid() bool {
	switch a {
	case PlanActionClarify, PlanActionDecompose, PlanActionExpand, PlanActionGenerate, PlanActionFinalize, PlanActionAudit:
		return true
	}
	return false
}

// PhaseInput represents user-provided phase data for interactive mode.
type PhaseInput struct {
	Title         string `json:"title"`
	Description   string `json:"description,omitempty"`
	Rationale     string `json:"rationale,omitempty"`
	ExpectedTasks int    `json:"expected_tasks,omitempty"`
}

// TaskInput is an alias for task.TaskInput - shared struct for explicit task definitions.
type TaskInput = task.TaskInput

// ClarifyAnswerInput is a structured answer to a clarification question.
type ClarifyAnswerInput struct {
	Question string `json:"question,omitempty"`
	Answer   string `json:"answer"`
}

// PlanToolParams defines the parameters for the plan tool.
//
// Required fields by action:
//   - clarify first call: goal
//   - clarify follow-up: clarify_session_id (+ answers unless auto_answer=true)
//   - decompose: plan_id (with enriched_goal) OR enriched_goal (creates new plan)
//   - expand: plan_id, phase_id OR phase_index
//   - generate: goal, enriched_goal, clarify_session_id
//   - finalize: plan_id
//   - audit: none (defaults to active plan)
type PlanToolParams struct {
	Action PlanAction `json:"action"`

	// Goal is the user's development goal.
	Goal string `json:"goal,omitempty"`

	// EnrichedGoal is the full technical specification produced by clarify.
	EnrichedGoal string `json:"enriched_goal,omitempty"`

	// ClarifySessionID identifies an existing clarify session for follow-up rounds.
	ClarifySessionID string `json:"clarify_session_id,omitempty"`

	// Answers are user responses for the previous clarification round.
	Answers []ClarifyAnswerInput `json:"answers,omitempty"`

	// AutoAnswer uses knowledge graph to auto-answer clarifying questions.
	AutoAnswer bool `json:"auto_answer,omitempty"`

	// Save persists the generated plan to the database (default true).
	Save *bool `json:"save,omitempty"`

	// PlanID is the plan to operate on.
	PlanID string `json:"plan_id,omitempty"`

	// AutoFix attempts to automatically fix audit failures (default true).
	AutoFix *bool `json:"auto_fix,omitempty"`

	// Mode is "interactive" or "batch" (default "batch" for backward compatibility).
	Mode string `json:"mode,omitempty"`

	// PhaseID is the ID of the phase to expand.
	PhaseID string `json:"phase_id,omitempty"`

	// PhaseIndex is the 0-based index of the phase to expand.
	PhaseIndex *int `json:"phase_index,omitempty"`

	// Phases is user-edited phase data for decompose feedback.
	Phases []PhaseInput `json:"phases,omitempty"`

	// Tasks is user-provided task data (passthrough mode for generate, feedback for expand).
	Tasks []TaskInput `json:"tasks,omitempty"`

	// Feedback is a regeneration hint for decompose/expand.
	Feedback string `json:"feedback,omitempty"`
}

type planToolParamsAlias PlanToolParams

// UnmarshalJSON enforces strict snake_case payloads and rejects removed fields.
func (p *PlanToolParams) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if _, hasLegacyPlanID := raw["planId"]; hasLegacyPlanID {
		return fmt.Errorf("planId is no longer supported; use plan_id")
	}
	if _, hasHistory := raw["history"]; hasHistory {
		return fmt.Errorf("history is no longer supported; use clarify_session_id and answers")
	}

	var aux planToolParamsAlias
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*p = PlanToolParams(aux)
	return nil
}
