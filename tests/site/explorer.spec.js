const {test,expect}=require('@playwright/test');
test('synthetic explorer supports time navigation and an accessible list',async({page})=>{
 await page.goto('/explore/');
 await expect(page.getByText(/SYNTHETIC DEMO/)).toBeVisible();
 await page.getByRole('tab',{name:'List'}).click();
 await expect(page.getByRole('list',{name:'Loaded memories'})).toBeVisible();
 await page.getByRole('button',{name:/Unknown/}).click();
 await expect(page.getByRole('button',{name:/Unknown/})).toHaveAttribute('aria-pressed','true');
 await page.getByRole('button',{name:/All time/}).click();
 await page.getByRole('searchbox',{name:'Search memories'}).fill('nothing-matches-this-fixture');
 await expect(page.getByRole('heading',{name:'No matching memories'})).toBeVisible();
 await page.getByRole('button',{name:'Clear filters'}).click();
 await expect(page.getByRole('list',{name:'Loaded memories'})).toBeVisible();
});
test('explorer works without WebGL and keeps synthetic data out of browser persistence',async({page})=>{
 await page.addInitScript(()=>{const original=HTMLCanvasElement.prototype.getContext;HTMLCanvasElement.prototype.getContext=function(type,...args){if(/webgl/i.test(type))return null;return original.call(this,type,...args);};});
 await page.goto('/explore/');
 await expect(page.getByRole('list',{name:'Loaded memories'})).toBeVisible();
 expect(await page.evaluate(()=>({local:localStorage.length,session:sessionStorage.length}))).toEqual({local:0,session:0});
});
test('changing filters aborts pending detail and clears its loading state',async({page})=>{
 const node={id:'fact:one',type:'fact',text:'Synthetic owner record',scope:'world',capturedAt:'2025-01-01T00:00:00Z'};
 const shell=await (await page.request.get('/explore/')).text();
 await page.route('**/dashboard/explore',route=>route.fulfill({contentType:'text/html',body:shell}));
 await page.route('**/api/inspector/v1/brains',route=>route.fulfill({json:{brains:[{id:'fixture',name:'Fixture'}]}}));
 await page.route('**/graph?*',route=>route.fulfill({json:{nodes:[node],contextNodes:[],edges:[],totalMatching:1}}));
 await page.route('**/facets?*',route=>route.fulfill({json:{years:[{year:'2025',count:1}]}}));
 let finishDetail;
 await page.route('**/nodes/**',async route=>{await new Promise(resolve=>{finishDetail=resolve;});await route.fulfill({json:{node,relatedNodes:[],edges:[]}}).catch(()=>{});});
 await page.goto('/dashboard/explore');
 await page.getByRole('tab',{name:'List'}).click();
 await page.getByRole('button',{name:/Synthetic owner record/}).click();
 await expect(page.getByText('Opening note…')).toBeVisible();
 await page.getByRole('combobox',{name:'Memory type'}).selectOption('fact');
 await expect(page.getByText('Opening note…')).not.toBeVisible();
 finishDetail();
 await expect(page.getByRole('heading',{name:'Your library, connected.'})).toBeVisible();
});
