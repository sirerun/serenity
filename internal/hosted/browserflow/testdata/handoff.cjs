const { chromium } = require(process.env.SERENITY_PLAYWRIGHT);
(async () => {
const browser = await chromium.launch({headless:true,executablePath:process.env.SERENITY_CHROMIUM});
try {
const context = await browser.newContext();
await context.addCookies([{name:'serenity_session',value:'s'.repeat(43),url:process.argv[2]}]);
const page = await context.newPage();
let blocked = false;
page.on('console',m=>{if(m.text().includes('form-action'))blocked=true;});
await page.goto(process.argv[2]);
await page.locator('button[value="'+process.argv[3]+'"]').click();
try {await page.waitForURL(process.argv[4]+'/complete',{timeout:5000});}
catch {throw new Error('Final application navigation failed; form-action violation='+blocked);}
console.log('One-click same-tab callback chain completed');
}finally{await browser.close();}
})().catch(e=>{console.error(e.message);process.exitCode=1;});
