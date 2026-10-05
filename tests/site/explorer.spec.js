const {test,expect}=require('@playwright/test');
async function renderedMap(page,canvas){
 await canvas.scrollIntoViewIfNeeded();
 const box=await canvas.boundingBox();
 if(!box) throw new Error('Explorer canvas has no rendered bounds');
 // Crop inside the canvas so its keyboard focus outline cannot affect comparisons.
 return page.screenshot({clip:{x:box.x+8,y:box.y+8,width:box.width-16,height:box.height-16},animations:'disabled'});
}
test('synthetic explorer supports time navigation and an accessible list',async({page},testInfo)=>{
 await page.goto('/explore/');
 await expect(page.getByText('Synthetic demo',{exact:true})).toBeVisible();
 await expect(page.locator('canvas.graph-canvas')).toHaveCount(1);
 await page.screenshot({path:testInfo.outputPath('explorer-map.png'),fullPage:true});
 await page.getByRole('button',{name:'List',exact:true}).click();
 await expect(page.getByRole('list',{name:'Loaded memories'})).toBeVisible();
 await page.getByRole('button',{name:/Unknown/}).click();
 await expect(page.getByRole('button',{name:/Unknown/})).toHaveAttribute('aria-pressed','true');
 await page.getByRole('button',{name:/All time/}).click();
 await page.getByRole('textbox',{name:'Search memories'}).fill('nothing-matches-this-fixture');
 await expect(page.getByRole('heading',{name:'No matching memories'})).toBeVisible();
 await page.getByRole('button',{name:'Clear filters'}).click();
 await expect(page.getByRole('list',{name:'Loaded memories'})).toBeVisible();
});
test('canvas accepts keyboard selection, drag orbit, and Home reset',async({page})=>{
 test.setTimeout(20000);
 await page.goto('/explore/');
 const canvas=page.locator('canvas.graph-canvas');
 await expect(canvas).toHaveCount(1,{timeout:8000});
 const detail=page.getByRole('complementary',{name:'Selected memory'});

 // The keyboard is real browser input; selection is confirmed by the rendered detail panel.
 await canvas.press('ArrowRight');
 await expect(detail.locator('.detail-empty')).toHaveCount(0,{timeout:4000});
 await page.waitForTimeout(120);
 const initial=await renderedMap(page,canvas);
 expect((await renderedMap(page,canvas)).equals(initial)).toBe(true);

 const box=await canvas.boundingBox();
 if(!box) throw new Error('Explorer canvas has no rendered bounds');
 const x=box.x+box.width*.43, y=box.y+box.height*.48;
 await page.mouse.move(x,y);
 await page.mouse.down();
 await page.mouse.move(x+90,y+55,{steps:8});
 await page.mouse.up();
 await page.waitForTimeout(180);
 const orbited=await renderedMap(page,canvas);
 expect(orbited.equals(initial)).toBe(false);

 await canvas.press('Home');
 await page.waitForTimeout(180);
 const reset=await renderedMap(page,canvas);
 expect(reset.equals(initial)).toBe(true);
});
test('zoom input changes the rendered map',async({page},testInfo)=>{
 test.setTimeout(30000);
 await page.goto('/explore/');
 const canvas=page.locator('canvas.graph-canvas');
 await expect(canvas).toHaveCount(1);
 await canvas.scrollIntoViewIfNeeded();
 const initial=await renderedMap(page,canvas);
 const box=await canvas.boundingBox();
 const cx=box.x+box.width/2,cy=box.y+box.height/2;
 if(testInfo.project.use.hasTouch){
  const session=await page.context().newCDPSession(page);
  await session.send('Input.dispatchTouchEvent',{type:'touchStart',touchPoints:[
   {x:cx-35,y:cy,radiusX:5,radiusY:5,force:1,id:1},
   {x:cx+35,y:cy,radiusX:5,radiusY:5,force:1,id:2},
  ]});
  await session.send('Input.dispatchTouchEvent',{type:'touchMove',touchPoints:[
   {x:cx-75,y:cy,radiusX:5,radiusY:5,force:1,id:1},
   {x:cx+75,y:cy,radiusX:5,radiusY:5,force:1,id:2},
  ]});
  await session.send('Input.dispatchTouchEvent',{type:'touchEnd',touchPoints:[]});
  await session.detach();
 }else{
  await page.mouse.move(cx,cy);
  await page.mouse.wheel(0,-240);
 }
 await expect.poll(async()=>!(await renderedMap(page,canvas)).equals(initial),{timeout:4000}).toBe(true);
});
test('explorer works without WebGL and keeps synthetic data out of browser persistence',async({page})=>{
 await page.addInitScript(()=>{const original=HTMLCanvasElement.prototype.getContext;HTMLCanvasElement.prototype.getContext=function(type,...args){if(/webgl/i.test(type))return null;return original.call(this,type,...args);};});
 await page.goto('/explore/');
 await expect(page.getByRole('list',{name:'Loaded memories'})).toBeVisible();
 expect(await page.evaluate(()=>({local:localStorage.length,session:sessionStorage.length}))).toEqual({local:0,session:0});
});
test('repeated presentation switches retain one usable map',async({page})=>{
 // Twenty software-rendered map reconstructions on the leased one-worker
 // mobile profile need a separate bounded harness deadline. This does not
 // change the release latency/frame-time targets.
 test.setTimeout(90000);
 const warnings=[];
 page.on('console',message=>{if(/too many active WebGL contexts/i.test(message.text()))warnings.push(message.text());});
 await page.goto('/explore/');
 for(let i=0;i<20;i++){
  await expect(page.locator('canvas.graph-canvas')).toHaveCount(1);
  await page.getByRole('button',{name:'List',exact:true}).click();
  await expect(page.locator('canvas.graph-canvas')).toHaveCount(0);
  await page.getByRole('button',{name:'Map',exact:true}).click();
 }
 await expect(page.locator('canvas.graph-canvas')).toHaveCount(1);
 expect(warnings).toEqual([]);
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
 await page.getByRole('button',{name:'List',exact:true}).click();
 await page.getByRole('button',{name:/Synthetic owner record/}).click();
 await expect(page.getByText('Opening note…')).toBeVisible();
 await page.getByRole('combobox',{name:'Memory type'}).selectOption('fact');
 await expect(page.getByText('Opening note…')).not.toBeVisible();
 finishDetail();
 await expect(page.getByRole('complementary',{name:'Selected memory'}).getByRole('heading',{name:'Select a memory'})).toBeVisible();
});
