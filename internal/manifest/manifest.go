// Package manifest loads Kubernetes YAML files and decodes the Agones
// resources in them into the upstream Agones Go types.
package manifest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	agonesv1 "agones.dev/agones/pkg/apis/agones/v1"
	allocationv1 "agones.dev/agones/pkg/apis/allocation/v1"
	autoscalingv1 "agones.dev/agones/pkg/apis/autoscaling/v1"
	kjson "sigs.k8s.io/json"
	"sigs.k8s.io/yaml"
)

// DefaultNamespace is used for objects that do not set metadata.namespace,
// matching what `kubectl apply` does without -n.
const DefaultNamespace = "default"

// Kind names of the Agones resources the checker understands.
const (
	KindFleet           = "Fleet"
	KindGameServer      = "GameServer"
	KindFleetAutoscaler = "FleetAutoscaler"
	KindAllocation      = "GameServerAllocation"
)

// Source points at the document an object came from.
type Source struct {
	File string `json:"file"`
	Doc  int    `json:"doc"`  // 0-based index of the YAML document in the file
	Line int    `json:"line"` // 1-based line where the document starts
}

func (s Source) String() string { return fmt.Sprintf("%s:%d", s.File, s.Line) }

// Object is one decoded Agones resource.
type Object struct {
	Kind      string
	Name      string
	Namespace string
	Source    Source
	Raw       []byte // JSON form of the document

	Fleet      *agonesv1.Fleet
	GameServer *agonesv1.GameServer
	Autoscaler *autoscalingv1.FleetAutoscaler
	Allocation *allocationv1.GameServerAllocation

	// DecodeErr is set when the document is an Agones kind but does not
	// decode into the typed struct (unknown field, wrong type). The API
	// server rejects such objects under kubectl's default strict field
	// validation.
	DecodeErr error
}

// ID is kind/namespace/name, unique per object in a cluster.
func (o *Object) ID() string { return o.Kind + "/" + o.Namespace + "/" + o.Name }

// Set is every object loaded from the inputs.
type Set struct {
	Objects []*Object
	// Skipped counts documents that are valid YAML but not an Agones kind
	// (Deployments, Services, RBAC...).
	Skipped int
	// Files is the number of YAML files read.
	Files int
}

// ByKind returns the objects of one kind, in load order.
func (s *Set) ByKind(kind string) []*Object {
	var out []*Object
	for _, o := range s.Objects {
		if o.Kind == kind {
			out = append(out, o)
		}
	}
	return out
}

// Load reads every .yaml/.yml file under the given paths (files or
// directories, walked recursively). "-" reads standard input.
func Load(paths ...string) (*Set, error) {
	set := &Set{}
	for _, p := range paths {
		if p == "-" {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return nil, err
			}
			if err := set.addFile("<stdin>", data); err != nil {
				return nil, err
			}
			continue
		}
		files, err := yamlFiles(p)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				return nil, err
			}
			if err := set.addFile(f, data); err != nil {
				return nil, err
			}
		}
	}
	return set, nil
}

// LoadBytes decodes one in-memory file. Used by tests and the benchmark.
func LoadBytes(name string, data []byte) (*Set, error) {
	set := &Set{}
	return set, set.addFile(name, data)
}

func yamlFiles(root string) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return []string{root}, nil
	}
	var files []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !d.IsDir() && (ext == ".yaml" || ext == ".yml") {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

func (s *Set) addFile(name string, data []byte) error {
	s.Files++
	for doc, d := range splitDocs(data) {
		obj, err := decode(d.body)
		if err != nil {
			return fmt.Errorf("%s:%d: %w", name, d.line, err)
		}
		if obj == nil {
			s.Skipped++
			continue
		}
		obj.Source = Source{File: name, Doc: doc, Line: d.line}
		s.Objects = append(s.Objects, obj)
	}
	return nil
}

type rawDoc struct {
	body []byte
	line int // first line with content
}

// splitDocs splits a YAML stream on "---" lines and drops documents that
// hold only comments or whitespace.
func splitDocs(data []byte) []rawDoc {
	var out []rawDoc
	var cur bytes.Buffer
	first := 0
	flush := func() {
		if first > 0 {
			out = append(out, rawDoc{body: append([]byte(nil), cur.Bytes()...), line: first})
		}
		cur.Reset()
		first = 0
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if line == "---" || strings.HasPrefix(line, "--- ") || line == "..." {
			flush()
			continue
		}
		t := strings.TrimSpace(line)
		if first == 0 && t != "" && !strings.HasPrefix(t, "#") {
			first = n
		}
		cur.WriteString(line)
		cur.WriteByte('\n')
	}
	flush()
	return out
}

type header struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name         string `json:"name"`
		GenerateName string `json:"generateName"`
		Namespace    string `json:"namespace"`
	} `json:"metadata"`
}

// decode returns nil, nil for documents that are not Agones resources.
func decode(chunk []byte) (*Object, error) {
	js, err := yaml.YAMLToJSON(chunk)
	if err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	// Duplicate keys are lost in the JSON form; the API server rejects them
	// under strict field validation, so remember the error.
	_, dupErr := yaml.YAMLToJSONStrict(chunk)
	var h header
	if err := json.Unmarshal(js, &h); err != nil {
		return nil, nil // a list or scalar document: not a resource
	}
	group := h.APIVersion
	if i := strings.Index(group, "/"); i >= 0 {
		group = group[:i]
	}
	o := &Object{Kind: h.Kind, Name: h.Metadata.Name, Namespace: h.Metadata.Namespace, Raw: js}
	if o.Name == "" {
		o.Name = h.Metadata.GenerateName
	}
	if o.Namespace == "" {
		o.Namespace = DefaultNamespace
	}
	switch {
	case group == "agones.dev" && h.Kind == KindFleet:
		o.Fleet = &agonesv1.Fleet{}
		o.DecodeErr = strictUnmarshal(js, o.Fleet)
	case group == "agones.dev" && h.Kind == KindGameServer:
		o.GameServer = &agonesv1.GameServer{}
		o.DecodeErr = strictUnmarshal(js, o.GameServer)
	case group == "autoscaling.agones.dev" && h.Kind == KindFleetAutoscaler:
		o.Autoscaler = &autoscalingv1.FleetAutoscaler{}
		o.DecodeErr = strictUnmarshal(js, o.Autoscaler)
	case group == "allocation.agones.dev" && h.Kind == KindAllocation:
		o.Allocation = &allocationv1.GameServerAllocation{}
		o.DecodeErr = strictUnmarshal(js, o.Allocation)
	default:
		return nil, nil
	}
	if o.DecodeErr == nil && dupErr != nil {
		o.DecodeErr = dupErr
	}
	applyCRDDefaults(o)
	return o, nil
}

// CRDDefaultCapacity is the default the Agones CRD schema gives a counter
// or list capacity that is left out or null
// (install/helm/agones/templates/crds/_gameserverspecschema.yaml).
const CRDDefaultCapacity = 1000

// applyCRDDefaults reproduces the API server's structural-schema
// defaulting for counter and list capacities. The Go types cannot tell a
// missing capacity from 0, so the raw JSON is consulted.
func applyCRDDefaults(o *Object) {
	var raw map[string]any
	if json.Unmarshal(o.Raw, &raw) != nil {
		return
	}
	spec, _ := raw["spec"].(map[string]any)
	var gsSpec map[string]any
	var counters map[string]agonesv1.CounterStatus
	var lists map[string]agonesv1.ListStatus
	switch {
	case o.Fleet != nil:
		tmpl, _ := spec["template"].(map[string]any)
		gsSpec, _ = tmpl["spec"].(map[string]any)
		counters, lists = o.Fleet.Spec.Template.Spec.Counters, o.Fleet.Spec.Template.Spec.Lists
	case o.GameServer != nil:
		gsSpec = spec
		counters, lists = o.GameServer.Spec.Counters, o.GameServer.Spec.Lists
	default:
		return
	}
	missing := func(section, key string) bool {
		m, _ := gsSpec[section].(map[string]any)
		entry, _ := m[key].(map[string]any)
		v, ok := entry["capacity"]
		return !ok || v == nil
	}
	for k, c := range counters {
		if missing("counters", k) {
			c.Capacity = CRDDefaultCapacity
			counters[k] = c
		}
	}
	for k, l := range lists {
		if missing("lists", k) {
			l.Capacity = CRDDefaultCapacity
			lists[k] = l
		}
	}
}

// strictUnmarshal decodes the way the API server does under kubectl's
// default strict field validation: field names are case-sensitive, and
// unknown or duplicate fields are errors.
func strictUnmarshal(js []byte, into any) error {
	strict, err := kjson.UnmarshalStrict(js, into, kjson.DisallowDuplicateFields, kjson.DisallowUnknownFields)
	if err != nil {
		return err
	}
	if len(strict) > 0 {
		msgs := make([]string, len(strict))
		for i, e := range strict {
			msgs[i] = e.Error()
		}
		return errors.New(strings.Join(msgs, "; "))
	}
	return nil
}
