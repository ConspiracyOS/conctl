package runner

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ConspiracyOS/conctl/internal/config"
)

func TestPickOldestTask(t *testing.T) {
	inbox := t.TempDir()

	// Create task files (numbered for ordering)
	os.WriteFile(filepath.Join(inbox, "002-second.task"), []byte("second task"), 0644)
	os.WriteFile(filepath.Join(inbox, "001-first.task"), []byte("first task"), 0644)
	os.WriteFile(filepath.Join(inbox, "003-third.task"), []byte("third task"), 0644)

	task, err := PickOldestTask(inbox)
	if err != nil {
		t.Fatalf("PickOldestTask failed: %v", err)
	}
	if filepath.Base(task.Path) != "001-first.task" {
		t.Errorf("expected 001-first.task, got %s", filepath.Base(task.Path))
	}
	if task.Content != "first task" {
		t.Errorf("unexpected content: %q", task.Content)
	}
}

func TestPickOldestTask_BadInboxPath(t *testing.T) {
	_, err := PickOldestTask("/nonexistent/inbox/path/xyz-abc")
	if err == nil {
		t.Error("expected error for nonexistent inbox path")
	}
}

func TestPickOldestTaskEmptyInbox(t *testing.T) {
	inbox := t.TempDir()

	_, err := PickOldestTask(inbox)
	if err == nil {
		t.Error("expected error for empty inbox")
	}
}

func TestRouteOutput(t *testing.T) {
	agentDir := t.TempDir()
	outbox := filepath.Join(agentDir, "outbox")
	processed := filepath.Join(agentDir, "processed")
	os.MkdirAll(outbox, 0755)
	os.MkdirAll(processed, 0755)

	task := Task{
		Path:    filepath.Join(agentDir, "inbox", "001-test.task"),
		Content: "original task",
	}
	os.MkdirAll(filepath.Dir(task.Path), 0755)
	os.WriteFile(task.Path, []byte(task.Content), 0644)

	output := "Task completed successfully"
	err := RouteOutput(task, output, outbox, processed)
	if err != nil {
		t.Fatalf("RouteOutput failed: %v", err)
	}

	// Check outbox has the response
	files, _ := os.ReadDir(outbox)
	if len(files) == 0 {
		t.Error("expected output file in outbox")
	}

	// Check task was moved to processed
	_, err = os.Stat(task.Path)
	if !os.IsNotExist(err) {
		t.Error("expected task to be moved from inbox")
	}

	processedFiles, _ := os.ReadDir(processed)
	if len(processedFiles) == 0 {
		t.Error("expected task in processed dir")
	}
}

func TestMoveOuterInboxTasks(t *testing.T) {
	// Patch the outer inbox and concierge inbox to temp dirs using env vars
	// Since MoveOuterInboxTasks uses hardcoded paths, we test it via a wrapper
	// by temporarily monkey-patching via a helper that accepts paths.
	outerInbox := t.TempDir()
	conciergeInbox := t.TempDir()

	// Create a task in the outer inbox
	os.WriteFile(filepath.Join(outerInbox, "001-test.task"), []byte("task content"), 0644)
	// Create a non-task file (should be ignored)
	os.WriteFile(filepath.Join(outerInbox, "README.txt"), []byte("not a task"), 0644)

	err := moveOuterInboxTasksTo(outerInbox, conciergeInbox)
	if err != nil {
		t.Fatalf("moveOuterInboxTasksTo failed: %v", err)
	}

	// Task should be in concierge inbox
	_, err = os.Stat(filepath.Join(conciergeInbox, "001-test.task"))
	if os.IsNotExist(err) {
		t.Error("expected task to be moved to concierge inbox")
	}

	// Task should be gone from outer inbox
	_, err = os.Stat(filepath.Join(outerInbox, "001-test.task"))
	if !os.IsNotExist(err) {
		t.Error("expected task to be removed from outer inbox")
	}

	// Non-task file should remain
	_, err = os.Stat(filepath.Join(outerInbox, "README.txt"))
	if os.IsNotExist(err) {
		t.Error("expected non-task file to remain in outer inbox")
	}
}

func TestPickOldestTaskTrust(t *testing.T) {
	inbox := t.TempDir()
	os.WriteFile(filepath.Join(inbox, "001-test.task"), []byte("test content"), 0644)

	task, err := PickOldestTask(inbox)
	if err != nil {
		t.Fatalf("PickOldestTask failed: %v", err)
	}

	// Files created by test process (non-root) should be unverified
	if task.Trust != TrustUnverified {
		t.Errorf("expected TrustUnverified for non-root file, got %s", task.Trust)
	}
}

func TestTrustLevelString(t *testing.T) {
	if TrustVerified.String() != "verified" {
		t.Errorf("TrustVerified.String() = %q, want %q", TrustVerified.String(), "verified")
	}
	if TrustUnverified.String() != "unverified" {
		t.Errorf("TrustUnverified.String() = %q, want %q", TrustUnverified.String(), "unverified")
	}
}

func TestFrameTaskPrompt_Verified(t *testing.T) {
	task := Task{Content: "do something", Trust: TrustVerified}
	prompt := FrameTaskPrompt(task)

	if !strings.Contains(prompt, "Task from verified source") {
		t.Error("verified task prompt should contain 'Task from verified source'")
	}
	if !strings.Contains(prompt, "do something") {
		t.Error("prompt should contain task content")
	}
}

func TestFrameTaskPrompt_Unverified(t *testing.T) {
	task := Task{Content: "do something", Trust: TrustUnverified}
	prompt := FrameTaskPrompt(task)

	if !strings.Contains(prompt, "unverified source") {
		t.Error("unverified task prompt should contain 'unverified source'")
	}
	if !strings.Contains(prompt, "exercise additional scrutiny") {
		t.Error("unverified prompt should advise scrutiny on external interactions")
	}
	if !strings.Contains(prompt, "do something") {
		t.Error("prompt should contain task content")
	}
}

func TestIsTrustedUID_Root(t *testing.T) {
	if !isTrustedUID(0) {
		t.Error("uid 0 should always be trusted")
	}
}

func TestIsTrustedUID_NonRoot(t *testing.T) {
	uid := uint32(os.Getuid())
	if uid == 0 {
		t.Skip("test must run as non-root")
	}
	if isTrustedUID(uid) {
		t.Error("non-root user without trusted group membership should not be trusted")
	}
}

func TestIsTrustedUID_UnknownUID(t *testing.T) {
	// A UID that almost certainly has no corresponding user; covers the
	// user.LookupId error return path.
	if isTrustedUID(999999) {
		t.Error("unknown UID should not be trusted")
	}
}

func TestPickOldestTaskOrder(t *testing.T) {
	inbox := t.TempDir()

	os.WriteFile(filepath.Join(inbox, "003.task"), []byte("third"), 0644)
	os.WriteFile(filepath.Join(inbox, "001.task"), []byte("first"), 0644)
	os.WriteFile(filepath.Join(inbox, "002.task"), []byte("second"), 0644)

	task, err := PickOldestTask(inbox)
	if err != nil {
		t.Fatalf("PickOldestTask failed: %v", err)
	}
	if filepath.Base(task.Path) != "001.task" {
		t.Errorf("expected 001.task to be picked first, got %s", filepath.Base(task.Path))
	}
	if task.Content != "first" {
		t.Errorf("expected content %q, got %q", "first", task.Content)
	}
}

func TestPickOldestTask_UnreadableFile(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root (chmod 0000 won't prevent root reads)")
	}
	inbox := t.TempDir()
	taskPath := filepath.Join(inbox, "001-unreadable.task")
	os.WriteFile(taskPath, []byte("secret"), 0644)
	os.Chmod(taskPath, 0000)
	defer os.Chmod(taskPath, 0644)

	_, err := PickOldestTask(inbox)
	if err == nil {
		t.Error("expected error when task file is unreadable")
	}
}

func TestPickOldestTaskOversize(t *testing.T) {
	inbox := t.TempDir()

	// Create a task file larger than 32KB
	bigContent := strings.Repeat("x", 33*1024)
	taskPath := filepath.Join(inbox, "001-big.task")
	os.WriteFile(taskPath, []byte(bigContent), 0644)

	task, err := PickOldestTask(inbox)
	if err != nil {
		t.Fatalf("PickOldestTask failed: %v", err)
	}
	if !strings.HasPrefix(task.Content, "[Attachment: file too large") {
		t.Errorf("expected attachment reference for oversized task, got: %q", task.Content[:80])
	}
	if !strings.Contains(task.Content, taskPath) {
		t.Errorf("attachment reference should include task path, got: %q", task.Content)
	}
}

func TestRouteOutputTimestamp(t *testing.T) {
	agentDir := t.TempDir()
	outbox := filepath.Join(agentDir, "outbox")
	processed := filepath.Join(agentDir, "processed")
	os.MkdirAll(outbox, 0755)
	os.MkdirAll(processed, 0755)

	inbox := filepath.Join(agentDir, "inbox")
	os.MkdirAll(inbox, 0755)
	taskPath := filepath.Join(inbox, "007-mytask.task")
	os.WriteFile(taskPath, []byte("task body"), 0644)

	task := Task{Path: taskPath, Content: "task body"}
	if err := RouteOutput(task, "done", outbox, processed); err != nil {
		t.Fatalf("RouteOutput failed: %v", err)
	}

	files, _ := os.ReadDir(outbox)
	if len(files) != 1 {
		t.Fatalf("expected 1 file in outbox, got %d", len(files))
	}
	name := files[0].Name()
	// Name format: <timestamp>-<taskbase>.response  e.g. 20260228-153000-007-mytask.response
	if !strings.HasSuffix(name, "-007-mytask.response") {
		t.Errorf("output filename should end with task basename (without .task): got %q", name)
	}
	// Timestamp prefix: 8 digits + '-' + 6 digits
	if len(name) < 16 || name[8] != '-' {
		t.Errorf("output filename should start with YYYYMMDD-HHMMSS timestamp: got %q", name)
	}
}

func TestRouteOutputMissingTask(t *testing.T) {
	agentDir := t.TempDir()
	outbox := filepath.Join(agentDir, "outbox")
	processed := filepath.Join(agentDir, "processed")
	os.MkdirAll(outbox, 0755)
	os.MkdirAll(processed, 0755)

	// task.Path does not exist — rename will get ENOENT, which should be tolerated
	task := Task{
		Path:    filepath.Join(agentDir, "inbox", "ghost.task"),
		Content: "never existed",
	}
	if err := RouteOutput(task, "output", outbox, processed); err != nil {
		t.Errorf("RouteOutput should tolerate missing task file (ENOENT), got: %v", err)
	}
}

func TestMoveOuterInboxCrossDevice(t *testing.T) {
	outerInbox := t.TempDir()
	conciergeInbox := t.TempDir()

	// Write multiple tasks and a non-task file
	os.WriteFile(filepath.Join(outerInbox, "002.task"), []byte("b"), 0644)
	os.WriteFile(filepath.Join(outerInbox, "001.task"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(outerInbox, "readme.txt"), []byte("ignore me"), 0644)

	if err := moveOuterInboxTasksTo(outerInbox, conciergeInbox); err != nil {
		t.Fatalf("moveOuterInboxTasksTo failed: %v", err)
	}

	// Both task files should appear in concierge inbox
	for _, name := range []string{"001.task", "002.task"} {
		if _, err := os.Stat(filepath.Join(conciergeInbox, name)); os.IsNotExist(err) {
			t.Errorf("expected %s in concierge inbox", name)
		}
		if _, err := os.Stat(filepath.Join(outerInbox, name)); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed from outer inbox", name)
		}
	}

	// Non-task file should remain untouched in outer inbox
	if _, err := os.Stat(filepath.Join(outerInbox, "readme.txt")); os.IsNotExist(err) {
		t.Error("expected readme.txt to remain in outer inbox")
	}
}

func TestReadSkills(t *testing.T) {
	skillsDir := t.TempDir()

	os.WriteFile(filepath.Join(skillsDir, "alpha.md"), []byte("alpha content"), 0644)
	os.WriteFile(filepath.Join(skillsDir, "beta.md"), []byte("beta content"), 0644)
	os.WriteFile(filepath.Join(skillsDir, "notes.txt"), []byte("should be ignored"), 0644)
	os.MkdirAll(filepath.Join(skillsDir, "subdir"), 0755)
	os.WriteFile(filepath.Join(skillsDir, "subdir", "nested.md"), []byte("should be ignored"), 0644)

	result := ReadSkills(skillsDir)

	if !strings.Contains(result, "## Skill: alpha") {
		t.Error("expected alpha skill section in output")
	}
	if !strings.Contains(result, "alpha content") {
		t.Error("expected alpha skill content in output")
	}
	if !strings.Contains(result, "## Skill: beta") {
		t.Error("expected beta skill section in output")
	}
	if !strings.Contains(result, "beta content") {
		t.Error("expected beta skill content in output")
	}
	if strings.Contains(result, "should be ignored") {
		t.Error("non-.md files and subdirectory files should not appear in skill output")
	}
}

func TestReadSkillsEmpty(t *testing.T) {
	// Directory exists but has no .md files
	skillsDir := t.TempDir()
	os.WriteFile(filepath.Join(skillsDir, "notes.txt"), []byte("not a skill"), 0644)

	if result := ReadSkills(skillsDir); result != "" {
		t.Errorf("expected empty string for dir with no .md files, got: %q", result)
	}
}

func TestReadSkillsMissingDir(t *testing.T) {
	result := ReadSkills("/nonexistent/path/to/skills")
	if result != "" {
		t.Errorf("expected empty string for missing dir, got: %q", result)
	}
}

func TestPickOldestTask_SymlinkUnverified(t *testing.T) {
	inbox := t.TempDir()
	targetDir := t.TempDir()

	// Create a real file in another directory (simulates a root-owned file)
	targetPath := filepath.Join(targetDir, "real.task")
	os.WriteFile(targetPath, []byte("spoofed task"), 0644)

	// Create a symlink in the inbox pointing to the target
	symlinkPath := filepath.Join(inbox, "001-spoof.task")
	os.Symlink(targetPath, symlinkPath)

	task, err := PickOldestTask(inbox)
	if err != nil {
		t.Fatalf("PickOldestTask failed: %v", err)
	}
	if task.Trust != TrustUnverified {
		t.Errorf("symlinked task should be TrustUnverified, got %s", task.Trust)
	}
	if task.Content != "spoofed task" {
		t.Errorf("content should still be readable through symlink, got %q", task.Content)
	}
}

func TestAssembleAgentsMD_NonExistentDir(t *testing.T) {
	// Assembler fails when ConfigBase doesn't exist and no InlineInstructions.
	agent := config.AgentConfig{Name: "nonexistent-test-xyz"}
	dirs := Dirs{HomeBase: t.TempDir(), StateBase: "/nonexistent-xyz", ConfigBase: "/nonexistent-xyz"}
	err := AssembleAgentsMD(agent, dirs)
	if err == nil {
		t.Error("expected error when config base does not exist")
	}
}

func TestPickOldestTask_TrustVerified(t *testing.T) {
	uid := uint32(os.Getuid())
	if uid == 0 {
		t.Skip("test must run as non-root")
	}

	u, err := user.Current()
	if err != nil {
		t.Fatalf("looking up current user: %v", err)
	}
	gids, err := u.GroupIds()
	if err != nil || len(gids) == 0 {
		t.Skip("cannot determine user groups")
	}
	g, err := user.LookupGroupId(gids[0])
	if err != nil {
		t.Skip("cannot look up group name")
	}

	old := TrustedGroupName
	TrustedGroupName = g.Name
	defer func() { TrustedGroupName = old }()

	inbox := t.TempDir()
	os.WriteFile(filepath.Join(inbox, "001-trusted.task"), []byte("trusted task"), 0644)

	task, err := PickOldestTask(inbox)
	if err != nil {
		t.Fatalf("PickOldestTask failed: %v", err)
	}
	if task.Trust != TrustVerified {
		t.Errorf("expected TrustVerified when user is in trusted group, got %s", task.Trust)
	}
}

func TestIsTrustedUID_NotInGroup(t *testing.T) {
	uid := uint32(os.Getuid())
	if uid == 0 {
		t.Skip("test must run as non-root")
	}

	u, err := user.Current()
	if err != nil {
		t.Fatalf("looking up current user: %v", err)
	}
	gids, err := u.GroupIds()
	if err != nil {
		t.Fatalf("getting group IDs: %v", err)
	}
	gidSet := make(map[string]bool)
	for _, g := range gids {
		gidSet[g] = true
	}

	// Find a system group that exists on this platform but the current user is NOT in.
	var foreignGroup string
	for _, candidate := range []string{"root", "wheel", "daemon", "sys", "nobody", "nogroup", "bin", "kmem"} {
		g, err := user.LookupGroup(candidate)
		if err != nil {
			continue
		}
		if !gidSet[g.Gid] {
			foreignGroup = candidate
			break
		}
	}
	if foreignGroup == "" {
		t.Skip("could not find a system group the current user is not in")
	}

	old := TrustedGroupName
	TrustedGroupName = foreignGroup
	defer func() { TrustedGroupName = old }()

	if isTrustedUID(uid) {
		t.Errorf("user should not be trusted when not in group %q", foreignGroup)
	}
}

func TestRouteOutput_WriteError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root (chmod 0555 won't prevent root writes)")
	}
	agentDir := t.TempDir()
	outbox := filepath.Join(agentDir, "outbox")
	processed := filepath.Join(agentDir, "processed")
	os.MkdirAll(outbox, 0555) // read-only — WriteFile will fail
	os.MkdirAll(processed, 0755)
	defer os.Chmod(outbox, 0755)

	task := Task{Path: filepath.Join(agentDir, "001.task")}
	err := RouteOutput(task, "output", outbox, processed)
	if err == nil {
		t.Error("expected error when outbox is read-only")
	}
}

func TestRouteOutput_RenameError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root (chmod 0555 won't prevent root writes)")
	}
	agentDir := t.TempDir()
	outbox := filepath.Join(agentDir, "outbox")
	processed := filepath.Join(agentDir, "processed")
	os.MkdirAll(outbox, 0755)
	os.MkdirAll(processed, 0555) // read-only — Rename can't create entry here
	defer os.Chmod(processed, 0755)

	taskPath := filepath.Join(agentDir, "001.task")
	os.WriteFile(taskPath, []byte("task content"), 0644)
	task := Task{Path: taskPath}

	err := RouteOutput(task, "output", outbox, processed)
	if err == nil {
		t.Error("expected error when processed dir is read-only (rename fails with EACCES)")
	}
}

func TestAssembleAgentsMD_WriteFails(t *testing.T) {
	// InlineInstructions makes Assemble succeed, but HomeBase doesn't exist → WriteFile fails.
	agent := config.AgentConfig{
		Name:         "test",
		Instructions: "You are a test agent with inline instructions.",
	}
	dirs := Dirs{HomeBase: "/nonexistent-xyz", StateBase: "/nonexistent-xyz", ConfigBase: "/nonexistent-xyz"}
	err := AssembleAgentsMD(agent, dirs)
	if err == nil {
		t.Error("expected error when agent home dir does not exist")
	}
}

func TestMoveOuterInboxTasks_NoInbox(t *testing.T) {
	// StateBase exists but contains no inbox subdir → error.
	dirs := Dirs{StateBase: t.TempDir()}
	err := MoveOuterInboxTasks(dirs)
	if err == nil {
		t.Error("expected error when outer inbox does not exist")
	}
}

func TestMoveOuterInboxTasksTo_CopyFallback(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root (chmod 0555 won't prevent root writes)")
	}
	outerInbox := t.TempDir()
	conciergeInbox := t.TempDir()

	os.WriteFile(filepath.Join(outerInbox, "001-copy.task"), []byte("copy fallback"), 0644)

	// Read-only source dir → Rename fails → copy+delete fallback runs.
	os.Chmod(outerInbox, 0555)
	defer os.Chmod(outerInbox, 0755)

	if err := moveOuterInboxTasksTo(outerInbox, conciergeInbox); err != nil {
		t.Fatalf("moveOuterInboxTasksTo should succeed via copy fallback: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(conciergeInbox, "001-copy.task"))
	if err != nil {
		t.Fatal("expected task in concierge inbox after copy fallback")
	}
	if string(data) != "copy fallback" {
		t.Errorf("unexpected content: %q", string(data))
	}
}

func TestMoveOuterInboxTasksTo_ReadError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root (chmod won't prevent root reads)")
	}
	outerInbox := t.TempDir()
	conciergeInbox := t.TempDir()

	taskPath := filepath.Join(outerInbox, "001-unreadable.task")
	os.WriteFile(taskPath, []byte("secret"), 0644)

	// File unreadable, dir read-only → Rename fails (no w on dir),
	// ReadFile also fails (no r on file) → readErr != nil → continue.
	os.Chmod(taskPath, 0000)
	os.Chmod(outerInbox, 0555)
	defer os.Chmod(taskPath, 0644)
	defer os.Chmod(outerInbox, 0755)

	if err := moveOuterInboxTasksTo(outerInbox, conciergeInbox); err != nil {
		t.Fatalf("moveOuterInboxTasksTo should return nil even when read fails: %v", err)
	}

	if _, err := os.Stat(filepath.Join(conciergeInbox, "001-unreadable.task")); !os.IsNotExist(err) {
		t.Error("task should not be in concierge inbox when read failed")
	}
}

func TestMoveOuterInboxTasksTo_WriteError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root (chmod won't prevent root writes)")
	}
	outerInbox := t.TempDir()
	conciergeInbox := t.TempDir()

	os.WriteFile(filepath.Join(outerInbox, "001-write-fail.task"), []byte("content"), 0644)

	// Source dir: read-only → Rename fails.
	// Destination dir: read-only → WriteFile also fails → writeErr != nil → continue.
	os.Chmod(outerInbox, 0555)
	os.Chmod(conciergeInbox, 0555)
	defer os.Chmod(outerInbox, 0755)
	defer os.Chmod(conciergeInbox, 0755)

	if err := moveOuterInboxTasksTo(outerInbox, conciergeInbox); err != nil {
		t.Fatalf("moveOuterInboxTasksTo should return nil even when write fails: %v", err)
	}

	if _, err := os.Stat(filepath.Join(conciergeInbox, "001-write-fail.task")); !os.IsNotExist(err) {
		t.Error("task should not be in concierge inbox when write failed")
	}
}

func TestRun_AgentNotFound(t *testing.T) {
	cfg := &config.Config{}
	err := Run("nonexistent-agent-xyz", cfg, DefaultDirs())
	if err == nil {
		t.Error("expected error for nonexistent agent")
	}
	if !strings.Contains(err.Error(), "not found in config") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_AgentsMDMissing(t *testing.T) {
	agentName := "test-runner-xyz"
	stateBase := t.TempDir()
	homeBase := t.TempDir()
	// Create the agent dirs structure but NOT the AGENTS.md file
	os.MkdirAll(filepath.Join(stateBase, "agents", agentName, "inbox"), 0755)
	cfg := &config.Config{
		Agents: []config.AgentConfig{
			{Name: agentName, Tier: "worker"},
		},
	}
	dirs := Dirs{HomeBase: homeBase, StateBase: stateBase, ConfigBase: t.TempDir()}
	err := Run(agentName, cfg, dirs)
	if err == nil {
		t.Error("expected error when AGENTS.md does not exist")
	}
	if !strings.Contains(err.Error(), "AGENTS.md") {
		t.Errorf("error should mention AGENTS.md, got: %v", err)
	}
}

func TestAssembleAgentsMD_Success(t *testing.T) {
	configBase := t.TempDir()
	homeBase := t.TempDir()

	// Set up a minimal base/AGENTS.md in the config root
	os.MkdirAll(filepath.Join(configBase, "base"), 0755)
	os.WriteFile(filepath.Join(configBase, "base", "AGENTS.md"), []byte("# Base\nYou are an agent.\n"), 0644)

	// Create the agent home dir
	agentName := "test"
	os.MkdirAll(filepath.Join(homeBase, "a-"+agentName), 0755)

	dirs := Dirs{HomeBase: homeBase, StateBase: t.TempDir(), ConfigBase: configBase}
	agent := config.AgentConfig{Name: agentName}

	if err := AssembleAgentsMD(agent, dirs); err != nil {
		t.Fatalf("AssembleAgentsMD failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(homeBase, "a-"+agentName, "AGENTS.md"))
	if err != nil {
		t.Fatalf("AGENTS.md not written: %v", err)
	}
	if !strings.Contains(string(content), "# Base") {
		t.Error("expected base content in written AGENTS.md")
	}
}

func TestAssembleAgentsMD_RootHash(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root: sha256 integrity file is only written during bootstrap (root)")
	}
	configBase := t.TempDir()
	homeBase := t.TempDir()
	os.MkdirAll(filepath.Join(configBase, "base"), 0755)
	os.WriteFile(filepath.Join(configBase, "base", "AGENTS.md"), []byte("# Base\n"), 0644)
	agentName := "test"
	os.MkdirAll(filepath.Join(homeBase, "a-"+agentName), 0755)
	dirs := Dirs{HomeBase: homeBase, StateBase: t.TempDir(), ConfigBase: configBase}

	if err := AssembleAgentsMD(config.AgentConfig{Name: agentName}, dirs); err != nil {
		t.Fatalf("AssembleAgentsMD failed: %v", err)
	}
	hashPath := filepath.Join(homeBase, "a-"+agentName, "AGENTS.md.sha256")
	if _, err := os.Stat(hashPath); os.IsNotExist(err) {
		t.Error("expected AGENTS.md.sha256 to be written when running as root")
	}
}

func setupRunDirs(t *testing.T, agentName string) (stateBase, homeBase string, dirs Dirs) {
	t.Helper()
	stateBase = t.TempDir()
	homeBase = t.TempDir()
	agentDir := filepath.Join(stateBase, "agents", agentName)
	os.MkdirAll(filepath.Join(agentDir, "inbox"), 0755)
	os.MkdirAll(filepath.Join(agentDir, "outbox"), 0755)
	os.MkdirAll(filepath.Join(agentDir, "processed"), 0755)
	os.MkdirAll(filepath.Join(agentDir, "workspace"), 0755)
	os.MkdirAll(filepath.Join(stateBase, "ledger"), 0755)
	os.MkdirAll(filepath.Join(stateBase, "logs", "audit"), 0755)
	agentHome := filepath.Join(homeBase, "a-"+agentName)
	os.MkdirAll(agentHome, 0755)
	os.WriteFile(filepath.Join(agentHome, "AGENTS.md"), []byte("# Test Agent\n"), 0644)
	dirs = Dirs{HomeBase: homeBase, StateBase: stateBase, ConfigBase: t.TempDir()}
	return
}

func TestRun_Success(t *testing.T) {
	agentName := "test-runner"
	stateBase, _, dirs := setupRunDirs(t, agentName)
	agentDir := filepath.Join(stateBase, "agents", agentName)

	os.WriteFile(filepath.Join(agentDir, "inbox", "001.task"), []byte("test task"), 0644)

	cfg := &config.Config{
		Agents: []config.AgentConfig{
			// "cat" echoes stdin to stdout — a trivial stand-in for a real runtime
			{Name: agentName, Runner: "cat"},
		},
	}
	if err := Run(agentName, cfg, dirs); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	entries, _ := os.ReadDir(filepath.Join(agentDir, "outbox"))
	if len(entries) == 0 {
		t.Error("expected response file in outbox after successful run")
	}
	if _, err := os.Stat(filepath.Join(agentDir, "processed", "001.task")); os.IsNotExist(err) {
		t.Error("expected task in processed dir after run")
	}
}

func TestRun_WithSkills(t *testing.T) {
	// Covers the skillsContent != "" branch in Run.
	agentName := "test-runner"
	stateBase, _, dirs := setupRunDirs(t, agentName)
	agentDir := filepath.Join(stateBase, "agents", agentName)

	// Add a skill to the workspace skills dir
	skillsDir := filepath.Join(agentDir, "workspace", "skills")
	os.MkdirAll(skillsDir, 0755)
	os.WriteFile(filepath.Join(skillsDir, "test-skill.md"), []byte("# Test skill content"), 0644)

	os.WriteFile(filepath.Join(agentDir, "inbox", "001.task"), []byte("test task"), 0644)

	cfg := &config.Config{Agents: []config.AgentConfig{{Name: agentName, Runner: "cat"}}}
	if err := Run(agentName, cfg, dirs); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
}

func TestRun_RuntimeError(t *testing.T) {
	// Covers the rt.Invoke error path where output is empty.
	agentName := "test-runner"
	stateBase, _, dirs := setupRunDirs(t, agentName)
	agentDir := filepath.Join(stateBase, "agents", agentName)
	os.WriteFile(filepath.Join(agentDir, "inbox", "001.task"), []byte("test task"), 0644)

	// "false" exits with code 1 and produces no output
	cfg := &config.Config{Agents: []config.AgentConfig{{Name: agentName, Runner: "false"}}}
	err := Run(agentName, cfg, dirs)
	if err == nil {
		t.Fatal("expected error when runtime fails with no output")
	}
	if !strings.Contains(err.Error(), "runtime returned no output") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_RouteOutputError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root (chmod 0555 won't prevent root writes)")
	}
	agentName := "test-runner"
	stateBase, _, dirs := setupRunDirs(t, agentName)
	agentDir := filepath.Join(stateBase, "agents", agentName)
	os.WriteFile(filepath.Join(agentDir, "inbox", "001.task"), []byte("test task"), 0644)

	// Make outbox unwritable so RouteOutput fails
	os.Chmod(filepath.Join(agentDir, "outbox"), 0555)
	defer os.Chmod(filepath.Join(agentDir, "outbox"), 0755)

	cfg := &config.Config{Agents: []config.AgentConfig{{Name: agentName, Runner: "cat"}}}
	err := Run(agentName, cfg, dirs)
	if err == nil {
		t.Fatal("expected error when route output fails")
	}
	if !strings.Contains(err.Error(), "routing output") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_EmptyInbox(t *testing.T) {
	agentName := "test-runner"
	stateBase := t.TempDir()
	homeBase := t.TempDir()

	os.MkdirAll(filepath.Join(stateBase, "agents", agentName, "inbox"), 0755)
	agentHome := filepath.Join(homeBase, "a-"+agentName)
	os.MkdirAll(agentHome, 0755)
	os.WriteFile(filepath.Join(agentHome, "AGENTS.md"), []byte("# Test Agent\n"), 0644)

	cfg := &config.Config{Agents: []config.AgentConfig{{Name: agentName, Runner: "cat"}}}
	dirs := Dirs{HomeBase: homeBase, StateBase: stateBase, ConfigBase: t.TempDir()}

	err := Run(agentName, cfg, dirs)
	if err == nil {
		t.Fatal("expected error for empty inbox")
	}
	if !strings.Contains(err.Error(), "no tasks in inbox") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestIsTrustedUID_WithGroupOverride(t *testing.T) {
	uid := uint32(os.Getuid())
	if uid == 0 {
		t.Skip("test must run as non-root")
	}

	u, err := user.Current()
	if err != nil {
		t.Fatalf("looking up current user: %v", err)
	}
	gids, err := u.GroupIds()
	if err != nil || len(gids) == 0 {
		t.Skip("cannot determine user groups")
	}
	g, err := user.LookupGroupId(gids[0])
	if err != nil {
		t.Skip("cannot look up group name")
	}

	old := TrustedGroupName
	TrustedGroupName = g.Name
	defer func() { TrustedGroupName = old }()

	if !isTrustedUID(uid) {
		t.Errorf("user in group %q should be trusted when TrustedGroupName=%q", g.Name, g.Name)
	}
}
