import { createRequire } from 'node:module';
import { join, resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
const requireFrom = createRequire(pathToFileURL(join(resolve(process.env.BROWSER_INSPECT_MODULES), 'package.json')));
const loaded = await import(pathToFileURL(requireFrom.resolve('puppeteer-core')).href);
const puppeteer = [loaded.default?.default, loaded.default, loaded].find(i => typeof i?.launch === 'function');
const browser = await puppeteer.launch({ executablePath: 'C:/Program Files/Google/Chrome/Application/chrome.exe', headless: true, userDataDir: process.env.PROF });
const html = 'data:text/html,<script>window.ev=[];document.addEventListener("visibilitychange",()=>ev.push(document.visibilityState+"@"+Date.now()))</script>x';
const p1 = await browser.newPage(); await p1.goto(html);
const st = async (l) => console.log(l, await p1.evaluate(() => [document.visibilityState, document.hasFocus(), JSON.stringify(window.ev)]));
await st('p1 alone');
const p2 = await browser.newPage(); await p2.goto('about:blank'); await p2.bringToFront();
await new Promise(r => setTimeout(r, 500)); await st('after p2 bringToFront');
await p1.bringToFront(); await new Promise(r => setTimeout(r, 500)); await st('after p1 bringToFront');
const s = await p1.createCDPSession();
const { windowId } = await s.send('Browser.getWindowForTarget');
console.log('window', windowId, JSON.stringify(await s.send('Browser.getWindowBounds', { windowId })));
try { await s.send('Browser.setWindowBounds', { windowId, bounds: { windowState: 'minimized' } }); await new Promise(r => setTimeout(r, 500)); await st('minimized'); } catch (e) { console.log('min err', e.message); }
try { await s.send('Browser.setWindowBounds', { windowId, bounds: { windowState: 'normal' } }); await new Promise(r => setTimeout(r, 500)); await st('normal'); } catch (e) { console.log('norm err', e.message); }
const s2 = await p2.createCDPSession();
console.log('p2 window', JSON.stringify(await s2.send('Browser.getWindowForTarget')));
await browser.close();
