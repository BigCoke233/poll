# nobloat/poll

A minimal alternative to Doodle: born out of frustration with Doodle's ongoing enshit***ion, and from wanting to build something really minimal and highly functional again. 

Its written using the **plain go stdlib only** (no dependencies, see [go.mod](go.mod)), with some JavaScript and CSS.
Everything ships in a single statically compiled binary, following the spirit of [nobloat.org](https://nobloat.org)

**It doesnt require: *node*, *npm*, *react/svelte/angular/vue*, a database.**

## Features

- You can easily create polls without having to create an account
- You can participate in polls without having to create an account
- Send out e-mails via SMTP using unique *manage links* to edit the poll or your answer.
- Participants only need a name; email is optional (without it, no updates are sent).
- GDPR compliant: Delete polls / answers, by dropping a single file from the filesystem. Done. This of course works also via Web.
- Each poll is a simple `.json` file on disk. Exclusively locked on each edit to the poll or the answers. 
- Admin view with `Basic Auth` to view all created events.
- Disable public poll creation and allow only via admin view.
- Easy [theming](#theming)
- 
## Non features

- Tracking / Analytics
- Account management
- Sophisticated Logins, we hav
- Scaling to million of polls with thousands of participants all voting at the same time.

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

See [Makefile](Makefile)

```sh
make dev          # Mailpit in Docker (mail UI on http://localhost:8025) + go run . (creates .env from .env.sample)
make test         # go vet + go test -race
make build        # static ./poll binary
make package      # Docker image ghcr.io/nobloat/poll:latest
make push         # package + push (TAG=v1.0 for a version)
make screenshots  # all pages × themes × desktop/phone × light/dark (needs chromium + ImageMagick)
```

Open http://localhost:8737/a (`admin` / `admin`). An empty `./data` gets the example polls from `seed/` (`rm -rf data` resets them).

One Go package: `main.go` (config, startup), `server.go` (routes, participant pages), `manage.go` (creating and managing polls), `store.go`, `mail.go`, `pow.go`; HTML in `templates/`, plain-text mails in `mail/`, texts in `i18n/` (en, de), CSS and JS in `static/`.

## Deployment

Configure with environment variables; [`.env.sample`](.env.sample) documents every setting. The app listens on `:8737` and stores polls in `./data` (`/data` in the image).
On startup it validates the config and logs in to the SMTP server; on any problem it exits with a list of what's wrong.

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

Put it behind a TLS reverse proxy that keeps the `Host` header. Run a single instance on a local disk (the storage lock is in-process). Back up `data/` and keep it private: it holds participant emails and personal links.

## Theming

Built-in themes are in [`static/themes/`](static/themes): `minimal` (default), `elegant`, `business`, `fun`. Pick the instance default with `THEME`; each poll can also choose its own in the poll form.

A theme is a small CSS file that overrides the variables on `:root` in [`static/app.css`](static/app.css) (colors, font, borders, corners, shadows, buttons) and may refine any other rule, since it is loaded after the main stylesheet. To add one, put it in `static/themes/<name>.css` and rebuild; it is picked up automatically.

Use `light-dark(light, dark)` for colors, and only system or self-hosted fonts (the Content Security Policy blocks font CDNs).

## Translations

All texts, including mails, are in [`i18n/`](i18n): `en.json` and `de.json`. To add a language, copy `en.json` to `<code>.json` (e.g. `fr.json`), translate the values and keep the keys and `%s` placeholders, then rebuild. New files are picked up automatically. The language comes from the browser's `Accept-Language` header, with English as the fallback. Mails use the language a participant had when they answered. `make test` fails if a file is missing a key or has extra ones.
