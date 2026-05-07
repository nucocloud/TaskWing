/*
Package core provides the agent registry - a static catalog of every agent
the bootstrap pipeline can run, used by the dashboard server to render the
"agents" tab on `taskwing start`.

Agents are constructed directly via their `New*Agent` constructors (see
internal/bootstrap/factory.go); the registry is metadata-only and is the
single source of truth for agent identity, name, and description.
*/
package core

// AgentInfo describes an agent for the registry.
type AgentInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// catalog is the canonical list of agents. Update here to add/rename one.
var catalog = []AgentInfo{
	{"react", "ReAct Explorer", "Dynamically explores codebase using tools to identify architectural patterns"},
	{"code", "Code Analysis", "Analyzes source code structure, patterns, and architecture"},
	{"deps", "Dependencies", "Analyzes project dependencies and their purposes"},
	{"doc", "Documentation", "Extracts knowledge from README and documentation files"},
	{"git", "Git History", "Extracts decisions and patterns from git commit history"},
	{"clarifying", "Goal Clarification", "Refines user goals by asking clarifying questions"},
	{"planning", "Task Planning", "Decomposes goals into actionable tasks with dependencies"},
	{"decomposition", "Goal Decomposition", "Breaks enriched goals into high-level phases"},
	{"expand", "Phase Expansion", "Expands phases into detailed tasks"},
	{"simplify", "Code Simplification", "Reduces code complexity and line count"},
	{"explain", "Code Explanation", "Provides deep-dive explanations of code and concepts"},
	{"debug", "Debug Helper", "Helps diagnose issues systematically"},
}

// Registry returns the catalog of all known agents. Used by the dashboard.
func Registry() []AgentInfo {
	out := make([]AgentInfo, len(catalog))
	copy(out, catalog)
	return out
}
