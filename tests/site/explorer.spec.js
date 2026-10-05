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
