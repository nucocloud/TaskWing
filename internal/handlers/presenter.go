// Package handlers also provides Markdown formatting for plan tool responses.
//
// Historically this file contained formatters for every MCP tool (knowledge,
// ask, task, plan, code, debug, simplify). After MCP was removed, only the
// plan formatters survive - task and ask render their own output in the cmd/
// layer, and code/debug were dropped along with their handlers.
package handlers

import (
	"fmt"
	"strings"

	"github.com/josephgoksu/TaskWing/internal/app"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// === Error Formatters ===

// FormatError returns a Markdown error message.
func FormatError(message string) string {
	return fmt.Sprintf("## ❌ Error\n\n**Details**: %s", message)
}

// FormatMultiValidationError returns a Markdown error for multiple validation failures.
// This helps LLMs understand all required fields at once rather than failing sequentially.
func FormatMultiValidationError(action string, missingFields []string, guidance string) string {
	var sb strings.Builder
	sb.WriteString("## ❌ Validation Error\n\n")
	fmt.Fprintf(&sb, "**Action**: `%s`\n\n", action)
	sb.WriteString("**Missing required fields**:\n")
	for _, f := range missingFields {
		fmt.Fprintf(&sb, "- `%s`\n", f)
	}
	if guidance != "" {
		fmt.Fprintf(&sb, "\n**Guidance**: %s", guidance)
	}
	return sb.String()
}

// === Plan Formatters ===

// FormatClarifyResult formats plan clarification output.
func FormatClarifyResult(result *app.ClarifyResult) string {
	if result == nil {
		return FormatError("No clarification result.")
	}
	if !result.Success {
		msg := result.Message
		if msg == "" {
			msg = "Clarification failed with no details"
		}
		return FormatError(msg)
	}

	var sb strings.Builder

	if result.IsReadyToPlan {
		sb.WriteString("## ✅ Ready to Generate Plan\n\n")
		if result.ClarifySessionID != "" {
			fmt.Fprintf(&sb, "**Session**: `%s` | **Round**: %d\n\n", result.ClarifySessionID, result.RoundIndex)
		}
		if result.GoalSummary != "" {
			fmt.Fprintf(&sb, "**Goal**: %s\n\n", result.GoalSummary)
		}
		if result.EnrichedGoal != "" {
			sb.WriteString("### What will be built\n")
			sb.WriteString(result.EnrichedGoal)
			sb.WriteString("\n\n")
		}
		sb.WriteString("> Approve this specification, then the plan will be generated with tasks.\n")
	} else {
		sb.WriteString("## 🔍 Decisions Needed Before Planning\n\n")
		if result.ClarifySessionID != "" {
			fmt.Fprintf(&sb, "**Session**: `%s` | **Round**: %d\n\n", result.ClarifySessionID, result.RoundIndex)
		}
		if result.GoalSummary != "" {
			fmt.Fprintf(&sb, "**Goal**: %s\n\n", result.GoalSummary)
		}

		if len(result.Questions) > 0 {
			for i, q := range result.Questions {
				fmt.Fprintf(&sb, "### Decision %d\n", i+1)
				fmt.Fprintf(&sb, "%s\n\n", q)
			}
		}

		if result.EnrichedGoal != "" {
			sb.WriteString("### Draft Specification\n")
			sb.WriteString(result.EnrichedGoal)
			sb.WriteString("\n\n")
		}

		sb.WriteString("> Answer the decisions above, or say **auto** to let TaskWing choose.\n")
	}

	if result.MaxRoundsReached {
		sb.WriteString("\n⚠️ Maximum clarification rounds reached.\n")
	}
	if result.ContextUsed != "" {
		fmt.Fprintf(&sb, "\n*%s*\n", result.ContextUsed)
	}

	return strings.TrimSpace(sb.String())
}

// FormatGenerateResult formats plan generation output.
func FormatGenerateResult(result *app.GenerateResult) string {
	if result == nil {
		return FormatError("No generation result.")
	}
	if !result.Success {
		msg := result.Message
		if msg == "" {
			msg = "Plan generation failed with no details"
		}
		return FormatError(msg)
	}

	var sb strings.Builder
	sb.WriteString("## ✅ Plan Generated\n\n")
	fmt.Fprintf(&sb, "**Plan**: `%s`\n", result.PlanID)
	fmt.Fprintf(&sb, "**Goal**: %s\n", result.Goal)
	fmt.Fprintf(&sb, "**Tasks**: %d\n\n", len(result.Tasks))

	if len(result.Tasks) > 0 {
		sb.WriteString("| # | Task | Priority |\n")
		sb.WriteString("|---|------|----------|\n")
		for i, t := range result.Tasks {
			fmt.Fprintf(&sb, "| %d | %s | P%d |\n", i+1, t.Title, t.Priority)
		}
		sb.WriteString("\n")
	}

	if result.Hint != "" {
		fmt.Fprintf(&sb, "> %s\n", result.Hint)
	}

	return strings.TrimSpace(sb.String())
}

// FormatAuditResult formats plan audit output.
func FormatAuditResult(result *app.AuditResult) string {
	if result == nil {
		return FormatError("No audit result.")
	}
	if !result.Success {
		msg := result.Message
		if msg == "" {
			msg = "Audit failed with no details"
		}
		return FormatError(msg)
	}

	var sb strings.Builder

	statusIcon := "🔍"
	switch result.Status {
	case "verified":
		statusIcon = "✅"
	case "needs_revision":
		statusIcon = "⚠️"
	case "failed":
		statusIcon = "❌"
	}

	fmt.Fprintf(&sb, "## %s Audit: %s\n\n", statusIcon, cases.Title(language.English).String(result.Status))
	fmt.Fprintf(&sb, "**Plan ID**: `%s`\n", result.PlanID)
	fmt.Fprintf(&sb, "**Attempts**: %d\n\n", result.RetryCount)

	sb.WriteString("### Checks\n")
	buildIcon := "❌"
	if result.BuildPassed {
		buildIcon = "✅"
	}
	testIcon := "❌"
	if result.TestsPassed {
		testIcon = "✅"
	}
	fmt.Fprintf(&sb, "- %s Build\n", buildIcon)
	fmt.Fprintf(&sb, "- %s Tests\n", testIcon)
	sb.WriteString("\n")

	if len(result.SemanticIssues) > 0 {
		sb.WriteString("### Semantic Issues\n")
		for _, issue := range result.SemanticIssues {
			fmt.Fprintf(&sb, "- %s\n", issue)
		}
		sb.WriteString("\n")
	}

	if len(result.FixesApplied) > 0 {
		sb.WriteString("### Fixes Applied\n")
		for _, fix := range result.FixesApplied {
			fmt.Fprintf(&sb, "- %s\n", fix)
		}
		sb.WriteString("\n")
	}

	if result.Message != "" {
		fmt.Fprintf(&sb, "%s\n\n", result.Message)
	}
	if result.Hint != "" {
		fmt.Fprintf(&sb, "> **Hint**: %s\n", result.Hint)
	}

	return strings.TrimSpace(sb.String())
}

// FormatDecomposeResult formats plan decomposition output.
func FormatDecomposeResult(result *app.DecomposeResult) string {
	if result == nil {
		return FormatError("No decomposition result.")
	}
	if !result.Success {
		msg := result.Message
		if msg == "" {
			msg = "Decomposition failed with no details"
		}
		return FormatError(msg)
	}

	var sb strings.Builder
	sb.WriteString("## 📋 Goal Decomposition\n\n")
	fmt.Fprintf(&sb, "**Plan ID**: `%s`\n", result.PlanID)
	fmt.Fprintf(&sb, "**Phases**: %d\n\n", len(result.Phases))

	sb.WriteString("### Phases\n")
	for i, phase := range result.Phases {
		fmt.Fprintf(&sb, "%d. **%s**\n", i+1, phase.Title)
		if phase.Description != "" {
			fmt.Fprintf(&sb, "   %s\n", phase.Description)
		}
		fmt.Fprintf(&sb, "   _Expected tasks: %d_\n", phase.ExpectedTasks)
	}
	sb.WriteString("\n")

	if result.Rationale != "" {
		sb.WriteString("### Rationale\n")
		sb.WriteString(result.Rationale)
		sb.WriteString("\n\n")
	}
	if result.Hint != "" {
		fmt.Fprintf(&sb, "> **Next**: %s\n", result.Hint)
	}

	return strings.TrimSpace(sb.String())
}

// FormatExpandResult formats phase expansion output.
func FormatExpandResult(result *app.ExpandResult) string {
	if result == nil {
		return FormatError("No expansion result.")
	}
	if !result.Success {
		msg := result.Message
		if msg == "" {
			msg = "Expansion failed with no details"
		}
		return FormatError(msg)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "## 🔧 Phase Expansion: %s\n\n", result.PhaseTitle)
	fmt.Fprintf(&sb, "**Plan ID**: `%s`\n", result.PlanID)
	fmt.Fprintf(&sb, "**Phase ID**: `%s`\n", result.PhaseID)
	fmt.Fprintf(&sb, "**Tasks Generated**: %d\n\n", len(result.Tasks))

	sb.WriteString("### Tasks\n")
	for i, t := range result.Tasks {
		complexityBadge := ""
		if t.Complexity != "" {
			complexityBadge = fmt.Sprintf(" [%s]", t.Complexity)
		}
		fmt.Fprintf(&sb, "%d. **%s**%s (P%d)\n", i+1, t.Title, complexityBadge, t.Priority)
		if t.Description != "" {
			fmt.Fprintf(&sb, "   %s\n", t.Description)
		}
	}
	sb.WriteString("\n")

	if result.RemainingPhases > 0 {
		fmt.Fprintf(&sb, "**Remaining phases**: %d\n", result.RemainingPhases)
		if result.NextPhaseTitle != "" {
			fmt.Fprintf(&sb, "**Next phase**: %s\n\n", result.NextPhaseTitle)
		}
	}
	if result.Hint != "" {
		fmt.Fprintf(&sb, "> **Next**: %s\n", result.Hint)
	}

	return strings.TrimSpace(sb.String())
}

// FormatFinalizeResult formats plan finalization output.
func FormatFinalizeResult(result *app.FinalizeResult) string {
	if result == nil {
		return FormatError("No finalization result.")
	}
	if !result.Success {
		msg := result.Message
		if msg == "" {
			msg = "Finalization failed with no details"
		}
		return FormatError(msg)
	}

	var sb strings.Builder
	sb.WriteString("## ✅ Plan Finalized\n\n")
	fmt.Fprintf(&sb, "**Plan ID**: `%s`\n", result.PlanID)
	fmt.Fprintf(&sb, "**Status**: %s\n", result.Status)
	fmt.Fprintf(&sb, "**Total Phases**: %d\n", result.TotalPhases)
	fmt.Fprintf(&sb, "**Total Tasks**: %d\n\n", result.TotalTasks)

	if result.Message != "" {
		fmt.Fprintf(&sb, "%s\n\n", result.Message)
	}
	if result.Hint != "" {
		fmt.Fprintf(&sb, "> **Next**: %s\n", result.Hint)
	}

	return strings.TrimSpace(sb.String())
}
