const $ = id => document.getElementById(id);
let state = null;

async function refresh() {
  state = await chrome.runtime.sendMessage({ type: "state" });
  const link = $("link");
  if (state.linked) {
    link.textContent = "Linked to Akra Player";
    link.className = "ok";
  } else if (state.hostRunning) {
    link.textContent = "Waiting for Akra Player to open…";
    link.className = "warn";
  } else {
    link.textContent = "Open Akra Player once to link it";
    link.className = "warn";
  }
  const v = state.video;
  $("title").textContent = v ? (v.paused ? "Paused · " : "Playing · ") + v.title : "Nothing playing";
  $("toggle").disabled = !v;
  $("open").disabled = !v;
}

$("toggle").onclick = async () => { await chrome.runtime.sendMessage({ type: "toggle" }); setTimeout(refresh, 150); };
$("open").onclick = () => state && state.video &&
  chrome.runtime.sendMessage({ type: "focus", tabId: state.video.tabId, windowId: state.video.windowId });

refresh();
setInterval(refresh, 1000);
