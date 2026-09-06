import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { access, mkdtemp, readFile, rm } from "node:fs/promises";
import { createServer } from "node:http";
import { tmpdir } from "node:os";
import { basename, dirname, extname, join, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "../dist");
const candidates = [process.argv[2], process.env.DAHUABRIDGE_BROWSER,
  "C:/Program Files/Google/Chrome/Application/chrome.exe",
  "C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe"].filter(Boolean);
let browser;
for (const candidate of candidates) {
  try { await access(candidate); browser = candidate; break; } catch { /* Try next installation. */ }
}
assert(browser, "Pass a Chromium executable path or set DAHUABRIDGE_BROWSER.");
await access(join(root, "dahuabridge-surveillance-panel.js"));
const requests = [];
const html = String.raw`<!doctype html><html><body><script type="module">
try {
  await import('/local/dahua/dahuabridge-surveillance-panel.js');
  const Card = customElements.get('dahuabridge-surveillance-panel');
  if (!Card || !customElements.get('dahuabridge-surveillance-tile')) throw Error('Cards not registered');
  const card = new Card(); card.setConfig(Card.getStubConfig()); document.body.append(card);
  await card.updateComplete;
  if (!card.shadowRoot.querySelector('ha-card')) throw Error('Card did not render');
  const before = await fetch('/requests').then(r => r.json());
  if (before.some(p => /chunks\/(?:hls-|dash\.|surveillance-panel-card-editor-)/.test(p))) throw Error('Eager optional module');
  const editor = await Card.getConfigElement();
  if (editor.localName !== 'dahuabridge-surveillance-panel-editor') throw Error('Editor not loaded');
  for (const kind of ['hls', 'dash']) {
    const player = document.createElement('dahuabridge-remote-stream');
    player.descriptor = {cacheKey:kind,alt:kind,fallbackImageUrl:null,sources:[{kind,url:location.origin+'/missing.'+(kind==='hls'?'m3u8':'mpd')}]};
    document.body.append(player);
    for (let attempt=0; attempt<100; attempt++) {
      const requested=await fetch('/requests').then(r=>r.json());
      if (requested.some(p=>p.includes('/chunks/'+(kind==='hls'?'hls-':'dash.')))) break;
      if(attempt===99) throw Error(kind+' chunk not requested');
      await new Promise(r=>setTimeout(r,25));
    }
    player.remove();
  }
  document.body.dataset.result='PASS';
} catch(error) { document.body.dataset.result='FAIL: '+error.message; }
</script></body></html>`;
const server = createServer(async (request, response) => {
  const path = new URL(request.url, "http://localhost").pathname;
  requests.push(path);
  if (path === "/") { response.setHeader("Content-Type", "text/html"); response.end(html); return; }
  if (path === "/requests") { response.setHeader("Content-Type", "application/json"); response.end(JSON.stringify(requests)); return; }
  const file = resolve(root, path.replace(/^\/local\/dahua\//, ""));
  if (!path.startsWith("/local/dahua/") || !file.startsWith(root + sep)) { response.writeHead(404).end(); return; }
  try {
    response.setHeader("Content-Type", extname(file) === ".js" ? "text/javascript" : "image/png");
    response.end(await readFile(file));
  } catch { response.writeHead(404).end(); }
});
await new Promise(resolve => server.listen(0, "127.0.0.1", resolve));
const temporaryRoot = resolve(tmpdir());
const profile = await mkdtemp(join(temporaryRoot, "dahuabridge-browser-"));
try {
  const address = server.address();
  const child = spawn(browser, ["--headless=new", "--disable-gpu", "--in-process-gpu", "--disable-background-networking", "--disable-component-update", "--disable-sync", "--no-first-run", "--no-default-browser-check", `--user-data-dir=${profile}`, "--dump-dom", "--virtual-time-budget=10000", `http://127.0.0.1:${address.port}/`], { windowsHide: true });
  let output = "";
  let errors = "";
  child.stdout.on("data", chunk => { output += chunk; });
  child.stderr.on("data", chunk => { errors += chunk; });
  const timeout = setTimeout(() => child.kill(), 30000);
  try {
    const code = await new Promise((resolve, reject) => { child.on("error", reject); child.on("close", resolve); });
    assert.equal(code, 0, `${errors.slice(-1000)}\nRequests: ${JSON.stringify(requests)}`);
    assert.match(output, /data-result="PASS"/, output.slice(-2500));
    console.log("Browser smoke passed: cards render; editor, HLS and DASH load on demand.");
  } finally { clearTimeout(timeout); }
} finally {
  server.closeAllConnections();
  await new Promise(resolve => server.close(resolve));
  // The helper may remove only its own mkdtemp profile under the OS temp directory.
  assert.equal(dirname(resolve(profile)), temporaryRoot);
  assert(basename(profile).startsWith("dahuabridge-browser-"));
  await rm(profile, { recursive: true, force: true, maxRetries: 3, retryDelay: 200 });
}
