package reconcile

import (
	"regexp"
	"strings"

	"eks-upgrader/internal/config"
)

// ApplyPolicy reconciles the differ's default classifications against the plan's
// stateReconciliation policy: ignoredFields always wins (forced IN_SYNC), then
// warnOnlyOn caps severity at WARNING, then blockOn forces BLOCKING. Fields matching
// none of the policy lists keep the differ's default classification.
func ApplyPolicy(findings []Finding, policy config.StateReconciliationSpec) []Finding {
	out := make([]Finding, len(findings))
	for i, f := range findings {
		out[i] = f
		switch {
		case matchesAnyPattern(f.Field, policy.IgnoredFields):
			out[i].Classification = InSync
			out[i].Message = "ignored by stateReconciliation.ignoredFields policy: " + f.Message
		case matchesAnyPattern(f.Field, policy.WarnOnlyOn):
			if out[i].Classification == Blocking || out[i].Classification == Unmanaged {
				out[i].Classification = Warning
			}
		case matchesAnyPattern(f.Field, policy.BlockOn):
			if out[i].Classification != Unmanaged {
				out[i].Classification = Blocking
			}
		}
	}
	return out
}

// matchesAnyPattern reports whether field matches any of patterns, where patterns may
// contain a `[*]` wildcard segment (e.g. "managedNodeGroups[*].kubernetesVersion").
func matchesAnyPattern(field string, patterns []string) bool {
	for _, p := range patterns {
		if patternToRegex(p).MatchString(field) {
			return true
		}
	}
	return false
}

func patternToRegex(pattern string) *regexp.Regexp {
	escaped := regexp.QuoteMeta(pattern)
	// QuoteMeta escapes [ and ] and * ; turn the escaped wildcard segment back into a
	// regex that matches any bracketed identifier, e.g. [system] or [general-purpose].
	wildcard := regexp.QuoteMeta("[*]")
	escaped = strings.ReplaceAll(escaped, wildcard, `\[[^\]]+\]`)
	return regexp.MustCompile("^" + escaped + "$")
}
