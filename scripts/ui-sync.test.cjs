const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');
const assert = require('node:assert/strict');
const root = process.argv[2];
const script = file => [...fs.readFileSync(path.join(root, file), 'utf8').matchAll(/<script\b[^>]*>([\s\S]*?)<\/script>/g)].at(-1)[1];
const flush = () => new Promise(resolve => setImmediate(resolve));
function harness(file, api = {}) {
  const nodes = new Map(), timers = new Map(), listeners = {};
  let seq = 0, now = 100;
  const document = { getElementById(id) {
    if (!nodes.has(id)) nodes.set(id, { textContent: '—', style: {}, checked: false, value: '', classList: { add() {}, remove() {} } });
    return nodes.get(id);
  }};
  const sockets = [];
  class Socket { static OPEN = 1; constructor() { this.readyState = 1; sockets.push(this); } close() {} send() {} }
  const context = { document, console, performance: { now: () => now }, WebSocket: Socket,
    window: { addEventListener(name, fn) { listeners[name] = fn; } }, chzzkApi: api,
    setTimeout(fn, ms) { timers.set(++seq, { fn, ms }); return seq; }, clearTimeout(id) { timers.delete(id); }
  };
  vm.createContext(context); vm.runInContext(script(file), context);
  const messages = []; context.showToast = (...args) => messages.push(args);
  return { context, nodes, timers, listeners, sockets, messages,
    el: id => document.getElementById(id), eval: code => vm.runInContext(code, context), advance(ms) { now += ms; },
    fire(ms) { const found = [...timers].find(([, t]) => t.ms === ms); assert(found, `timer ${ms}`); timers.delete(found[0]); found[1].fn(); },
    count(ms) { return [...timers.values()].filter(t => t.ms === ms).length; }
  };
}
const ok = data => ({ ok: true, status: 200, json: async () => data });
const fail = status => ({ ok: false, status, json: async () => ({}) });

async function run() {
  let status = 502, serverEmoji = false, reads = 0;
  const api = {
    getChatMode: async () => { reads++; return status === 200 ? ok({emojiOnly: serverEmoji, slowSec: 5}) : fail(status); },
    getDonations: async () => status === 200 ? ok({chat:true}) : fail(status),
    setEmojiMode: async enabled => { serverEmoji = enabled; return ok({emojiOnly:enabled}); }
  };
  const mini = harness('ui/mini-dock/index.html', api);
  await mini.context.fetchAllStatus(); assert.equal(mini.count(10000),1);
  status=200; await mini.context.updateEmojiMode(true); await flush();
  assert.equal(mini.count(10000),0); assert.equal(mini.el('toggle-emoji').checked,true);
  console.log('PASS: user change cancels retry, refresh succeeds and stays manual');
  let oldResolve;
  api.getChatMode=()=>new Promise(resolve=>{oldResolve=resolve});
  const oldRead=mini.context.fetchAllStatus(true);
  api.getChatMode=async()=>ok({emojiOnly:serverEmoji});
  await mini.context.updateEmojiMode(false); await flush();
  oldResolve(ok({emojiOnly:true})); await oldRead;
  assert.equal(serverEmoji,false); assert.equal(mini.el('toggle-emoji').checked,false);
  console.log('PASS: old in-flight read cannot overwrite successful user change');
  status=502; api.getChatMode=async()=>fail(status);
  await mini.context.updateEmojiMode(true); await flush(); assert.equal(mini.count(10000),1);
  mini.fire(10000); await flush(); assert.equal(mini.count(10000),1);
  status=200;api.getChatMode=async()=>ok({emojiOnly:serverEmoji});
  mini.fire(10000); await flush(); assert.equal(mini.count(10000),0);
  console.log('PASS: failed post-change refresh retries every 10 seconds, success stops');
  let diskCalls=0, diskFails=true;
  const stats=harness('ui/stats-dock/index.html',{getDiskTotal:async()=>{
    diskCalls++; if(diskFails)throw Error('temporary'); return ok({totalBytes:1000*1024*1024});
  }});
  stats.context.fetchTotalDiskSpace('C:/record'); await flush();
  diskFails=false; stats.context.fetchTotalDiskSpace('C:/record'); await flush();
  assert.equal(diskCalls,2); assert.equal(stats.eval('cachedTotalDiskBytes'),1000*1024*1024);
  stats.context.renderStats({availableDiskSpace:400});
  assert.equal(stats.el('label-disk-pct').textContent,'60.0% 사용 중');
  stats.context.fetchTotalDiskSpace('C:/record');await flush();assert.equal(diskCalls,2);
  console.log('PASS: same-drive failure can retry, success is cached, usage is correct');
  let resetCalls = 0, resetFails = true;
  const reset = harness('ui/stats-dock/index.html', { getDiskTotal: async () => {
    resetCalls++;
    if (resetFails) throw Error('temporary');
    return ok({ totalBytes: 1000 * 1024 * 1024 });
  }});
  reset.context.updateRecordDirectoryUI('C:/record'); await flush();
  resetFails = false;
  reset.context.resetStatsAction(); await flush();
  reset.context.renderStats({ availableDiskSpace: 400 });
  assert.equal(resetCalls, 2, 'reset button must retry failed disk-total request');
  assert.equal(reset.el('label-disk-pct').textContent, '60.0% 사용 중');
  console.log('PASS: reset button recovers failed storage lookup without page reload');
  let autoCalls = 0;
  const auto = harness('ui/stats-dock/index.html', { getDiskTotal: async () => {
    if (++autoCalls === 1) throw Error('temporary');
    return ok({ totalBytes: 1000 * 1024 * 1024 });
  }});
  auto.context.updateRecordDirectoryUI('C:/record'); await flush();
  auto.context.renderStats({ availableDiskSpace: 400 });
  assert.equal(autoCalls, 1);
  auto.advance(10000); auto.context.renderStats({ availableDiskSpace: 400 }); await flush();
  assert.equal(autoCalls, 2);
  assert.equal(auto.el('label-disk-pct').textContent, '60.0% 사용 중');
  auto.context.renderStats({ availableDiskSpace: 300 });
  assert.equal(auto.el('label-disk-pct').textContent, '70.0% 사용 중');
  console.log('PASS: failed total recovers automatically and subsequent OBS values update usage');
  let resolveTotal;
  const order=harness('ui/stats-dock/index.html',{getDiskTotal:()=>new Promise(r=>{resolveTotal=r})});
  order.context.fetchTotalDiskSpace('D:/record'); order.context.fetchTotalDiskSpace('D:/record');
  order.context.renderStats({availableDiskSpace:400});assert.equal(order.el('label-disk-pct').textContent,'—');
  resolveTotal(ok({totalBytes:1000*1024*1024}));await flush();assert.equal(order.el('label-disk-pct').textContent,'60.0% 사용 중');
  console.log('PASS: pending total blocks calculation; free-first response order works');
}
run().catch(error=>{console.error(error);process.exitCode=1});
