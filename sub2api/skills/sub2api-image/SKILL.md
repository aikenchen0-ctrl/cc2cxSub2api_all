---
name: sub2api-image
description: Generate or edit raster images through Sub2API's image API with a separate image-group key. Use for Sub2API image requests, or ordinary image work when the user has chosen Sub2API as their image provider.
---

# Sub2API Image

Use `scripts/image_api.py` (Python 3.10+, standard library only) to generate or
edit images. Text/code tasks continue through the user's existing provider.
Do not change Codex's `config.toml`, `auth.json`, or other skills.
Respect an explicit request to use another image provider or built-in tool.

## Setup

Run `python "<skill-dir>/scripts/image_api.py" status` to check configuration.
Only `SUB2API_IMAGE_API_KEY`, `SUB2API_IMAGE_BASE_URL`, and this skill's private
credential file are used. Never substitute the text provider's key or account
login credentials.

If missing, ask the user to provide an API key created under a Sub2API group with image generation
enabled. When reporting the missing-key condition, tell the user:
“未配置生图密钥，请先配置生图密钥。” Default base URL: `https://api.cc2.cx`. Do not invent a key or use a
site administrator's upstream credentials.

For persistent setup, have the user run:

```sh
python "<skill-dir>/scripts/image_api.py" configure
```

The command prompts for the key without echoing it. With explicit permission to
save a provided key, `configure --key-stdin` also accepts the key through stdin.
Never put keys in command arguments, prompts sent to the image model, logs,
source files, or generated artifacts. For one-off use, pass
`SUB2API_IMAGE_API_KEY` in the child process environment. Do not persist a one-off
key unless the user asks.

Credentials live under `$CODEX_HOME/credentials/sub2api-image.json` (default
`~/.codex`). Windows uses current-user DPAPI; other systems use a file with mode
0600. Replacing the base URL requires an explicitly provided key for that
provider. A saved key is never silently sent to a different base URL.

## Workflow

1. Decide between generation and editing. Any reference image or mask uses
   `edit`; do not turn an edit into a fresh generation without telling the user.
2. Write a focused prompt: subject, composition, visual style, required wording,
   output use, and what must stay unchanged for edits. Pass long prompts using
   a UTF-8 `--prompt-file`.
3. Start with one image. Request multiple images only when the user asks.
4. Run the script. Save outputs within the user's task directory.
5. Inspect the resulting image with an available image-viewing tool. Check
   requested content and legibility before claiming success. Return the actual
   local file path and use it in the ongoing project when requested.

```sh
python "<skill-dir>/scripts/image_api.py" generate \
  --prompt "A red enamel teapot on a white background, product photo" \
  --size 1024x1024 --out outputs/teapot.png

python "<skill-dir>/scripts/image_api.py" edit \
  --image inputs/reference.png --prompt "Change only the background to pale blue" \
  --out outputs/edited.png
```

On PowerShell, use a single command line or PowerShell backticks instead of `\`.

- `--model` defaults to `gpt-image-2.6-flare`. Override only for an image model actually
  offered by the chosen group; do not silently downgrade after an error.
- `--size` defaults to `1024x1024`, `--quality` to `auto`, `--n` to 1.
  Model/size/quality availability is determined by the upstream.
- Edits support repeated `--image`, plus an optional `--mask` PNG.
  A mask must match the first input's dimensions and contain a transparent edit
  region; inspect these before submission.
- `--output-format png|jpeg|webp`, `--timeout` (default 180 seconds, max 600).
- Multiple outputs use `name-1.png`, `name-2.png`, etc. Existing files are never
  overwritten. The response's format determines the saved extension.
- This version uses synchronous JSON Images API calls. It does not implement
  Responses image tools, streaming previews, or background task polling.

## Errors and Billing

Each submitted image request may be billable. The script never automatically
retries a generation/edit POST. On timeout or dropped connection, report that
the result is uncertain and check the site's usage history before asking to
submit another paid request.

For 401, check the dedicated image key; for 403, check image-group permissions;
for 429, wait; for insufficient balance, ask the user to recharge. Surface the
sanitized upstream message. Do not claim a failed request was free.

Base64 images and HTTPS image URLs are supported. Image URL downloads never
receive the API key. Redirects are rejected rather than forwarding credentials.
Do not fabricate a completed image, rename a file to imply another image format,
or claim native 2K/4K dimensions based only on the requested size.
