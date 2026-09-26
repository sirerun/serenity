#!/usr/bin/env python3
"""Content-version stylesheet/script URLs in ready-to-serve HTML after authoring."""
from pathlib import Path
import hashlib,re
repo=Path(__file__).resolve().parents[2]
root=repo/'site'
paths=list(root.rglob('*.html')) + [repo/'internal/hosted/dashboard/page.html', repo/'internal/hosted/oauth/connections.go', repo/'internal/hosted/oauth/hosted.go']
for path in paths:
 text=path.read_text()
 def version(match):
  asset=match[1]
  # The dashboard's small script is served dynamically, not from site/assets.
  if asset == '/assets/app.js': return match[0]
  digest=hashlib.sha256((root/asset.lstrip('/')).read_bytes()).hexdigest()[:10]
  return asset+'?v='+digest
 text=re.sub(r'(/assets/[^"\s?]+\.(?:css|js))(?:\?v=[^"\s]+)?',version,text)
 path.write_text(text)
