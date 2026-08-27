#!/usr/bin/env bash
#
# the body of the warpmap action: work out the ratchet delta for a pull request, write it as a
# comment, and decide whether the job fails.
#
# it analyses two trees exported with `git archive` rather than the checked-out workspace. the
# baseline command writes .warpmap/baseline.json into the directory it snapshots, so running it
# in place would overwrite a baseline the project committed itself; exporting leaves the
# workspace exactly as it was found.
#
# every input arrives as an environment variable and action.yml maps the action's inputs onto
# them, so the script also runs outside github: point WARPMAP_BIN at a binary, set
# INPUT_BASE_SHA, and run it from inside a clone.

set -uo pipefail

bin="${WARPMAP_BIN:-warpmap}"
dir="${INPUT_DIRECTORY:-.}"
base="${INPUT_BASE_SHA:-}"
fail_on_risk="${INPUT_FAIL_ON_RISK:-true}"
want_comment="${INPUT_COMMENT:-true}"
token="${INPUT_TOKEN:-}"
pr="${INPUT_PR_NUMBER:-}"
repo="${GITHUB_REPOSITORY:-}"
api="${GITHUB_API_URL:-https://api.github.com}"
workspace="${GITHUB_WORKSPACE:-$PWD}"
tmp="${RUNNER_TEMP:-${TMPDIR:-/tmp}}"

# the marker is how the action finds its own previous comment, so a pull request collects one
# comment that gets edited rather than one per push. it is an html comment, so it does not show.
marker="<!-- warpmap-ratchet -->"
comment_url=""

die() {
	printf 'warpmap: %s\n' "$1" >&2
	exit 1
}

cd "$workspace" || die "cannot enter $workspace"

# a plain directory name is what every warpmap command wants; "./src" and "src/" are the same
# place and would otherwise produce two different prefixes when changed paths are matched.
dir="${dir#./}"
dir="${dir%/}"
[ -n "$dir" ] || dir="."

[ -n "$base" ] || die "no base commit. base-sha defaults to the base of the pull request, so it is only empty outside a pull_request event; pass it explicitly there."

# a default checkout is shallow and does not contain the base of the pull request. asking for it
# is worth a try, but the reliable fix is on the workflow side, so say so.
if ! git cat-file -e "${base}^{commit}" 2>/dev/null; then
	git fetch --no-tags --depth=1 origin "$base" >/dev/null 2>&1
fi
git cat-file -e "${base}^{commit}" 2>/dev/null ||
	die "commit $base is not in this clone; check out with fetch-depth: 0 so the base of the pull request is present"

work="$(mktemp -d "${tmp}/warpmap-XXXXXX")" || die "cannot create a working directory under $tmp"
trap 'rm -rf "$work"' EXIT
tree="$work/tree"
mkdir -p "$tree"

# one tree, used twice. the base commit goes in first and gets snapshotted; then its sources are
# swapped for the pull request's and the snapshot is compared against them in place. warpmap
# writes .warpmap/baseline.json itself and reads it back itself, so nothing here has to move it.
git archive "$base" | tar -x -C "$tree" || die "cannot export $base"

target="$tree"
[ "$dir" = "." ] || target="$tree/$dir"
# a pull request that creates the analysed directory has nothing on the base side; an empty
# directory is the honest baseline for that, not an error.
mkdir -p "$target"

"$bin" baseline "$target" >"$work/baseline.txt" 2>&1 || {
	cat "$work/baseline.txt" >&2
	die "cannot snapshot the base commit"
}

# clear the base sources, keep the snapshot. -mindepth 1 keeps the directory itself.
find "$target" -mindepth 1 -maxdepth 1 ! -name '.warpmap' -exec rm -rf {} + ||
	die "cannot clear the exported base sources"
git archive HEAD | tar -x -C "$tree" || die "cannot export the checked-out commit"
mkdir -p "$target"

"$bin" diff "$target" >"$work/ratchet.txt" 2>&1
case $? in
0) ratchet_state="ok" ;;
1) ratchet_state="risk-up" ;;
*)
	cat "$work/ratchet.txt" >&2
	die "diff could not run; a malformed warpmap.json is the usual cause"
	;;
esac

# only files that still exist can be traced, so deletions are dropped along with everything the
# graph does not read. paths come out relative to the repository root and warpmap wants them
# relative to the analysed directory.
files=()
while IFS= read -r p; do
	case "$p" in
	*.ts | *.tsx | *.js | *.jsx | *.py) ;;
	*) continue ;;
	esac
	if [ "$dir" != "." ]; then
		case "$p" in
		"$dir"/*) p="${p#"$dir"/}" ;;
		*) continue ;;
		esac
	fi
	[ -f "$target/$p" ] || continue
	files+=("$p")
done < <(git diff --name-only --diff-filter=ACMR "$base" HEAD)

risk_state="skipped"
if [ "${#files[@]}" -gt 0 ]; then
	"$bin" risk "$target" "${files[@]}" >"$work/risk.txt" 2>&1
	case $? in
	0) risk_state="manageable" ;;
	1) risk_state="risky" ;;
	*)
		cat "$work/risk.txt" >&2
		die "risk could not run; a malformed warpmap.json is the usual cause"
		;;
	esac
fi

case "$ratchet_state:$risk_state" in
risk-up:*) headline="risk up" ;;
ok:risky) headline="high-blast untested files changed" ;;
*) headline="no net degradation" ;;
esac

body="$work/comment.md"
{
	printf '%s\n' "$marker"
	printf '### warpmap: %s\n\n' "$headline"
	printf 'the ratchet, against `%s`:\n\n' "$(git rev-parse --short "$base")"
	printf '```text\n'
	cat "$work/ratchet.txt"
	printf '```\n'
	if [ "$risk_state" != "skipped" ]; then
		printf '\nthe %d source file(s) this pull request adds or changes:\n\n' "${#files[@]}"
		printf '```text\n'
		cat "$work/risk.txt"
		printf '```\n'
	fi
	printf '\n<sub>%s - the baseline is the commit this pull request targets, so the delta belongs to this pull request alone.</sub>\n' \
		"$("$bin" version 2>/dev/null || printf 'warpmap')"
} >"$body"

cat "$body"

# commenting is best-effort on purpose: a build that would otherwise have passed must not fail
# because a comment did not land. every path here logs why and returns 0.
post_comment() {
	local payload="$work/payload.json" resp="$work/resp.json" id url method code
	if [ "$want_comment" != "true" ]; then
		echo "comment: turned off by the comment input"
		return 0
	fi
	if [ -z "$token" ] || [ -z "$pr" ] || [ -z "$repo" ]; then
		echo "comment: skipped, no token, pull request number or repository in the environment"
		return 0
	fi
	if ! command -v curl >/dev/null 2>&1 || ! command -v jq >/dev/null 2>&1; then
		echo "comment: skipped, curl and jq are what talk to the api"
		return 0
	fi
	jq -Rs '{body: .}' <"$body" >"$payload" 2>/dev/null || {
		echo "comment: skipped, could not encode the body"
		return 0
	}

	id="$(curl -sS --max-time 30 \
		-H "authorization: Bearer $token" \
		-H "accept: application/vnd.github+json" \
		"$api/repos/$repo/issues/$pr/comments?per_page=100" 2>/dev/null |
		jq -r --arg m "$marker" \
			'if type == "array" then (map(select((.body // "") | contains($m))) | last | .id // "") else "" end' 2>/dev/null)"

	url="$api/repos/$repo/issues/$pr/comments"
	method="POST"
	if [ -n "$id" ]; then
		url="$api/repos/$repo/issues/comments/$id"
		method="PATCH"
	fi

	code="$(curl -sS --max-time 30 -o "$resp" -w '%{http_code}' -X "$method" \
		-H "authorization: Bearer $token" \
		-H "accept: application/vnd.github+json" \
		-H "content-type: application/json" \
		-d "@$payload" "$url" 2>/dev/null)"
	case "$code" in
	2*)
		comment_url="$(jq -r '.html_url // ""' <"$resp" 2>/dev/null)"
		echo "comment: $method $code $comment_url"
		;;
	*)
		echo "comment: the api answered $code, so nothing was written; the report is above and the gate is unaffected"
		[ -s "$resp" ] && head -c 400 "$resp"
		;;
	esac
	return 0
}

post_comment

if [ -n "${GITHUB_OUTPUT:-}" ]; then
	{
		printf 'ratchet=%s\n' "$ratchet_state"
		printf 'risk=%s\n' "$risk_state"
		printf 'comment-url=%s\n' "$comment_url"
		# a heredoc delimiter, because the report is markdown and contains newlines
		printf 'report<<WARPMAP_REPORT_EOF\n'
		cat "$body"
		printf 'WARPMAP_REPORT_EOF\n'
	} >>"$GITHUB_OUTPUT"
fi

if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
	cat "$body" >>"$GITHUB_STEP_SUMMARY"
fi

if [ "$fail_on_risk" = "true" ] && { [ "$ratchet_state" = "risk-up" ] || [ "$risk_state" = "risky" ]; }; then
	exit 1
fi
exit 0
