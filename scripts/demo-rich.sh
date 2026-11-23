#!/usr/bin/env bash
set -u

# Prefer system Python, then try the interpreter used by a pipx Rich CLI install.
python=python3
if ! "$python" -c 'import rich' 2>/dev/null; then
    rich_path=$(type -P rich)
    if [[ -n $rich_path ]] && read -r shebang _ < "$rich_path" && [[ $shebang == '#!/'* ]]; then
        python=${shebang#\#!}
    fi
    if ! "$python" -c 'import rich' 2>/dev/null; then
        printf 'Install Python Rich with python3 -m pip install rich (in a virtual environment), or use pipx install rich-cli.\n' >&2
        exit 0
    fi
fi
if ! command -v go >/dev/null; then
    printf 'Go is required to run this demo.\n' >&2
    exit 2
fi

cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." || exit 2
scratch=$(mktemp -d) || exit 2
trap 'rm -rf -- "$scratch"' EXIT

# go run maps all nonzero program statuses to 1. Run the binary directly.
go build -o "$scratch/agones-check" ./cmd/agones-check || exit 2

interactive=false
if [[ -t 0 && -t 1 && ${TERM:-dumb} != dumb ]]; then
    interactive=true
fi

redraw() {
    if "$interactive"; then
        printf '\033[2J\033[H'
    fi
}

show_report() {
    redraw
    if ! "$python" scripts/format-demo.py "$title" "$first" "$1" < "$scratch/output"; then
        printf 'Rich formatting failed. Showing the original JSON report.\n' >&2
        cat -- "$scratch/output" || exit 2
    fi
    case "$status" in
        0) printf 'Validation completed (exit 0).\n\n' ;;
        1) printf 'Validation reported findings (exit 1).\n\n' ;;
    esac
}

while true; do
    redraw
    printf '%s\n' 'AGONES-CHECK demo' \
        '1. Clean manifests — backend environment matrix' \
        '2. Error detection — findings across all fixtures' \
        '3. Environment compatibility — full environment matrix' \
        'q. Quit'
    read -r -p 'Choose a scenario: ' choice || exit 0
    case "$choice" in
        1) title='Clean manifests'; path=testdata/backend/; first=matrix ;;
        2) title='Error detection'; path=testdata/; first=findings ;;
        3) title='Environment compatibility'; path=testdata/; first=matrix ;;
        q|Q) exit 0 ;;
        *) continue ;;
    esac

    "$scratch/agones-check" -e prod,dev -o json -min-severity info "$path" > "$scratch/output"
    status=$?
    case "$status" in
        0|1) ;;
        *) cat -- "$scratch/output"
           printf 'Validator failed (exit %s).\n' "$status" >&2
           exit "$status" ;;
    esac

    if "$interactive"; then
        view=summary
        while true; do
            show_report "$view"
            if [[ $view == summary ]]; then
                read -r -p '[d] Full details  [Enter] Menu  [q] Quit: ' choice || exit 0
                case "$choice" in
                    d|D) view=full ;;
                    q|Q) exit 0 ;;
                    *) break ;;
                esac
            else
                read -r -p '[Enter] Menu  [q] Quit: ' choice || exit 0
                case "$choice" in q|Q) exit 0 ;; esac
                break
            fi
        done
    else
        show_report full
    fi
done
