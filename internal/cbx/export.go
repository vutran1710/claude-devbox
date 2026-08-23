package cbx

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Export writes some part of this box's configuration to Out.
//
// This is the mirror of what the setup tool does: setuptool pushes a laptop's
// config to a box; export reads out what is on the box, from the box. That is
// what lets the master session answer "what skills do I have" and "what have I
// been working on" without anyone opening an SSH connection, and it is the
// backup path — exporting rules and the database is enough to rebuild a box's
// identity elsewhere.
func (a *App) Export(what string) error {
	switch what {
	case "skills":
		return a.exportSkills()
	case "rules":
		return a.exportRules()
	case "db":
		return a.exportDB()
	default:
		return fmt.Errorf("unknown export %q (skills, rules, db)", what)
	}
}

func claudeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/root/.claude"
	}
	return filepath.Join(home, ".claude")
}

// exportSkills lists the skills installed here, with the first line of each
// description so the output is useful on its own rather than a bare inventory.
func (a *App) exportSkills() error {
	dir := filepath.Join(claudeDir(), "skills")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil // no skills is a valid answer, not a failure
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", dir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(a.Out, "%s\t%s\n", n, firstDescription(filepath.Join(dir, n)))
	}
	return nil
}

// firstDescription pulls the description out of a skill's frontmatter.
func firstDescription(skillDir string) string {
	for _, name := range []string{"SKILL.md", "skill.md"} {
		data, err := os.ReadFile(filepath.Join(skillDir, name))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "description:") {
				return strings.TrimSpace(strings.TrimPrefix(line, "description:"))
			}
		}
	}
	return ""
}

// exportRules concatenates everything that shapes how sessions behave, each
// under a header naming where it came from, so the output is restorable by
// hand and diffable between boxes.
func (a *App) exportRules() error {
	home, _ := os.UserHomeDir()
	for _, p := range []string{
		filepath.Join(claudeDir(), "CLAUDE.md"),
		filepath.Join(claudeDir(), "settings.json"),
		filepath.Join(home, "CLAUDE.md"),
		filepath.Join(a.Root, "CLAUDE.md"),
	} {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		fmt.Fprintf(a.Out, "===== %s =====\n%s\n", p, strings.TrimRight(string(data), "\n"))
	}
	return nil
}

// exportDB writes the session table as tab-separated rows. Not SQL: the point
// is something a person or an agent can read and a script can cut, and the
// schema is small enough that rebuilding from this is trivial.
func (a *App) exportDB() error {
	sessions, err := a.Store.List()
	if err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "name\tdir\trepo\turl\tcreated\n")
	for _, s := range sessions {
		fmt.Fprintf(a.Out, "%s\t%s\t%s\t%s\t%s\n",
			s.Name, s.Dir, s.Repo, s.RCURL, s.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"))
	}
	return nil
}
