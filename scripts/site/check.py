#!/usr/bin/env python3
from html.parser import HTMLParser
from pathlib import Path
from urllib.parse import urlsplit, unquote
import json
root=Path(__file__).resolve().parents[2]/'site'
class Page(HTMLParser):
 def __init__(self):super().__init__();self.links=[];self.ids=set();self.titles=[];self.in_title=False
 def handle_starttag(self,tag,attrs):
  d=dict(attrs)
  if 'id' in d:self.ids.add(d['id'])
  for key in ('href','src'):
   if key in d:self.links.append(d[key])
  if tag=='title':self.in_title=True
 def handle_endtag(self,tag):
  if tag=='title':self.in_title=False
 def handle_data(self,data):
  if self.in_title:self.titles.append(data)
parsed={};errors=[];count=0
for f in root.rglob('*.html'):
 p=Page();p.feed(f.read_text());parsed[f]=p
 if not p.titles or '<br>' in ''.join(p.titles):errors.append(f'{f}: invalid title')
for f,p in parsed.items():
 for url in p.links:
  u=urlsplit(url)
  if u.scheme or u.netloc:continue
  target=(root/u.path.lstrip('/')) if u.path.startswith('/') else (f.parent/u.path)
  if not u.path:target=f
  if target.is_dir():target=target/'index.html'
  count+=1
  if not target.exists():errors.append(f'{f.relative_to(root)}: missing {url}')
  elif u.fragment and target in parsed and unquote(u.fragment) not in parsed[target].ids:errors.append(f'{f.relative_to(root)}: missing anchor {url}')
print(json.dumps({'pages':len(parsed),'local_links_checked':count,'errors':errors},indent=2))
raise SystemExit(bool(errors))
