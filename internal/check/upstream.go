package check

import (
	"fmt"
	"sync"

	agonesv1 "agones.dev/agones/pkg/apis/agones/v1"
	"agones.dev/agones/pkg/cloudproduct/generic"
	"agones.dev/agones/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/Rohan-Muslekar/agones-check/internal/manifest"
	"github.com/Rohan-Muslekar/agones-check/internal/profile"
)

// Agones keeps feature gates in package-level state, so only one profile
// can be active at a time in a process.
var gateMu sync.Mutex

var apiHooks agonesv1.APIHooks = generic.New()

// Upstream runs Agones' own defaulting and validation for every object
// with the profile's gates switched on, the same way the admission webhook
// (Fleet, GameServer, FleetAutoscaler) and the allocation API
// (GameServerAllocation) do. It returns one finding per rejected object.
func Upstream(set *manifest.Set, p *profile.Profile) ([]Finding, error) {
	gateMu.Lock()
	defer gateMu.Unlock()
	if err := runtime.ParseFeatures(p.Query()); err != nil {
		return nil, fmt.Errorf("profile %s: %w", p.Name, err)
	}
	var out []Finding
	for _, o := range set.Objects {
		if o.Autoscaler != nil {
			out = append(out, nestedPolicies(o, p)...)
			out = append(out, scheduleNulls(o, p)...)
		}
		errs := validateOne(o)
		if len(errs) == 0 {
			continue
		}
		for _, e := range errs {
			f := objFinding(o, RuleUpstream, Error, e.ErrorBody())
			f.Field = e.Field
			f.Profiles = []string{p.Name}
			out = append(out, f)
		}
	}
	return out, nil
}

// validateOne mirrors the webhook handlers in pkg/gameservers,
// pkg/fleets, pkg/fleetautoscalers and pkg/gameserverallocations: apply
// defaults to a copy, then call Validate.
func validateOne(o *manifest.Object) field.ErrorList {
	switch {
	case o.Fleet != nil:
		f := o.Fleet.DeepCopy()
		f.ApplyDefaults()
		return f.Validate(apiHooks)
	case o.GameServer != nil:
		gs := o.GameServer.DeepCopy()
		gs.ApplyDefaults()
		return gs.Validate(apiHooks)
	case o.Autoscaler != nil:
		fas := o.Autoscaler.DeepCopy()
		fas.ApplyDefaults()
		return fas.Validate()
	case o.Allocation != nil:
		gsa := o.Allocation.DeepCopy()
		gsa.ApplyDefaults()
		return gsa.Validate()
	}
	return nil
}

// nestedPolicies validates the policy inside every Schedule policy.
// Agones' SchedulePolicy.ValidateSchedulePolicy checks the schedule window
// but never calls ValidatePolicy on .policy, so the webhook accepts, for
// example, a nested Buffer policy with maxReplicas below minReplicas.
// Must be called with the profile's gates active.
func nestedPolicies(o *manifest.Object, p *profile.Profile) []Finding {
	fas := o.Autoscaler.DeepCopy()
	fas.ApplyDefaults()
	var out []Finding
	for _, pr := range policies(&fas.Spec.Policy, "spec.policy") {
		if pr.policy.Schedule == nil {
			continue
		}
		inner := field.NewPath(pr.path + ".schedule.policy")
		for _, e := range pr.policy.Schedule.Policy.ValidatePolicy(inner) {
			f := objFinding(o, RuleNestedPolicy, Error, e.ErrorBody()+
				"; the Agones webhook does not validate policies nested in a Schedule, so the cluster accepts this and the autoscaler misbehaves while the schedule is active")
			f.Field = e.Field
			f.Profiles = []string{p.Name}
			out = append(out, f)
		}
	}
	return out
}

// scheduleNulls reports Schedule policies whose between.start or
// between.end is unset. Agones documents both as optional, but
// Between.Start and Between.End are metav1.Time values without omitempty:
// the mutating webhook re-encodes the defaulted object, an unset time
// becomes null, and the CRD schema ("must be of type string") rejects the
// request. Seen with kubectl apply --dry-run=server against Agones 1.61.0.
func scheduleNulls(o *manifest.Object, p *profile.Profile) []Finding {
	var out []Finding
	for _, pr := range policies(&o.Autoscaler.Spec.Policy, "spec.policy") {
		s := pr.policy.Schedule
		if s == nil {
			continue
		}
		for _, t := range []struct {
			name, hint string
			zero       bool
		}{
			{"start", `a time in the past, e.g. "2020-01-01T00:00:00Z", to start now`, s.Between.Start.IsZero()},
			{"end", `a far-future time, e.g. "2100-01-01T00:00:00Z", for no end`, s.Between.End.IsZero()},
		} {
			if !t.zero {
				continue
			}
			path := pr.path + ".schedule.between." + t.name
			f := objFinding(o, RuleScheduleNull, Error, fmt.Sprintf(
				"%s is not set; Agones' mutating webhook writes it as null and the API server rejects the FleetAutoscaler (%s must be of type string), although Agones documents it as optional; set it to %s",
				path, path, t.hint))
			f.Field = path
			f.Profiles = []string{p.Name}
			out = append(out, f)
		}
	}
	return out
}
