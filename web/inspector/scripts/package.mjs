import { cp, mkdir, readFile, readdir, unlink, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../../..');
const output=path.join(root,'site/assets/explorer');
await mkdir(output,{recursive:true});
// This directory is generated exclusively by this build; do not delete other assets.
for(const file of await readdir(output)) if(/\.(js|css)$/.test(file)) await unlink(path.join(output,file));
for(const file of await readdir(new URL('../dist/',import.meta.url))) if(file!=='index.html') await cp(new URL('../dist/'+file,import.meta.url),path.join(output,file),{recursive:true});
await mkdir(path.join(root,'site/explore'),{recursive:true});
await writeFile(path.join(root,'site/explore/index.html'),await readFile(new URL('../dist/index.html',import.meta.url)));
