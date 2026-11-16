#!/usr/bin/env python3
"""Run kubeconform over every mutant as an independent schema-only baseline.

Needs kubeconform on PATH (or KUBECONFORM=/path), kubectl pointed at a cluster
running Agones (the CRD schemas are taken from it), and bench/mutants from
`make bench`. Writes bench/results/kubeconform.md and kubeconform.tsv.

The CRDs' openAPIV3Schema is converted to JSON Schema with
additionalProperties: false on every object that lists its properties, which
is what `kubeconform -strict` expects. GameServerAllocation is not a CRD (it
is served by the Agones extension API server), so kubeconform has no schema
for it and skips it. Null fields are dropped first, as the API server drops
them before validating a CRD object; the mutant files carry `capacity: null`
where the bench re-serialised a List without a capacity.
"""
import collections
import datetime
import json
import os
import subprocess
import sys
import tempfile

import yaml
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
RESULTS = ROOT / "bench" / "results"
KC = os.environ.get("KUBECONFORM", "kubeconform")


def strict(node):
    if isinstance(node, dict):
        if node.get("x-kubernetes-preserve-unknown-fields"):
            return {k: strict(v) for k, v in node.items() if k != "properties"} | (
                {"properties": strict(node["properties"])} if "properties" in node else {})
        out = {k: strict(v) for k, v in node.items()}
        if "properties" in out and "additionalProperties" not in out:
            out["additionalProperties"] = False
        return out
    if isinstance(node, list):
        return [strict(v) for v in node]
    return node


def write_schemas(dest):
    crds = json.loads(subprocess.check_output(["kubectl", "get", "crd", "-o", "json"]))
    for crd in crds["items"]:
        spec = crd["spec"]
        if not spec["group"].endswith("agones.dev"):
            continue
        for v in spec["versions"]:
            schema = strict(v["schema"]["openAPIV3Schema"])
            # apiVersion, kind and metadata are checked by the API server, not the CRD schema.
            schema.setdefault("properties", {}).update(
                {"apiVersion": {"type": "string"}, "kind": {"type": "string"}, "metadata": {"type": "object"}})
            out = dest / spec["group"] / f"{spec['names']['kind'].lower()}_{v['name']}.json"
            out.parent.mkdir(parents=True, exist_ok=True)
            out.write_text(json.dumps(schema))
    image = subprocess.check_output(
        ["kubectl", "get", "deploy", "-n", "agones-system", "agones-controller",
         "-o", "jsonpath={.spec.template.spec.containers[0].image}"], text=True)
    return image.rsplit(":", 1)[-1]


def drop_nulls(node):
    if isinstance(node, dict):
        return {k: drop_nulls(v) for k, v in node.items() if v is not None}
    if isinstance(node, list):
        return [drop_nulls(v) for v in node]
    return node


def rejected(path, schemas):
    docs = [drop_nulls(d) for d in yaml.safe_load_all(path.read_text()) if d]
    clean = schemas.parent / "input.yaml"
    clean.write_text(yaml.safe_dump_all(docs))
    run = subprocess.run(
        [KC, "-strict", "-ignore-missing-schemas", "-output", "json",
         "-schema-location", str(schemas) + "/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json",
         str(clean)],
        capture_output=True, text=True)
    res = json.loads(run.stdout or "{}").get("resources", [])
    bad = [r for r in res if r.get("status") in ("statusInvalid", "statusError")]
    return bool(bad), "; ".join(f"{r.get('kind')}/{r.get('name')}: {r.get('msg', '')[:120]}" for r in bad)


def main():
    version = subprocess.check_output([KC, "-v"], text=True).strip()
    if version == "development":  # built with go install; read the module version instead
        info = subprocess.check_output(["go", "version", "-m", subprocess.check_output(
            ["sh", "-c", f"command -v {KC}"], text=True).strip()], text=True)
        version = next(l.split()[2] for l in info.splitlines() if l.split()[:1] == ["mod"])
    mutants = json.loads((RESULTS / "mutants.json").read_text())
    with tempfile.TemporaryDirectory() as tmp:
        schemas = Path(tmp) / "schemas"
        agones = write_schemas(schemas)
        rows = []
        for m in mutants:
            hit, msg = rejected(ROOT / m["file"], schemas)
            rows.append((m, hit, msg))

    with open(RESULTS / "kubeconform.tsv", "w") as f:
        f.write("id\toperator\tfaulty\tkubeconform\tagones-check\tdry-run-prod\tmessage\n")
        for m, hit, msg in rows:
            f.write(f"{m['id']}\t{m['operator']}\t{m['faulty']}\t{hit}\t{m['detected'].get('agones-check', False)}\t"
                    f"{m['detected'].get('dry-run-prod', False)}\t{msg}\n")

    per_op = collections.OrderedDict()
    for m, hit, _ in rows:
        o = per_op.setdefault((m["operator"], m["faulty"]), [0, 0, 0, 0])
        o[0] += 1
        o[1] += hit
        o[2] += m["detected"].get("dry-run-prod", False)
        o[3] += m["detected"].get("agones-check", False)
    faulty = [r for r in rows if r[0]["faulty"]]
    benign = [r for r in rows if not r[0]["faulty"]]
    now = datetime.datetime.now().astimezone().strftime("%Y-%m-%d %H:%M %Z")
    lines = [
        "# kubeconform baseline",
        "",
        f"Generated {now} by `scripts/kubeconform-baseline.py`. kubeconform {version}, `-strict -ignore-missing-schemas`, "
        f"schemas converted from the Agones {agones} CRDs of the current cluster with unknown fields rejected. "
        "GameServerAllocation has no CRD, so kubeconform skips it.",
        "",
        f"Faulty mutants rejected: {sum(r[1] for r in faulty)} of {len(faulty)}. "
        f"Benign mutants rejected (false alarms): {sum(r[1] for r in benign)} of {len(benign)}.",
        "",
        "| Operator | Faulty | Mutants | kubeconform | dry-run-prod | agones-check |",
        "|---|---|---|---|---|---|",
    ]
    for (op, fl), (n, k, d, a) in per_op.items():
        lines.append(f"| {op} | {str(fl).lower()} | {n} | {k} | {d} | {a} |")
    (RESULTS / "kubeconform.md").write_text("\n".join(lines) + "\n")
    print("\n".join(lines))
    return 0


if __name__ == "__main__":
    sys.exit(main())
