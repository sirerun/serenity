#!/usr/bin/env python3
"""Content-version stylesheet/script URLs in ready-to-serve HTML after authoring."""
from pathlib import Path
import hashlib,re
root=Path(__file__).resolve().parents[2]/'site'
for path in root.rglob('*.html'):
 text=path.read_text()
 def version(match):
  asset=match[1];digest=hashlib.sha256((root/asset.lstrip('/')).read_bytes()).hexdigest()[:10]
  return asset+'?v='+digest
 text=re.sub(r'(/assets/[^"\s?]+\.(?:css|js))(?:\?v=[^"\s]+)?',version,text)
 path.write_text(text)
