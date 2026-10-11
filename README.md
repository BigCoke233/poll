# nobloat/poll

A minimal alternative to Doodle: born out of frustration with Doodle's ongoing enshit***ion, and from wanting to build something really minimal and highly functional again.

- **Plain Go stdlib only**, no dependencies (see [go.mod](go.mod)), plus a little JavaScript and CSS
- One statically compiled binary, in the spirit of [nobloat.org](https://nobloat.org)
- **No** *node*, *npm*, *react/svelte/angular/vue* or database

## Features

- Create polls without an account: a name is enough, email is optional
- Answer polls without an account: a name is enough, email is optional (without it, no updates are sent)
- Mails via SMTP with personal links to manage a poll or change an answer
- GDPR friendly: each poll is one `.json` file; delete the file (or use the web UI) and it's gone
- Every change to a poll is exclusively locked
- Admin (Basic Auth) can list and manage all polls
- Public poll creation can be switched off (admin only)
- Easy [theming](#theming)
- Light and dark mode, following the system setting
- [Translations](#translations) (English, German, French, Spanish), picked from the browser language

## Non-features

- Tracking / analytics
- Account management
- Sophisticated logins: one admin password, personal links for everyone else
- Scaling to millions of polls with thousands of participants voting at the same time

## Screenshots

**minimal**

![minimal theme](screenshots/minimal.png)

**elegant**

![elegant theme](screenshots/elegant.png)

**business**

![business theme](screenshots/business.png)

**fun**

![fun theme](screenshots/fun.png)

## Development

```sh
make dev          # Mailpit (mail UI on http://localhost:8025) + go run . (creates .env from .env.sample)
make test         # go vet + go test -race
make build        # static ./poll binary
make package      # Docker image ghcr.io/nobloat/poll:latest
make push         # package + push (TAG=v1.0 for a version)
make screenshots  # all pages × themes × desktop/phone × light/dark (needs chromium + ImageMagick)
```

- Admin: http://localhost:8737/a (`admin` / `admin`)
- CI (GitHub Actions) runs `make test` and `make build` on every push to `main` and on pull requests
- Release: push a tag like `v1.2.0`; GitHub Actions builds and pushes `ghcr.io/nobloat/poll:v1.2.0` and `:latest`. The footer shows the version and short commit
- Example polls: an empty `./data` gets the polls from `seed/`; `rm -rf data` resets them
- Code layout, one Go package:
  - `main.go`: config, startup
  - `server.go`: routes, participant pages
  - `manage.go`: creating and managing polls
  - `store.go`, `mail.go`, `pow.go`: storage, mail, spam check
  - `templates/` HTML, `mail/` plain-text mails, `i18n/` texts, `static/` CSS and JS

## Deployment

- Configure with environment variables; [`.env.sample`](.env.sample) documents every setting
- Listens on `:8737` (`PORT` to change), stores polls in `./data` (`/data` in the image)
- Startup checks the config and the SMTP login, and exits with a list of problems if anything is wrong

```sh
mkdir data
docker run -d --restart unless-stopped --env-file .env --user 1000:1000 -v ./data:/data \
  -p 127.0.0.1:8737:8737 ghcr.io/nobloat/poll:latest
```

Or with Docker Compose:

```yaml
services:
  poll:
    image: ghcr.io/nobloat/poll:latest
    restart: unless-stopped
    env_file: .env
    user: "1000:1000"
    volumes: [./data:/data]
    ports: ["127.0.0.1:8737:8737"]
```

## Theming

- Built-in themes in [`static/themes/`](static/themes): `minimal` (default), `elegant`, `business`, `fun`
- `THEME` sets the default; each poll can pick its own in the poll form
- A theme is a small CSS file that overrides the `:root` variables in [`static/app.css`](static/app.css) (colors, font, borders, corners, shadows, buttons) and may refine any other rule
- Add one: put it in `static/themes/<name>.css` and rebuild, it's picked up automatically
- Use `light-dark(light, dark)` for colors
- Fonts: only the built-in generic families (`system-ui`, `ui-monospace`, `ui-serif`, `ui-rounded`, `sans-serif`, …), no font files

## Translations

- All texts, including mails, are in [`i18n/`](i18n): `en.json`, `de.json`, `fr.json`, `es.json`
- Add a language: copy `en.json` to `<code>.json` (e.g. `it.json`), translate the values (keep keys and `%s` placeholders), rebuild
- The language comes from the browser's `Accept-Language`, English is the fallback
- Mails use the language a participant had when they answered
- `make test` fails if a file is missing a key or has extra ones
