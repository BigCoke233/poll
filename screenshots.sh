#!/usr/bin/env bash
# Renders every page in every built-in theme (desktop + phone, light + dark) from the seed
# data into screenshots/<theme>/ (ignored), plus screenshots/<theme>.png for the README (the
# poll page: desktop light + dark, phone light + dark) if ImageMagick is installed.
# Needs chromium or google-chrome, and Docker for Mailpit. Usage: make screenshots
set -euo pipefail
cd "$(dirname "$0")"

chrome=$(command -v chromium || command -v chromium-browser || command -v google-chrome || true)
[ -n "$chrome" ] || { echo "screenshots: chromium or google-chrome not found" >&2; exit 1; }

out=screenshots tmp=$(mktemp -d)
trap 'jobs -p | xargs -r kill 2>/dev/null || true; rm -rf "$tmp"' EXIT
rm -rf "$out"/*/ && mkdir -p "$out"
CGO_ENABLED=0 go build -o "$tmp/poll" .
# the app checks SMTP on startup; closing mails land in Mailpit (reuse one already on :1025)
(exec 3<>/dev/tcp/localhost/1025) 2>/dev/null || make -s mailpit >/dev/null

# each theme's seed poll (seed/<slug>.json, open, with answers) and the leading option it is closed on for the closed page
declare -A polls=([minimal]=pizza-friday:0 [elegant]=book-club:0 [business]=office-plant:1 [fun]=karaoke:0)

shots() { # page=path ... (uses theme and base from shoot)
	for page; do
		for size in desktop=1280,900 phone=390,844; do
			for scheme in light=1 dark=0; do
				"$chrome" --headless=new --disable-gpu --no-sandbox --hide-scrollbars --user-data-dir="$tmp/chrome-$theme" \
					--window-size="${size#*=}" --blink-settings="preferredColorScheme=${scheme#*=}" --virtual-time-budget=2000 \
					--screenshot="$out/$theme/${page%%=*}-${size%%=*}-${scheme%%=*}.png" \
					"$base${page#*=}" >/dev/null 2>&1
			done
		done
	done
}

shoot() { # theme: one instance on :8737 with its own data dir, seeded from seed/
	local theme=$1 slug=${polls[$1]%:*} outcome=${polls[$1]#*:} base="http://admin:admin@localhost:8737"
	mkdir -p "$tmp/$theme/data/polls" "$out/$theme" && cp seed/*.json "$tmp/$theme/data/polls/"
	(cd "$tmp/$theme" && exec env -i PATH="$PATH" BASE_URL=http://localhost:8737 ADMIN_PASSWORD=admin \
		ADMIN_EMAIL=admin@example.com SMTP_URL=smtp+insecure://localhost:1025 SMTP_FROM=poll@example.com \
		PUBLIC_CREATE=true THEME="$theme" "$tmp/poll") >"$tmp/$theme.log" 2>&1 &
	local pid=$!
	until curl -s -o /dev/null localhost:8737/logo; do
		kill -0 "$pid" 2>/dev/null || { echo "screenshots: $theme server died:" >&2; cat "$tmp/$theme.log" >&2; exit 1; }
		sleep .1
	done
	local token=$(grep -o '"token": "[A-Z2-7]*"' "seed/$slug.json" | head -1 | cut -d'"' -f4) # first participant
	shots start=/ new=/new poll=/p/$slug answer=/p/$slug/a/$token admin=/a admin-poll=/a/p/$slug
	curl -sf -o /dev/null -d option=$outcome "$base/a/p/$slug/close"
	shots closed=/p/$slug
	if command -v magick >/dev/null; then
		local p="$out/$theme/poll"
		magick "$p-desktop-light.png" "$p-desktop-dark.png" "$p-phone-light.png" "$p-phone-dark.png" \
			-resize x600 -background none -splice 16x0 +append -chop 16x0 -strip "$out/$theme.png"
	fi
	kill "$pid" && wait "$pid" 2>/dev/null || true # free the port for the next theme
	echo "screenshots: $theme done"
}

# the app listens on the fixed port 8737, so themes run one after another
! curl -s -o /dev/null localhost:8737 || { echo "screenshots: port 8737 is in use (make dev running?)" >&2; exit 1; }
for theme in "${!polls[@]}"; do shoot "$theme"; done
echo "screenshots: $(find "$out" -name '*.png' | wc -l) images in $out/"
