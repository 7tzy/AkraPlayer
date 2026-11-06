// Akra Player browser extension — background service worker.
//
// Akra Player (the Windows app) owns the global keys. When you press its
// "Play / pause YouTube" key, or its "Play / pause window" key with a browser
// chosen, it sends a command here through Chrome's native messaging.
//   toggle-youtube : pause whatever YouTube is playing; press again to resume it
//   toggle-media   : the same for any tab with a <video> or <audio>

const HOST = "com.akra.player";
let port = null;
let connected = false; // the Windows app is running and linked

// ---------- functions injected into pages ----------

function pageGetState() {
  const media = [...document.querySelectorAll("video, audio")];
  const m = media.find(x => !x.paused) || media.find(x => x.readyState > 0);
  if (!m) return null;
  const isYT = location.hostname.endsWith("youtube.com");
  let title = document.title.replace(/^\(\d+\)\s*/, "").replace(/ - YouTube$/, "").trim();
  if (isYT) {
    const h1 = document.querySelector("h1.ytd-watch-metadata yt-formatted-string, h1.title");
    if (h1 && h1.textContent.trim()) title = h1.textContent.trim();
  }
  return { paused: m.paused, title, currentTime: m.currentTime || 0, duration: isFinite(m.duration) ? m.duration : 0 };
}

function pageControl(action) {
  const media = [...document.querySelectorAll("video, audio")];
  const m = media.find(x => !x.paused) || media.find(x => x.readyState > 0);
  if (!m) return false;
  if (action === "play") { const p = m.play(); if (p && p.catch) p.catch(() => {}); }
  else if (action === "pause") m.pause();
  return true;
}

// ---------- helpers ----------

async function runInTab(tabId, func, args = []) {
  try {
    const [res] = await chrome.scripting.executeScript({ target: { tabId }, func, args, world: "MAIN" });
    return res ? res.result : null;
  } catch (e) {
    return null; // tab closed, discarded, or a page we can't touch
  }
}

async function mediaTabs(youtubeOnly) {
  const query = youtubeOnly ? { url: ["*://*.youtube.com/*"] } : {};
  let tabs = await chrome.tabs.query(query);
  if (!youtubeOnly) tabs = tabs.filter(t => /^https?:/.test(t.url || ""));
  const states = await Promise.all(tabs.map(async t => ({ tab: t, state: await runInTab(t.id, pageGetState) })));
  return states.filter(s => s.state);
}

async function getLastPaused(kind) {
  const key = "lastPaused_" + kind;
  const got = await chrome.storage.session.get(key);
  return got[key] || [];
}

async function setLastPaused(kind, ids) {
  await chrome.storage.session.set({ ["lastPaused_" + kind]: ids });
}

// toggle: pause everything playing; otherwise resume what we paused last,
// or the most recently used tab.
async function toggle(kind) {
  const states = await mediaTabs(kind === "youtube");
  if (states.length === 0) return;
  const playing = states.filter(s => !s.state.paused);
  if (playing.length > 0) {
    await Promise.all(playing.map(s => runInTab(s.tab.id, pageControl, ["pause"])));
    await setLastPaused(kind, playing.map(s => s.tab.id));
    return;
  }
  const open = new Set(states.map(s => s.tab.id));
  const resume = (await getLastPaused(kind)).filter(id => open.has(id));
  if (resume.length > 0) {
    await Promise.all(resume.map(id => runInTab(id, pageControl, ["play"])));
    return;
  }
  const recent = [...states].sort((a, b) => (b.tab.lastAccessed || 0) - (a.tab.lastAccessed || 0))[0];
  if (recent) await runInTab(recent.tab.id, pageControl, ["play"]);
}

// ---------- link to the Windows app ----------

function connect() {
  if (port) return;
  try {
    port = chrome.runtime.connectNative(HOST);
  } catch (e) {
    port = null;
    return;
  }
  port.onMessage.addListener(msg => {
    switch (msg && msg.cmd) {
      case "toggle-youtube": toggle("youtube"); break;
      case "toggle-media": toggle("media"); break;
      case "connected": connected = true; break;
      case "disconnected": connected = false; break;
    }
  });
  port.onDisconnect.addListener(() => {
    void chrome.runtime.lastError; // e.g. Akra Player isn't installed yet
    port = null;
    connected = false;
  });
}

chrome.runtime.onStartup.addListener(connect);
chrome.runtime.onInstalled.addListener(connect);
chrome.alarms.create("akra-reconnect", { periodInMinutes: 0.5 });
chrome.alarms.onAlarm.addListener(a => { if (a.name === "akra-reconnect") connect(); });
connect();

// ---------- popup ----------

chrome.runtime.onMessage.addListener((msg, sender, reply) => {
  (async () => {
    if (msg.type === "state") {
      connect();
      const states = await mediaTabs(true);
      const playing = states.find(s => !s.state.paused);
      const top = playing || states[0];
      reply({
        linked: !!port && connected,
        hostRunning: !!port,
        video: top ? { title: top.state.title, paused: top.state.paused, tabId: top.tab.id, windowId: top.tab.windowId } : null
      });
    } else if (msg.type === "toggle") {
      await toggle("youtube");
      reply({ ok: true });
    } else if (msg.type === "focus" && msg.tabId) {
      try {
        await chrome.tabs.update(msg.tabId, { active: true });
        if (msg.windowId) await chrome.windows.update(msg.windowId, { focused: true });
      } catch (e) {}
      reply({ ok: true });
    }
  })();
  return true;
});
