package bootstrap

import "fmt"

// ProvisionFromManifest generates shell commands from a Manifest.
// This covers groups, users, directories, files, ACLs, and systemd units.
func ProvisionFromManifest(m Manifest) []string {
	var cmds []string

	// Groups
	for _, g := range m.Groups {
		cmds = append(cmds, fmt.Sprintf("groupadd -f %s", g.Name))
	}

	// Files (pre-user, for things like env and signing key)
	for _, f := range m.Files {
		if f.Content != "" {
			cmds = append(cmds, fmt.Sprintf("cat > %s << 'EOF'\n%sEOF", f.Path, f.Content))
		}
		cmds = append(cmds, fmt.Sprintf("chmod %s %s 2>/dev/null || true", f.Mode, f.Path))
		cmds = append(cmds, fmt.Sprintf("chown %s:%s %s 2>/dev/null || true", f.Owner, f.Group, f.Path))
	}

	// Users
	for _, u := range m.Users {
		if len(u.Groups) == 0 {
			continue
		}
		groupList := u.Groups[0]
		for _, g := range u.Groups[1:] {
			groupList += "," + g
		}
		shell := u.Shell
		if shell == "" {
			shell = "/bin/bash"
		}
		cmds = append(cmds, fmt.Sprintf(
			"useradd -r -m -d %s -s %s -g %s -G %s %s || true",
			u.Home, shell, u.Groups[0], groupList, u.Name,
		))
		cmds = append(cmds, fmt.Sprintf("install -d -o %s -g %s -m 700 %s", u.Name, u.Groups[0], u.Home))
	}

	// Directories
	for _, d := range m.Directories {
		if d.Owner != "" && d.Owner != "root" {
			cmds = append(cmds, fmt.Sprintf("install -d -o %s -g %s -m %s %s", d.Owner, d.Group, d.Mode, d.Path))
		} else if d.Group != "" && d.Group != "root" {
			cmds = append(cmds, fmt.Sprintf("install -d -o %s -g %s -m %s %s", d.Owner, d.Group, d.Mode, d.Path))
		} else {
			cmds = append(cmds, fmt.Sprintf("install -d -m %s %s", d.Mode, d.Path))
		}
	}

	// ACLs
	for _, acl := range m.ACLs {
		flag := ""
		if acl.Default {
			flag = "-d "
		}
		if acl.User != "" {
			cmds = append(cmds, fmt.Sprintf("setfacl %s-m u:%s:%s %s", flag, acl.User, acl.Perms, acl.Path))
		} else if acl.Group != "" {
			cmds = append(cmds, fmt.Sprintf("setfacl %s-m g:%s:%s %s", flag, acl.Group, acl.Perms, acl.Path))
		}
	}

	// Systemd units
	for _, u := range m.Units {
		path := "/etc/systemd/system/" + u.Name
		cmds = append(cmds, fmt.Sprintf("cat > %s << 'UNITEOF'\n%sUNITEOF", path, u.Content))
	}

	// Setup commands (imperative steps)
	for _, sc := range m.SetupCommands {
		cmds = append(cmds, sc.Cmd)
	}

	return cmds
}
