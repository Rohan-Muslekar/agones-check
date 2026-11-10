// Package profile describes a target environment: an Agones version plus
// the feature-gate settings of the cluster.
package profile

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/mod/semver"
	"sigs.k8s.io/yaml"
)

// Profile is one deployment environment.
type Profile struct {
	Name          string          `json:"name"`
	AgonesVersion string          `json:"agonesVersion"`
	FeatureGates  map[string]bool `json:"featureGates,omitempty"`
	// Source is where the profile came from (file path or "builtin").
	Source string `json:"-"`
}

// CheckerVersion is the Agones version whose validation code is linked in.
const CheckerVersion = "1.61.0"

//go:embed versionmap.json
var versionMapJSON []byte

type stageSet struct {
	Alpha []string `json:"alphaGates"`
	Beta  []string `json:"betaGates"`
}

var versionMap map[string]stageSet

// knownGates are every gate that Agones 1.61 understands, with its stage
// there. Gates not in the version map (dev/example) are off by default.
var knownGates = []string{
	"DisableResyncOnSDKServer", "AutopilotPassthroughPort", "PortPolicyNone", "PortRanges", "RollingUpdateFix",
	"CountsAndLists", "GKEAutopilotExtendedDurationPods", "ScheduledAutoscaler", "SidecarContainers", "FleetAutoscaleRequestMetaData",
	"PlayerAllocationFilter", "PlayerTracking", "WasmAutoscaler",
	"ProcessorAllocator", "Example",
}

func init() {
	if err := json.Unmarshal(versionMapJSON, &versionMap); err != nil {
		panic(err)
	}
}

// Versions lists the Agones versions the profile model knows about.
func Versions() []string {
	var vs []string
	for v := range versionMap {
		if v != "Dev" {
			vs = append(vs, v)
		}
	}
	sort.Slice(vs, func(i, j int) bool { return semver.Compare("v"+vs[i], "v"+vs[j]) < 0 })
	return vs
}

func normalize(v string) string {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if v == "" {
		return CheckerVersion
	}
	if strings.Count(v, ".") == 1 {
		v += ".0"
	}
	return v
}

// GateStage is the stage of a gate in a given Agones version.
type GateStage string

const (
	StageMissing GateStage = "missing" // the gate does not exist yet in that version
	StageAlpha   GateStage = "alpha"
	StageBeta    GateStage = "beta"
	StageStable  GateStage = "stable" // the gate existed before and is now always on
	StageDev     GateStage = "dev"
)

// Stage returns the stage of gate in Agones version v.
func Stage(v, gate string) GateStage {
	v = normalize(v)
	set, ok := versionMap[v]
	if !ok {
		return StageDev
	}
	if contains(set.Alpha, gate) {
		return StageAlpha
	}
	if contains(set.Beta, gate) {
		return StageBeta
	}
	// Did the gate exist in an earlier version? Then it graduated.
	for _, old := range Versions() {
		if semver.Compare("v"+old, "v"+v) >= 0 {
			break
		}
		s := versionMap[old]
		if contains(s.Alpha, gate) || contains(s.Beta, gate) {
			return StageStable
		}
	}
	if gate == "ProcessorAllocator" || gate == "Example" {
		return StageDev
	}
	return StageMissing
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// Validate reports problems with the profile itself.
func (p *Profile) Validate() error {
	v := normalize(p.AgonesVersion)
	if _, ok := versionMap[v]; !ok {
		return fmt.Errorf("profile %q: unknown Agones version %q (known: %s)", p.Name, p.AgonesVersion, strings.Join(Versions(), ", "))
	}
	for g, on := range p.FeatureGates {
		if !contains(knownGates, g) {
			return fmt.Errorf("profile %q: unknown feature gate %q", p.Name, g)
		}
		st := Stage(v, g)
		if on && st == StageMissing {
			return fmt.Errorf("profile %q: feature gate %q does not exist in Agones %s", p.Name, g, v)
		}
		if !on && st == StageStable {
			return fmt.Errorf("profile %q: feature gate %q is stable in Agones %s and cannot be turned off", p.Name, g, v)
		}
	}
	return nil
}

// Effective returns the full gate map for this profile: version defaults
// with the profile's overrides applied.
func (p *Profile) Effective() map[string]bool {
	v := normalize(p.AgonesVersion)
	out := map[string]bool{}
	for _, g := range knownGates {
		switch Stage(v, g) {
		case StageBeta, StageStable:
			out[g] = true
		default:
			out[g] = false
		}
	}
	for g, on := range p.FeatureGates {
		out[g] = on
	}
	return out
}

// Query encodes the effective gates in the format Agones'
// runtime.ParseFeatures accepts ("A=true&B=false").
func (p *Profile) Query() string {
	vals := url.Values{}
	for g, on := range p.Effective() {
		vals.Set(g, strconv.FormatBool(on))
	}
	return vals.Encode()
}

// Load reads a profile YAML file. The file may hold a single profile, or
// a Helm values file (agones.featureGates: "A=true&B=false").
func Load(path string) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	p, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if p.Name == "" {
		p.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	p.Source = path
	return p, p.Validate()
}

// Parse decodes a profile document.
func Parse(data []byte) (*Profile, error) {
	var raw struct {
		Profile
		Agones struct {
			FeatureGates string `json:"featureGates"`
			Image        struct {
				Tag string `json:"tag"`
			} `json:"image"`
		} `json:"agones"`
	}
	if err := yaml.UnmarshalStrict(data, &raw); err != nil {
		// Helm values files carry many unrelated keys.
		if err2 := yaml.Unmarshal(data, &raw); err2 != nil {
			return nil, err2
		}
	}
	p := raw.Profile
	if raw.Agones.FeatureGates != "" {
		gates, err := ParseGateString(raw.Agones.FeatureGates)
		if err != nil {
			return nil, err
		}
		if p.FeatureGates == nil {
			p.FeatureGates = map[string]bool{}
		}
		for k, v := range gates {
			p.FeatureGates[k] = v
		}
	}
	if p.AgonesVersion == "" {
		p.AgonesVersion = raw.Agones.Image.Tag
	}
	if p.AgonesVersion == "" {
		p.AgonesVersion = CheckerVersion
	}
	return &p, nil
}

// ParseGateString parses the Helm/flag format "A=true&B=false".
func ParseGateString(s string) (map[string]bool, error) {
	vals, err := url.ParseQuery(s)
	if err != nil {
		return nil, fmt.Errorf("feature gates %q: %w", s, err)
	}
	out := map[string]bool{}
	for k := range vals {
		b, err := strconv.ParseBool(vals.Get(k))
		if err != nil {
			return nil, fmt.Errorf("feature gate %s: %w", k, err)
		}
		out[k] = b
	}
	return out, nil
}

// Builtin returns the named built-in profiles.
//
//	prod      Agones 1.61 defaults (beta on, alpha off)
//	dev       Agones 1.61 with every alpha gate on
//	beta-off  Agones 1.61 with the beta gates switched off
//	1.NN      the defaults of that Agones version, e.g. "1.54"
func Builtin(name string) (*Profile, error) {
	p := &Profile{Name: name, AgonesVersion: CheckerVersion, Source: "builtin"}
	switch name {
	case "prod":
	case "dev":
		p.FeatureGates = map[string]bool{"PlayerAllocationFilter": true, "PlayerTracking": true, "WasmAutoscaler": true}
	case "beta-off":
		p.FeatureGates = map[string]bool{
			"CountsAndLists": false, "GKEAutopilotExtendedDurationPods": false, "ScheduledAutoscaler": false,
			"SidecarContainers": false, "FleetAutoscaleRequestMetaData": false,
		}
	default:
		if _, ok := versionMap[normalize(name)]; !ok {
			return nil, fmt.Errorf("unknown profile %q (built-ins: prod, dev, beta-off, or an Agones version such as 1.54)", name)
		}
		p.AgonesVersion = normalize(name)
	}
	return p, p.Validate()
}

// Resolve turns a CLI argument into a profile: an existing file path is
// loaded, anything else is looked up as a built-in.
func Resolve(arg string) (*Profile, error) {
	if _, err := os.Stat(arg); err == nil {
		return Load(arg)
	}
	return Builtin(arg)
}
