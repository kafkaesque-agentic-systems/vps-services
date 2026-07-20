package main

import (
	"strings"
	"testing"
)

// namesOf extracts the canonical names from a registrar slice so tests can
// assert on selection and ordering without comparing function values (which
// are not comparable in Go).
func namesOf(regs []skillRegistrar) []string {
	names := make([]string, 0, len(regs))
	for _, r := range regs {
		names = append(names, r.name)
	}
	return names
}

// TestSelectSkillsDefaultsToAll verifies the production default: an unset or
// blank MCP_SKILLS registers every skill, so existing deployments are
// unaffected by the introduction of the allowlist.
func TestSelectSkillsDefaultsToAll(t *testing.T) {
	for _, spec := range []string{"", "   ", "\t\n"} {
		selected, err := selectSkills(spec)
		if err != nil {
			t.Fatalf("selectSkills(%q): unexpected error: %v", spec, err)
		}
		if got, want := len(selected), len(allSkills); got != want {
			t.Errorf("selectSkills(%q): got %d skills, want all %d", spec, got, want)
		}
	}
}

// TestSelectSkillsAllowlist covers the accepted forms of a restricted surface:
// single and multiple names, case-insensitivity, surrounding whitespace, and
// duplicates. Selection must always come back in allSkills order regardless of
// the order the caller listed them.
func TestSelectSkillsAllowlist(t *testing.T) {
	tests := []struct {
		name string
		spec string
		want []string
	}{
		{"single", "deploy", []string{"deploy"}},
		{"multiple", "system,docker", []string{"system", "docker"}},
		{"uppercase", "DEPLOY", []string{"deploy"}},
		{"mixed case", "DePloY", []string{"deploy"}},
		{"padded whitespace", "  deploy , system  ", []string{"system", "deploy"}},
		{"reordered input yields canonical order", "database,system", []string{"system", "database"}},
		{"duplicates collapse", "deploy,deploy", []string{"deploy"}},
		{"trailing comma", "deploy,", []string{"deploy"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selected, err := selectSkills(tt.spec)
			if err != nil {
				t.Fatalf("selectSkills(%q): unexpected error: %v", tt.spec, err)
			}
			got := namesOf(selected)
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("selectSkills(%q) = %v, want %v", tt.spec, got, tt.want)
			}
		})
	}
}

// TestSelectSkillsDeployOnly pins the specific configuration the local
// developer instance runs. push_codebase must be the ONLY surface exposed:
// registering docker or database on a local instance would offer system_down
// and db_delete against production credentials.
func TestSelectSkillsDeployOnly(t *testing.T) {
	selected, err := selectSkills("deploy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(selected) != 1 || selected[0].name != "deploy" {
		t.Fatalf("MCP_SKILLS=deploy selected %v, want exactly [deploy]", namesOf(selected))
	}
	for _, r := range selected {
		switch r.name {
		case "docker", "database", "system", "snapshot":
			t.Errorf("deploy-only selection must not include %q", r.name)
		}
	}
}

// TestSelectSkillsRejectsUnknown verifies the fail-loud contract. A typo must
// be an error, never a silently reduced tool surface — the failure mode that
// made the 2026-07-19 DOCKER_GID incident expensive to diagnose.
func TestSelectSkillsRejectsUnknown(t *testing.T) {
	tests := []struct {
		name string
		spec string
	}{
		{"typo", "deply"},
		{"known plus unknown", "deploy,dockr"},
		{"entirely unknown", "weather"},
		{"separators only", ",,,"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selected, err := selectSkills(tt.spec)
			if err == nil {
				t.Fatalf("selectSkills(%q) = %v, want error", tt.spec, namesOf(selected))
			}
			if selected != nil {
				t.Errorf("selectSkills(%q) returned %v alongside an error; want nil",
					tt.spec, namesOf(selected))
			}
			// The message must be actionable: it has to list the valid names
			// so an operator can correct the value without reading the source.
			for _, valid := range skillNames() {
				if !strings.Contains(err.Error(), valid) {
					t.Errorf("error %q omits valid skill name %q", err.Error(), valid)
				}
			}
		})
	}
}

// TestSkillNamesMatchesAllSkills guards the invariant that skillNames() — used
// to build operator-facing error messages — stays in sync with the canonical
// registrar table as skills are added.
func TestSkillNamesMatchesAllSkills(t *testing.T) {
	got := skillNames()
	if len(got) != len(allSkills) {
		t.Fatalf("skillNames() returned %d names, want %d", len(got), len(allSkills))
	}
	for i, name := range got {
		if name != allSkills[i].name {
			t.Errorf("skillNames()[%d] = %q, want %q", i, name, allSkills[i].name)
		}
	}
}

// TestAllSkillsWellFormed verifies the registrar table itself: every entry
// needs a lowercase name (the allowlist lowercases input before matching, so an
// uppercase entry would be unreachable) and a non-nil registration function.
func TestAllSkillsWellFormed(t *testing.T) {
	seen := make(map[string]bool)
	for i, s := range allSkills {
		if s.name == "" {
			t.Errorf("allSkills[%d] has an empty name", i)
		}
		if s.name != strings.ToLower(s.name) {
			t.Errorf("allSkills[%d] name %q must be lowercase to be selectable", i, s.name)
		}
		if s.register == nil {
			t.Errorf("allSkills[%d] (%q) has a nil register func", i, s.name)
		}
		if seen[s.name] {
			t.Errorf("allSkills contains duplicate name %q", s.name)
		}
		seen[s.name] = true
	}
}
