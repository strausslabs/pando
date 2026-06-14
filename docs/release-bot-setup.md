# Homebrew formula bot — one-time setup

The daily release regenerates `Formula/pando.rb` and pushes it to `main`. `main`
is ruleset-protected, and the default `GITHUB_TOKEN` cannot bypass a ruleset. A
small **GitHub App** owned by the org can: its short-lived token pushes the
formula, and the app is listed as a ruleset bypass actor.

This is browser-only setup (creating an app can't be scripted). ~5 minutes.

## 1. Create the app (org-owned)

<https://github.com/organizations/strausslabs/settings/apps/new>

- **Name:** `pando-release-bot` (any unique name)
- **Homepage URL:** `https://github.com/strausslabs/pando`
- **Webhook:** uncheck **Active** (no webhook needed)
- **Repository permissions → Contents:** **Read and write**
- Everything else: leave default
- **Where can this app be installed:** *Only on this account*
- Click **Create GitHub App**

## 2. Grab credentials

On the app's page:
- Note the **App ID** (a number).
- **Generate a private key** → downloads a `.pem`. Keep it; you'll paste it once.

## 3. Install the app on the repo

App page → **Install App** → install on `strausslabs`, **Only select
repositories → pando**.

## 4. Add the repo variable + secret

```sh
# App ID is not sensitive — a repo variable:
unset GITHUB_TOKEN
gh variable set FORMULA_APP_ID --repo strausslabs/pando --body "<APP_ID>"

# Private key is sensitive — a secret (reads the file, never echoes it):
gh secret set FORMULA_APP_PRIVATE_KEY --repo strausslabs/pando < path/to/app.pem
```

## 5. Tell me the App ID

Once installed, give me the App ID. I add the app as a ruleset bypass actor:

```sh
# (run by maintainer; APP_ID is the numeric app id)
gh api repos/strausslabs/pando/rulesets/17655673 --method PUT \
  --input - <<JSON
{ "bypass_actors": [
    { "actor_id": 5, "actor_type": "RepositoryRole", "bypass_mode": "always" },
    { "actor_id": <APP_ID>, "actor_type": "Integration", "bypass_mode": "always" }
] }
JSON
```

After that, `TAP_TOKEN` is unused — delete it, and revoke the leaked PAT.

## How it fails safe

`release.yml` only mints the token when `vars.FORMULA_APP_ID` is set, and only
pushes the formula when the token is non-empty. Until setup is done the release
still publishes binaries; just the in-repo formula update no-ops.
