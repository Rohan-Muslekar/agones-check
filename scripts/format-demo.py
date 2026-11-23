"""Render the validator's JSON report for the classroom demo."""

import json
import sys
from collections import Counter

from rich.columns import Columns
from rich.console import Console, Group
from rich.panel import Panel
from rich.table import Table
from rich.text import Text


SEVERITY = {"error": "red", "warning": "yellow", "info": "cyan"}
STATUS = {"ok": "green", "warning": "yellow", "broken": "red", "rejected": "red"}
STATUS_PRIORITY = {"ok": 0, "warning": 1, "broken": 2, "rejected": 3}
FINDING_PREVIEW = 2
MATRIX_PREVIEW = 5


def summary_view(title, resource_count, profiles, counts, width):
    heading = Text.assemble(
        ("AGONES-CHECK", "bold cyan"), f"  ·  {title}\n",
        (f"{len(profiles)} environments · {', '.join(profiles)}", "dim"),
    )
    metrics = [
        ("Total resources", resource_count, "cyan"),
        ("Errors", counts["error"], "red" if counts["error"] else "green"),
        ("Warnings", counts["warning"], "yellow" if counts["warning"] else "green"),
    ]
    cards = [
        Panel(Text(str(count), style=f"bold {color}", justify="center"),
              title=label, border_style=color, width=19)
        for label, count, color in metrics
    ]
    return Group(heading, Columns(cards, padding=(0, 1)) if width >= 64 else Group(*cards))


def findings_view(findings, full):
    if not findings:
        return [Text("No findings. All checks passed.", style="bold green")]

    ordered = [item for severity in SEVERITY for item in findings
               if item["severity"] == severity]
    visible = ordered if full else ordered[:FINDING_PREVIEW]
    sections = [Text(f"Findings ({len(findings)})", style="bold cyan")]
    for finding in visible:
        severity = finding["severity"]
        resource = finding["object"]
        if not full:
            resource = resource.rsplit("/", 1)[-1]
        env = ", ".join(finding.get("profiles") or ["all environments"])
        source = finding["source"]
        location = f"{source['file']}:{source['line']}"
        if finding.get("field"):
            location += f" · {finding['field']}"
        card = Text.assemble(
            (f"{severity.upper()} · {finding['rule']}", f"bold {SEVERITY[severity]}"),
            f"\n{resource} · {env}\n{finding['message']}\n",
            (location, "dim"),
        )
        if finding.get("related"):
            card.append("\nRelated: " + ", ".join(finding["related"]), style="dim")
        sections.append(card)
    if len(visible) < len(findings):
        sections.append(Text(f"{len(findings) - len(visible)} more findings in full details.", style="dim"))
    return sections


def matrix_view(objects, profiles, matrix, full):
    rows = [(resource, matrix[resource]) for resource in objects]
    for resource, statuses in rows:
        if len(statuses) != len(profiles):
            raise ValueError(f"Invalid matrix row for {resource}")
        for status in statuses:
            if status not in STATUS:
                raise ValueError(f"Unknown matrix status for {resource}: {status}")
    if not full:
        rows.sort(key=lambda row: max(STATUS_PRIORITY[status] for status in row[1]), reverse=True)
        rows = rows[:MATRIX_PREVIEW]

    table = Table(title="Environment compatibility", title_style="bold cyan",
                  title_justify="left", box=None, pad_edge=False)
    table.add_column("Resource", header_style="bold cyan", overflow="fold")
    for profile in profiles:
        table.add_column(Text(profile), header_style="bold cyan", overflow="fold")
    for resource, statuses in rows:
        priority = max(statuses, key=STATUS_PRIORITY.get)
        label = resource if full else resource.rsplit("/", 1)[-1]
        style = STATUS[priority] if priority != "ok" else ""
        cells = [Text(status, style=STATUS[status]) for status in statuses]
        table.add_row(Text(label, style=style), *cells)
    legend = Text.assemble(
        ("ok", "green"), " passes  ·  ", ("warning", "yellow"), " warning only\n",
        ("broken", "red"), " fails at runtime  ·  ", ("rejected", "red"), " refused at apply",
    )
    if len(rows) < len(objects):
        legend.append(f"\n{len(objects) - len(rows)} more resource rows in full details.", style="dim")
    return [Group(table, legend)]


def main():
    title, first, view = sys.argv[1:]
    full = view == "full"
    report = json.load(sys.stdin)
    console = Console()
    objects = report["objects"] or []
    profiles = report["profiles"]
    findings = report["findings"]
    counts = Counter(finding["severity"] for finding in findings)
    if counts.keys() - SEVERITY.keys():
        raise ValueError("Unknown finding severity")

    summary = summary_view(title, len(objects), profiles, counts, console.width)
    sections = {
        "findings": findings_view(findings, full),
        "matrix": matrix_view(objects, profiles, report["matrix"], full),
    }
    second = "matrix" if first == "findings" else "findings"
    console.print(summary, *sections[first], *sections[second], sep="\n\n")


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        sys.exit(f"Report formatter: {error}")
