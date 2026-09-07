# Static site authoring

The files under `site/` are the deployable product. GitHub Pages uploads them directly, with no generation or framework build.

To revise the shared page content/templates locally:

```sh
python3 scripts/site/build-home.py
python3 scripts/site/build-chat.py
python3 scripts/site/version-assets.py
python3 deploy/chat/package.py
python3 scripts/site/check.py
python3 -m unittest discover -s deploy/chat -p 'test_*.py' -v
```

`build-home.py` refreshes the supporting pages through `build-content.py`; `build-chat.py` uses the vendored original composer CSS. Shared site CSS and browser scripts are authored directly. After changing CSS/JS, run `version-assets.py` to avoid stale assets. The chat URL configuration is a public endpoint and is preserved during regeneration. Regenerate and deploy the Lambda template when public documentation changes so its bundled corpus stays identical.

The initial motion/video URLs and reference-card proportions come from the user-supplied brief. Logo SVGs are standalone vector assets. The designer's intent and verification notes live under `docs/design/serenity/`.
