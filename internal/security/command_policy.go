package security

import (
	"fmt"
	"strings"
)

type CommandDecision struct {
	Safe    bool
	Denied  bool
	Summary string
}

type CommandPolicy struct {
	SafePrefixes     [][]string
	CautiousPrefixes [][]string
	DeniedPrefixes   [][]string
}

func DefaultCommandPolicy() CommandPolicy {
	return CommandPolicy{
		SafePrefixes: [][]string{
			{"ls"},
			{"pwd"},
			{"whoami"},
			{"date"},
			{"uname"},
			{"file"},
			{"stat"},
			{"head"},
			{"tail"},
			{"wc"},
			{"mdfind"},
			{"grep"},
			{"du"},
			{"ps"},
			{"cat"},
		},
		CautiousPrefixes: [][]string{
			{"cp"},
			{"chmod"},
			{"chown"},
			{"mkdir"},
			{"touch"},
			{"open"},
		},
		DeniedPrefixes: [][]string{
			{"sh"},
			{"bash"},
			{"zsh"},
			{"fish"},
			{"rm"},
			{"rmdir"},
			{"unlink"},
			{"trash"},
			{"trash-put"},
			{"mv"},
			{"sudo"},
			{"launchctl"},
			{"osascript"},
			{"curl"},
			{"wget"},
			{"ssh"},
			{"scp"},
			{"nc"},
			{"dd"},
			{"diskutil"},
			{"python"},
			{"python3"},
			{"ruby"},
			{"perl"},
			{"node"},
			{"npm"},
			{"npx"},
			{"go"},
			{"make"},
			{"tee"},
		},
	}
}

func (p CommandPolicy) Check(args []string) CommandDecision {
	if len(args) == 0 {
		return CommandDecision{Safe: false, Denied: true, Summary: "empty command"}
	}

	switch strings.ToLower(args[0]) {
	case "find":
		return checkFind(args)
	case "git":
		return checkGit(args)
	case "defaults":
		return checkDefaults(args)
	case "kill", "pkill":
		return deny(args, "process control is denied by policy")
	}

	for _, prefix := range p.DeniedPrefixes {
		if hasPrefix(args, prefix) {
			return deny(args, "command denied by policy")
		}
	}
	for _, prefix := range p.SafePrefixes {
		if hasPrefix(args, prefix) {
			return safe(args, "safe read-only command")
		}
	}
	for _, prefix := range p.CautiousPrefixes {
		if hasPrefix(args, prefix) {
			return needsApproval(args, "command changes local state and requires explicit approval")
		}
	}
	return deny(args, "unknown command denied by allowlist policy")
}

func hasPrefix(full []string, prefix []string) bool {
	if len(full) < len(prefix) {
		return false
	}
	for i := range prefix {
		if full[i] != prefix[i] {
			return false
		}
	}
	return true
}

func safe(args []string, reason string) CommandDecision {
	return CommandDecision{
		Safe:    true,
		Denied:  false,
		Summary: fmt.Sprintf("%s: %s", reason, strings.Join(args, " ")),
	}
}

func needsApproval(args []string, reason string) CommandDecision {
	return CommandDecision{
		Safe:    false,
		Denied:  false,
		Summary: fmt.Sprintf("%s: %s", reason, strings.Join(args, " ")),
	}
}

func deny(args []string, reason string) CommandDecision {
	return CommandDecision{
		Safe:    false,
		Denied:  true,
		Summary: fmt.Sprintf("%s: %s", reason, strings.Join(args, " ")),
	}
}

func checkFind(args []string) CommandDecision {
	dangerous := map[string]struct{}{
		"-delete":  {},
		"-exec":    {},
		"-execdir": {},
		"-ok":      {},
		"-okdir":   {},
		"-fprint":  {},
		"-fprint0": {},
		"-fprintf": {},
		"-fls":     {},
	}
	for _, arg := range args[1:] {
		if _, ok := dangerous[strings.ToLower(arg)]; ok {
			return deny(args, "find command can delete or modify data and is denied")
		}
	}
	return safe(args, "safe read-only command")
}

func checkGit(args []string) CommandDecision {
	if len(args) < 2 {
		return deny(args, "git command denied unless it is explicitly read-only")
	}
	switch strings.ToLower(args[1]) {
	case "status", "diff", "log", "show", "rev-parse", "ls-files":
		return safe(args, "safe read-only git command")
	case "rm", "clean":
		return deny(args, "git deletion command is denied")
	default:
		return deny(args, "git command denied by high-security policy")
	}
}

func checkDefaults(args []string) CommandDecision {
	if len(args) < 2 {
		return deny(args, "defaults command denied unless it is a read operation")
	}
	switch strings.ToLower(args[1]) {
	case "read":
		return safe(args, "safe read-only command")
	case "delete":
		return deny(args, "defaults delete is denied")
	default:
		return needsApproval(args, "defaults command changes local state and requires explicit approval")
	}
}
