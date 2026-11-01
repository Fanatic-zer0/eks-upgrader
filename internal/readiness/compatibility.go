package readiness

import "fmt"

// karpenterMinVersionForK8s is a conservative, manually curated compatibility floor:
// the minimum Karpenter controller version known to support each Kubernetes minor
// version. This table must be kept up to date as new Kubernetes/Karpenter versions
// are released; unknown target versions fall back to an informational finding rather
// than a fabricated guess.
var karpenterMinVersionForK8s = map[string]string{
	"1.29": "0.37.0",
	"1.30": "1.0.0",
	"1.31": "1.0.1",
	"1.32": "1.1.0",
	"1.33": "1.3.0",
}

// CheckKarpenterCompatibility compares the installed Karpenter controller version
// against the minimum known-compatible version for targetK8sVersion.
func CheckKarpenterCompatibility(installedVersion, targetK8sVersion string) Finding {
	minVersion, known := karpenterMinVersionForK8s[targetK8sVersion]
	if !known {
		return Finding{
			Source: "compatibility", Category: "karpenter", Severity: SeverityWarning,
			Message:        fmt.Sprintf("no compatibility data available for Kubernetes %s; manually verify Karpenter %s supports it", targetK8sVersion, installedVersion),
			Recommendation: "check the Karpenter release notes for the target Kubernetes version before proceeding",
		}
	}

	if compareSemverLike(installedVersion, minVersion) < 0 {
		return Finding{
			Source: "compatibility", Category: "karpenter", Severity: SeverityBlocking,
			Message:        fmt.Sprintf("Karpenter %s is not compatible with Kubernetes %s (requires >= %s)", installedVersion, targetK8sVersion, minVersion),
			Recommendation: "upgrade the Karpenter controller and CRDs to a compatible version before upgrading node groups",
		}
	}
	return Finding{
		Source: "compatibility", Category: "karpenter", Severity: SeveritySafe,
		Message: fmt.Sprintf("Karpenter %s satisfies the minimum compatible version (%s) for Kubernetes %s", installedVersion, minVersion, targetK8sVersion),
	}
}

// CheckAddonCompatibility is intentionally conservative: EKS add-on compatibility
// matrices change frequently and are already enforced by EKS itself at update time, so
// this emits an informational reminder rather than a fabricated version table.
func CheckAddonCompatibility(addonName, addonVersion, targetK8sVersion string) Finding {
	return Finding{
		Source: "compatibility", Category: "addon", Resource: addonName, Severity: SeverityWarning,
		Message:        fmt.Sprintf("verify add-on %s version %s is listed as compatible with Kubernetes %s in the AWS EKS add-on compatibility documentation before upgrading", addonName, addonVersion, targetK8sVersion),
		Recommendation: "EKS itself will reject an incompatible --version at update time, but confirm proactively to avoid a paused run",
	}
}

// compareSemverLike compares two "MAJOR.MINOR.PATCH"-ish version strings, returning
// -1, 0, or 1. Non-numeric components compare as equal-weight zero.
func compareSemverLike(a, b string) int {
	ap := splitVersionParts(a)
	bp := splitVersionParts(b)
	for i := 0; i < len(ap) || i < len(bp); i++ {
		var av, bv int
		if i < len(ap) {
			av = ap[i]
		}
		if i < len(bp) {
			bv = bp[i]
		}
		if av != bv {
			if av < bv {
				return -1
			}
			return 1
		}
	}
	return 0
}

func splitVersionParts(v string) []int {
	var parts []int
	cur := 0
	has := false
	for _, r := range v {
		switch {
		case r >= '0' && r <= '9':
			cur = cur*10 + int(r-'0')
			has = true
		default:
			if has {
				parts = append(parts, cur)
			}
			cur = 0
			has = false
		}
	}
	if has {
		parts = append(parts, cur)
	}
	return parts
}
