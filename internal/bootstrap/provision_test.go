package bootstrap

import (
	"strings"
	"testing"
)

func TestProvisionFromManifest_EmptyManifest(t *testing.T) {
	cmds := ProvisionFromManifest(Manifest{})
	if len(cmds) != 0 {
		t.Errorf("expected 0 commands for empty manifest, got %d", len(cmds))
	}
}

func TestProvisionFromManifest_GroupOrdering(t *testing.T) {
	m := Manifest{
		Groups: []Group{{Name: "agents"}, {Name: "officers"}},
		Users:  []User{{Name: "a-test", Home: "/home/a-test", Groups: []string{"agents", "officers"}, Shell: "/bin/bash"}},
	}
	cmds := ProvisionFromManifest(m)

	// Groups must come before users
	groupIdx := -1
	userIdx := -1
	for i, c := range cmds {
		if strings.Contains(c, "groupadd") && groupIdx == -1 {
			groupIdx = i
		}
		if strings.Contains(c, "useradd") && userIdx == -1 {
			userIdx = i
		}
	}
	if groupIdx == -1 {
		t.Fatal("no groupadd command found")
	}
	if userIdx == -1 {
		t.Fatal("no useradd command found")
	}
	if groupIdx > userIdx {
		t.Error("groups must be created before users")
	}
}

func TestProvisionFromManifest_FileContent(t *testing.T) {
	m := Manifest{
		Files: []File{
			{Path: "/etc/conos/env", Mode: "600", Owner: "root", Group: "root", Content: "KEY=value\n"},
		},
	}
	cmds := ProvisionFromManifest(m)

	hasContent := false
	hasChmod := false
	hasChown := false
	for _, c := range cmds {
		if strings.Contains(c, "cat > /etc/conos/env") && strings.Contains(c, "KEY=value") {
			hasContent = true
		}
		if strings.Contains(c, "chmod 600 /etc/conos/env") {
			hasChmod = true
		}
		if strings.Contains(c, "chown root:root /etc/conos/env") {
			hasChown = true
		}
	}
	if !hasContent {
		t.Error("expected cat command for file with content")
	}
	if !hasChmod {
		t.Error("expected chmod for file")
	}
	if !hasChown {
		t.Error("expected chown for file")
	}
}

func TestProvisionFromManifest_FileNoContent(t *testing.T) {
	m := Manifest{
		Files: []File{
			{Path: "/etc/conos/signing.key", Mode: "600", Owner: "root", Group: "root"},
		},
	}
	cmds := ProvisionFromManifest(m)

	for _, c := range cmds {
		if strings.Contains(c, "cat >") && strings.Contains(c, "signing.key") {
			t.Error("should not write content when Content is empty")
		}
	}
}

func TestProvisionFromManifest_UserDefaultShell(t *testing.T) {
	m := Manifest{
		Users: []User{{Name: "a-test", Home: "/home/a-test", Groups: []string{"agents"}}},
	}
	cmds := ProvisionFromManifest(m)

	for _, c := range cmds {
		if strings.Contains(c, "useradd") && strings.Contains(c, "/bin/bash") {
			return
		}
	}
	t.Error("expected default shell /bin/bash when Shell is empty")
}

func TestProvisionFromManifest_UserCustomShell(t *testing.T) {
	m := Manifest{
		Users: []User{{Name: "a-test", Home: "/home/a-test", Groups: []string{"agents"}, Shell: "/usr/sbin/nologin"}},
	}
	cmds := ProvisionFromManifest(m)

	for _, c := range cmds {
		if strings.Contains(c, "useradd") && strings.Contains(c, "/usr/sbin/nologin") {
			return
		}
	}
	t.Error("expected custom shell in useradd command")
}

func TestProvisionFromManifest_UserSkippedWithNoGroups(t *testing.T) {
	m := Manifest{
		Users: []User{{Name: "a-orphan", Home: "/home/a-orphan"}},
	}
	cmds := ProvisionFromManifest(m)

	for _, c := range cmds {
		if strings.Contains(c, "useradd") && strings.Contains(c, "a-orphan") {
			t.Error("user with no groups should be skipped")
		}
	}
}

func TestProvisionFromManifest_DirRootOwner(t *testing.T) {
	m := Manifest{
		Directories: []Directory{
			{Path: "/srv/conos", Mode: "755", Owner: "root", Group: "root"},
		},
	}
	cmds := ProvisionFromManifest(m)

	for _, c := range cmds {
		if strings.Contains(c, "install -d -m 755 /srv/conos") {
			return // root-owned dir uses simple install -d -m
		}
	}
	t.Error("root-owned dir should use simple install -d -m (no -o/-g)")
}

func TestProvisionFromManifest_DirNonRootOwner(t *testing.T) {
	m := Manifest{
		Directories: []Directory{
			{Path: "/srv/conos/agents/test/inbox", Mode: "700", Owner: "a-test", Group: "agents"},
		},
	}
	cmds := ProvisionFromManifest(m)

	for _, c := range cmds {
		if strings.Contains(c, "install -d -o a-test -g agents -m 700") {
			return
		}
	}
	t.Error("non-root dir should use install -d -o -g -m")
}

func TestProvisionFromManifest_ACLUser(t *testing.T) {
	m := Manifest{
		ACLs: []ACL{
			{Path: "/srv/conos/agents/sysadmin/inbox", User: "a-concierge", Perms: "rwx"},
		},
	}
	cmds := ProvisionFromManifest(m)

	for _, c := range cmds {
		if strings.Contains(c, "setfacl") && strings.Contains(c, "u:a-concierge:rwx") {
			return
		}
	}
	t.Error("expected setfacl -m u:user:perms for user ACL")
}

func TestProvisionFromManifest_ACLGroup(t *testing.T) {
	m := Manifest{
		ACLs: []ACL{
			{Path: "/srv/conos/logs/audit", Group: "agents", Perms: "rwx"},
		},
	}
	cmds := ProvisionFromManifest(m)

	for _, c := range cmds {
		if strings.Contains(c, "setfacl") && strings.Contains(c, "g:agents:rwx") {
			return
		}
	}
	t.Error("expected setfacl -m g:group:perms for group ACL")
}

func TestProvisionFromManifest_ACLDefault(t *testing.T) {
	m := Manifest{
		ACLs: []ACL{
			{Path: "/srv/conos/logs/audit", Group: "agents", Perms: "rw", Default: true},
		},
	}
	cmds := ProvisionFromManifest(m)

	for _, c := range cmds {
		if strings.Contains(c, "setfacl -d") && strings.Contains(c, "g:agents:rw") {
			return
		}
	}
	t.Error("expected setfacl -d for default ACL")
}

func TestProvisionFromManifest_SystemdUnits(t *testing.T) {
	m := Manifest{
		Units: []SystemdUnit{
			{Name: "conos-test.service", Content: "[Service]\nType=oneshot\n"},
		},
	}
	cmds := ProvisionFromManifest(m)

	for _, c := range cmds {
		if strings.Contains(c, "cat > /etc/systemd/system/conos-test.service") {
			return
		}
	}
	t.Error("expected systemd unit write command")
}

func TestProvisionFromManifest_DirRootOwnerNonRootGroup(t *testing.T) {
	m := Manifest{
		Directories: []Directory{
			{Path: "/srv/conos/inbox", Mode: "770", Owner: "root", Group: "agents"},
		},
	}
	cmds := ProvisionFromManifest(m)

	for _, c := range cmds {
		if strings.Contains(c, "install -d -o root -g agents -m 770") {
			return
		}
	}
	t.Error("root-owned dir with non-root group should use install -d -o -g -m")
}

func TestProvisionFromManifest_ACLNoUserNoGroup(t *testing.T) {
	m := Manifest{
		ACLs: []ACL{
			{Path: "/srv/test", Perms: "rwx"}, // neither user nor group
		},
	}
	cmds := ProvisionFromManifest(m)

	for _, c := range cmds {
		if strings.Contains(c, "setfacl") {
			t.Error("ACL with neither user nor group should not generate a setfacl command")
		}
	}
}

func TestProvisionFromManifest_MultipleSetupCommands(t *testing.T) {
	m := Manifest{
		SetupCommands: []SetupCommand{
			{Description: "first", Cmd: "echo first"},
			{Description: "second", Cmd: "echo second"},
			{Description: "third", Cmd: "echo third"},
		},
	}
	cmds := ProvisionFromManifest(m)

	if len(cmds) != 3 {
		t.Errorf("expected 3 commands, got %d", len(cmds))
	}
	// Verify order preserved
	firstIdx := -1
	thirdIdx := -1
	for i, c := range cmds {
		if c == "echo first" {
			firstIdx = i
		}
		if c == "echo third" {
			thirdIdx = i
		}
	}
	if firstIdx > thirdIdx {
		t.Error("setup commands must preserve order")
	}
}

func TestProvisionFromManifest_Ordering(t *testing.T) {
	// Full manifest: verify groups → files → users → dirs → ACLs → units → setup
	m := Manifest{
		Groups:        []Group{{Name: "agents"}},
		Files:         []File{{Path: "/etc/conos/env", Mode: "600", Owner: "root", Group: "root", Content: "X=1\n"}},
		Users:         []User{{Name: "a-test", Home: "/home/a-test", Groups: []string{"agents"}}},
		Directories:   []Directory{{Path: "/srv/test", Mode: "755", Owner: "root", Group: "root"}},
		ACLs:          []ACL{{Path: "/srv/test", Group: "agents", Perms: "rwx"}},
		Units:         []SystemdUnit{{Name: "test.service", Content: "[Service]\n"}},
		SetupCommands: []SetupCommand{{Description: "final", Cmd: "echo done"}},
	}
	cmds := ProvisionFromManifest(m)

	phases := map[string]int{
		"groupadd": -1,
		"cat > /etc/conos/env": -1,
		"useradd":  -1,
		"install -d": -1,
		"setfacl":  -1,
		"cat > /etc/systemd": -1,
		"echo done": -1,
	}
	for i, c := range cmds {
		for key := range phases {
			if strings.Contains(c, key) && phases[key] == -1 {
				phases[key] = i
			}
		}
	}

	if phases["groupadd"] > phases["useradd"] {
		t.Error("groups must come before users")
	}
	if phases["useradd"] > phases["install -d"] {
		t.Error("users must come before directories")
	}
	if phases["install -d"] > phases["setfacl"] {
		t.Error("directories must come before ACLs")
	}
	if phases["setfacl"] > phases["cat > /etc/systemd"] {
		t.Error("ACLs must come before units")
	}
	if phases["cat > /etc/systemd"] > phases["echo done"] {
		t.Error("units must come before setup commands")
	}
}
