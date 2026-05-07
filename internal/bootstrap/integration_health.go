package bootstrap

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// Ownership describes who owns a configuration artifact.
type Ownership string

const (
	OwnershipManaged   Ownership = "managed"
	OwnershipUnmanaged Ownership = "unmanaged"
	OwnershipNone      Ownership = "none"
)

// ComponentStatus describes artifact health at the component level.
type ComponentStatus string

const (
	ComponentStatusOK      ComponentStatus = "ok"
	ComponentStatusMissing ComponentStatus = "missing"
	ComponentStatusInvalid ComponentStatus = "invalid"
	ComponentStatusStale   ComponentStatus = "stale"
)

// AIComponent identifies a concrete integration component.
type AIComponent string

const (
	AIComponentCommands AIComponent = "commands"
	AIComponentHooks    AIComponent = "hooks"
	AIComponentPlugin   AIComponent = "plugin"
)

// IntegrationIssue is a normalized drift signal used by bootstrap + doctor.
type IntegrationIssue struct {
	AI            string          `json:"ai"`
	Component     AIComponent     `json:"component"`
	Ownership     Ownership       `json:"ownership"`
	Status        ComponentStatus `json:"status"`
	Reason        string          `json:"reason"`
	AutoFixable   bool            `json:"auto_fixable"`
	MutatesGlobal bool            `json:"mutates_global"`
	AdoptRequired bool            `json:"adopt_required"`
}

// IntegrationReport is per-AI evaluation output from the shared evaluator.
type IntegrationReport struct {
	AI                    string                          `json:"ai"`
	Issues                []IntegrationIssue              `json:"issues,omitempty"`
	ComponentStatuses     map[AIComponent]ComponentStatus `json:"component_statuses,omitempty"`
	ComponentOwnership    map[AIComponent]Ownership       `json:"component_ownership,omitempty"`
	ManagedLocalDrift     bool                            `json:"managed_local_drift"`
	UnmanagedDrift        bool                            `json:"unmanaged_drift"`
	CommandsDirExists     bool                            `json:"commands_dir_exists"`
	MarkerFileExists      bool                            `json:"marker_file_exists"`
	CommandFilesCount     int                             `json:"command_files_count"`
	HooksConfigExists     bool                            `json:"hooks_config_exists"`
	HooksConfigValid      bool                            `json:"hooks_config_valid"`
	TaskWingLikeUnmanaged bool                            `json:"taskwing_like_unmanaged"`
}

// RepairAction is an executable fix step derived from integration issues.
type RepairAction struct {
	AI               string      `json:"ai"`
	Component        AIComponent `json:"component"`
	Primitive        string      `json:"primitive"`
	Apply            bool        `json:"apply"`
	Reason           string      `json:"reason,omitempty"`
	MutatesGlobal    bool        `json:"mutates_global"`
	RequiresAdoption bool        `json:"requires_adoption"`
}

// RepairPlan is an ordered collection of remediation actions.
type RepairPlan struct {
	Actions []RepairAction `json:"actions"`
}

// RepairPlanOptions tunes what kind of actions become applicable.
type RepairPlanOptions struct {
	TargetAIs                []string
	IncludeGlobalMutations   bool
	IncludeUnmanagedAdoption bool
}

// EvaluateIntegrations runs a shared health evaluation across all supported AIs.
func EvaluateIntegrations(basePath string) map[string]IntegrationReport {
	reports := make(map[string]IntegrationReport, len(ValidAINames()))
	for _, ai := range ValidAINames() {
		reports[ai] = EvaluateIntegration(basePath, ai)
	}
	return reports
}

// EvaluateIntegration evaluates one AI integration from filesystem state.
func EvaluateIntegration(basePath, aiName string) IntegrationReport {
	report := IntegrationReport{
		AI:                 aiName,
		ComponentStatuses:  make(map[AIComponent]ComponentStatus),
		ComponentOwnership: make(map[AIComponent]Ownership),
	}

	cfg, ok := aiHelpers[aiName]
	if !ok {
		return report
	}

	commandsStatus, commandsOwner, cmdExists, cmdMarker, cmdCount, cmdTaskwingLike, cmdReason := evalCommandsComponent(basePath, aiName, cfg)
	report.CommandsDirExists = cmdExists
	report.MarkerFileExists = cmdMarker
	report.CommandFilesCount = cmdCount
	report.TaskWingLikeUnmanaged = cmdTaskwingLike && commandsOwner == OwnershipUnmanaged
	report.ComponentStatuses[AIComponentCommands] = commandsStatus
	report.ComponentOwnership[AIComponentCommands] = commandsOwner
	if commandsStatus != ComponentStatusOK &&
		(commandsStatus != ComponentStatusMissing || commandsOwner != OwnershipNone) {
		report.Issues = append(report.Issues, IntegrationIssue{
			AI:            aiName,
			Component:     AIComponentCommands,
			Ownership:     commandsOwner,
			Status:        commandsStatus,
			Reason:        cmdReason,
			AutoFixable:   commandsOwner == OwnershipManaged,
			MutatesGlobal: false,
			AdoptRequired: commandsOwner == OwnershipUnmanaged,
		})
	}
	localConfigured := commandsOwner != OwnershipNone || commandsStatus != ComponentStatusMissing || cmdExists

	if aiName == "claude" || aiName == "codex" {
		if commandsStatus != ComponentStatusMissing {
			hookStatus, hookOwner, hookExists, hookValid, hookReason := evalHooksComponent(basePath, aiName, commandsOwner)
			report.HooksConfigExists = hookExists
			report.HooksConfigValid = hookValid
			report.ComponentStatuses[AIComponentHooks] = hookStatus
			report.ComponentOwnership[AIComponentHooks] = hookOwner
			if hookStatus != ComponentStatusOK {
				report.Issues = append(report.Issues, IntegrationIssue{
					AI:            aiName,
					Component:     AIComponentHooks,
					Ownership:     hookOwner,
					Status:        hookStatus,
					Reason:        hookReason,
					AutoFixable:   hookOwner == OwnershipManaged,
					MutatesGlobal: false,
					AdoptRequired: hookOwner == OwnershipUnmanaged,
				})
			}
		}

	}

	if aiName == "opencode" {
		pluginPath := filepath.Join(basePath, ".opencode", "plugins", "taskwing-hooks.js")
		opencodeConfigPath := filepath.Join(basePath, "opencode.json")
		opencodeConfigured := localConfigured || pathExists(pluginPath) || pathExists(opencodeConfigPath)

		if opencodeConfigured {
			pluginStatus, pluginOwner, pluginReason := evalOpenCodePluginComponent(basePath, commandsOwner)
			report.ComponentStatuses[AIComponentPlugin] = pluginStatus
			report.ComponentOwnership[AIComponentPlugin] = pluginOwner
			if pluginStatus != ComponentStatusOK {
				report.Issues = append(report.Issues, IntegrationIssue{
					AI:            aiName,
					Component:     AIComponentPlugin,
					Ownership:     pluginOwner,
					Status:        pluginStatus,
					Reason:        pluginReason,
					AutoFixable:   pluginOwner == OwnershipManaged,
					MutatesGlobal: false,
					AdoptRequired: pluginOwner == OwnershipUnmanaged,
				})
			}
		}
	}

	for _, issue := range report.Issues {
		switch {
		case issue.Ownership == OwnershipManaged:
			report.ManagedLocalDrift = true
		case issue.Ownership == OwnershipUnmanaged:
			report.UnmanagedDrift = true
		}
	}

	return report
}

// BuildRepairPlan translates integration issues into executable repair actions.
func BuildRepairPlan(reports map[string]IntegrationReport, opts RepairPlanOptions) RepairPlan {
	targetSet := make(map[string]struct{})
	for _, ai := range opts.TargetAIs {
		trimmed := strings.TrimSpace(ai)
		if trimmed != "" {
			targetSet[trimmed] = struct{}{}
		}
	}

	keys := make([]string, 0, len(reports))
	for ai := range reports {
		keys = append(keys, ai)
	}
	sort.Strings(keys)

	plan := RepairPlan{Actions: []RepairAction{}}
	for _, ai := range keys {
		if len(targetSet) > 0 {
			if _, ok := targetSet[ai]; !ok {
				continue
			}
		}
		report := reports[ai]
		for _, issue := range report.Issues {
			action := RepairAction{
				AI:               issue.AI,
				Component:        issue.Component,
				Primitive:        primitiveForComponent(issue.Component),
				Apply:            issue.AutoFixable,
				Reason:           issue.Reason,
				MutatesGlobal:    issue.MutatesGlobal,
				RequiresAdoption: issue.AdoptRequired,
			}
			if issue.MutatesGlobal && !opts.IncludeGlobalMutations {
				action.Apply = false
				action.Reason = "global mutation disabled"
			}
			if issue.AdoptRequired {
				action.Primitive = "adopt_and_" + action.Primitive
				action.Apply = opts.IncludeUnmanagedAdoption
				if !opts.IncludeUnmanagedAdoption {
					action.Reason = "adoption required (use --adopt-unmanaged)"
				}
			}
			plan.Actions = append(plan.Actions, action)
		}
	}

	return plan
}

func primitiveForComponent(component AIComponent) string {
	switch component {
	case AIComponentCommands:
		return "repairCommands"
	case AIComponentHooks:
		return "repairHooks"
	case AIComponentPlugin:
		return "repairPlugin"
	default:
		return "repairUnknown"
	}
}

func evalCommandsComponent(basePath, aiName string, cfg aiHelperConfig) (ComponentStatus, Ownership, bool, bool, int, bool, string) {
	if cfg.singleFile {
		filePath := filepath.Join(basePath, cfg.commandsDir, cfg.singleFileName)
		content, err := os.ReadFile(filePath)
		if err != nil {
			return ComponentStatusMissing, OwnershipNone, false, false, 0, false, fmt.Sprintf("%s missing", cfg.singleFileName)
		}
		text := string(content)
		managed := strings.Contains(text, "<!-- TASKWING_MANAGED -->")
		taskwingLike := managed || strings.Contains(strings.ToLower(text), "taskwing")
		if managed {
			version := parseEmbeddedVersion(text)
			if version != "" && version != AIToolConfigVersion(aiName) {
				return ComponentStatusStale, OwnershipManaged, true, true, 1, true, "managed instructions version mismatch"
			}
			return ComponentStatusOK, OwnershipManaged, true, true, 1, true, ""
		}
		if taskwingLike {
			return ComponentStatusStale, OwnershipUnmanaged, true, false, 1, true, "taskwing-like unmanaged instructions detected"
		}
		return ComponentStatusOK, OwnershipUnmanaged, true, false, 1, false, ""
	}

	commandsDir := filepath.Join(basePath, cfg.commandsDir)
	info, err := os.Stat(commandsDir)
	if err != nil || !info.IsDir() {
		return ComponentStatusMissing, OwnershipNone, false, false, 0, false, "commands directory missing"
	}

	markerPath := filepath.Join(commandsDir, TaskWingManagedFile)
	_, markerErr := os.Stat(markerPath)
	managed := markerErr == nil
	ownership := OwnershipUnmanaged
	if managed {
		ownership = OwnershipManaged
	}

	expected := expectedSlashCommandFiles(cfg.fileExt)

	// Check for legacy flat tw-* files in commands root
	entries, _ := os.ReadDir(commandsDir)
	commandFileCount := 0
	taskwingLike := false
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, cfg.fileExt) {
			commandFileCount++
			if strings.HasPrefix(strings.TrimSuffix(name, cfg.fileExt), "tw-") {
				taskwingLike = true
			}
		}
	}

	// Check the taskwing/ namespace subdirectory for expected command files
	actual := map[string]struct{}{}
	nsDir := filepath.Join(commandsDir, slashCommandNamespace)
	nsEntries, _ := os.ReadDir(nsDir)
	for _, entry := range nsEntries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, cfg.fileExt) {
			commandFileCount++
			actual[name] = struct{}{}
			taskwingLike = true
		}
	}

	missing := make([]string, 0)
	for name := range expected {
		if _, ok := actual[name]; !ok {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)

	if !taskwingLike {
		matches, _ := filepath.Glob(filepath.Join(commandsDir, "*."+strings.TrimPrefix(cfg.fileExt, ".")))
		for _, match := range matches {
			b, readErr := os.ReadFile(match)
			if readErr != nil {
				continue
			}
			if strings.Contains(strings.ToLower(string(b)), "taskwing") {
				taskwingLike = true
				break
			}
		}
	}

	if ownership == OwnershipManaged {
		if len(missing) > 0 {
			return ComponentStatusMissing, ownership, true, true, commandFileCount, taskwingLike, fmt.Sprintf("missing expected command files: %s", strings.Join(missing, ", "))
		}
		markerVersion := parseManagedMarkerVersion(markerPath)
		if markerVersion != "" && markerVersion != AIToolConfigVersion(aiName) {
			return ComponentStatusStale, ownership, true, true, commandFileCount, taskwingLike, "managed marker version mismatch"
		}
		if markerVersion == "" {
			return ComponentStatusStale, ownership, true, true, commandFileCount, taskwingLike, "managed marker missing version"
		}
		return ComponentStatusOK, ownership, true, true, commandFileCount, taskwingLike, ""
	}

	if commandFileCount == 0 {
		return ComponentStatusStale, ownership, true, false, commandFileCount, false, "commands directory exists but no command files"
	}

	if taskwingLike {
		if len(missing) > 0 {
			return ComponentStatusStale, ownership, true, false, commandFileCount, true, fmt.Sprintf("taskwing-like unmanaged directory missing expected files: %s", strings.Join(missing, ", "))
		}
		return ComponentStatusStale, ownership, true, false, commandFileCount, true, "taskwing-like unmanaged directory (adoption recommended)"
	}

	return ComponentStatusOK, ownership, true, false, commandFileCount, false, ""
}

func evalHooksComponent(basePath, aiName string, commandsOwnership Ownership) (ComponentStatus, Ownership, bool, bool, string) {
	settingsPath := filepath.Join(basePath, "."+aiName, "settings.json")
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		owner := commandsOwnership
		if owner == OwnershipNone {
			owner = OwnershipManaged
		}
		return ComponentStatusMissing, owner, false, false, "hooks config missing"
	}

	owner := commandsOwnership
	if owner == OwnershipNone {
		if strings.Contains(strings.ToLower(string(content)), "taskwing hook") {
			owner = OwnershipUnmanaged
		} else {
			owner = OwnershipManaged
		}
	}

	var parsed map[string]any
	if err := json.Unmarshal(content, &parsed); err != nil {
		return ComponentStatusInvalid, owner, true, false, "hooks config invalid JSON"
	}

	hooksRaw, ok := parsed["hooks"]
	if !ok {
		return ComponentStatusInvalid, owner, true, true, "hooks key missing"
	}
	hooksMap, ok := hooksRaw.(map[string]any)
	if !ok {
		return ComponentStatusInvalid, owner, true, true, "hooks key has invalid type"
	}

	if _, hasStop := hooksMap["Stop"]; !hasStop {
		return ComponentStatusInvalid, owner, true, true, "required Stop hook missing"
	}
	if !HookEventContainsCommand(hooksMap, "Stop", "hook continue-check") {
		return ComponentStatusInvalid, owner, true, true, "required Stop hook command missing taskwing continue-check"
	}
	missingRecommended := make([]string, 0)
	if _, hasSessionStart := hooksMap["SessionStart"]; !hasSessionStart {
		missingRecommended = append(missingRecommended, "SessionStart")
	} else if !HookEventContainsCommand(hooksMap, "SessionStart", "hook session-init") {
		missingRecommended = append(missingRecommended, "SessionStart(command)")
	}
	if _, hasSessionEnd := hooksMap["SessionEnd"]; !hasSessionEnd {
		missingRecommended = append(missingRecommended, "SessionEnd")
	} else if !HookEventContainsCommand(hooksMap, "SessionEnd", "hook session-end") {
		missingRecommended = append(missingRecommended, "SessionEnd(command)")
	}
	if len(missingRecommended) > 0 {
		return ComponentStatusStale, owner, true, true, fmt.Sprintf("recommended hooks missing: %s", strings.Join(missingRecommended, ", "))
	}

	return ComponentStatusOK, owner, true, true, ""
}

// HookEventContainsCommand returns true when the hook event contains a command
// entry whose command field includes requiredSubstr (case-insensitive).
func HookEventContainsCommand(hooksMap map[string]any, eventName, requiredSubstr string) bool {
	rawEvent, ok := hooksMap[eventName]
	if !ok {
		return false
	}
	eventEntries, ok := rawEvent.([]any)
	if !ok {
		return false
	}
	required := strings.ToLower(strings.TrimSpace(requiredSubstr))
	if required == "" {
		return false
	}

	for _, entry := range eventEntries {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		rawHooks, ok := entryMap["hooks"]
		if !ok {
			continue
		}
		hookCommands, ok := rawHooks.([]any)
		if !ok {
			continue
		}
		for _, cmdEntry := range hookCommands {
			cmdMap, ok := cmdEntry.(map[string]any)
			if !ok {
				continue
			}
			cmdStr, _ := cmdMap["command"].(string)
			if strings.Contains(strings.ToLower(cmdStr), required) {
				return true
			}
		}
	}

	return false
}

func evalOpenCodePluginComponent(basePath string, commandsOwnership Ownership) (ComponentStatus, Ownership, string) {
	pluginPath := filepath.Join(basePath, ".opencode", "plugins", "taskwing-hooks.js")
	content, err := os.ReadFile(pluginPath)
	if err != nil {
		owner := commandsOwnership
		if owner == OwnershipNone {
			owner = OwnershipManaged
		}
		return ComponentStatusMissing, owner, "OpenCode plugin missing"
	}

	text := string(content)
	managed := strings.Contains(text, "TASKWING_MANAGED_PLUGIN")
	owner := OwnershipUnmanaged
	if managed {
		owner = OwnershipManaged
	}
	taskwingLike := managed || strings.Contains(strings.ToLower(text), "taskwing hook")

	requiredFragments := []string{"session.created", "session.idle", "taskwing hook session-init", "taskwing hook continue-check"}
	for _, fragment := range requiredFragments {
		if !strings.Contains(strings.ToLower(text), strings.ToLower(fragment)) {
			if owner == OwnershipManaged {
				return ComponentStatusStale, owner, fmt.Sprintf("plugin missing required fragment: %s", fragment)
			}
			if taskwingLike {
				return ComponentStatusStale, owner, fmt.Sprintf("taskwing-like unmanaged plugin missing required fragment: %s", fragment)
			}
			return ComponentStatusOK, owner, ""
		}
	}

	if owner == OwnershipUnmanaged && taskwingLike {
		return ComponentStatusStale, owner, "taskwing-like unmanaged OpenCode plugin"
	}

	return ComponentStatusOK, owner, ""
}

func pathExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func parseManagedMarkerVersion(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if strings.HasPrefix(strings.ToLower(line), "# version:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# Version:"))
		}
	}
	return ""
}

func parseEmbeddedVersion(content string) string {
	const prefix = "<!-- Version:"
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			trimmed := strings.TrimPrefix(line, prefix)
			trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, "-->"))
			return trimmed
		}
	}
	return ""
}

// ManagedLocalDriftAIs returns all AIs with managed local drift.
func ManagedLocalDriftAIs(reports map[string]IntegrationReport) []string {
	out := make([]string, 0)
	for ai, report := range reports {
		if report.ManagedLocalDrift {
			out = append(out, ai)
		}
	}
	sort.Strings(out)
	return out
}

// UnmanagedDriftAIs returns all AIs with unmanaged drift.
func UnmanagedDriftAIs(reports map[string]IntegrationReport) []string {
	out := make([]string, 0)
	for ai, report := range reports {
		if report.UnmanagedDrift {
			out = append(out, ai)
		}
	}
	sort.Strings(out)
	return out
}

// HasManagedLocalDrift checks whether any AI has managed local drift.
func HasManagedLocalDrift(reports map[string]IntegrationReport) bool {
	for _, report := range reports {
		if report.ManagedLocalDrift {
			return true
		}
	}
	return false
}

// AIHasIssueComponent reports whether the AI has an issue for a component.
func AIHasIssueComponent(report IntegrationReport, component AIComponent) bool {
	for _, issue := range report.Issues {
		if issue.Component == component {
			return true
		}
	}
	return false
}

// IsTaskWingLikeUnmanaged determines whether an unmanaged config resembles TaskWing output.
func IsTaskWingLikeUnmanaged(report IntegrationReport) bool {
	return report.TaskWingLikeUnmanaged || (report.UnmanagedDrift && slices.ContainsFunc(report.Issues, func(issue IntegrationIssue) bool {
		return issue.Ownership == OwnershipUnmanaged && issue.AdoptRequired
	}))
}
