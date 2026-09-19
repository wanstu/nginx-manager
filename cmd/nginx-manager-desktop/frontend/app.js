const state = { items: [], selected: "" };

function api() {
  return window.go?.main?.App;
}
function $(id) { return document.getElementById(id); }

async function refresh() {
  const app = api();
  if (!app) return setTimeout(refresh, 100);
  state.items = await app.ListConnections();
  state.selected = await app.GetSelectedID();
  renderList();
  if (state.selected) loadEditor(state.selected);
  else clearEditor();
}

function renderList() {
  const root = $("connections");
  root.innerHTML = "";
  if (!state.items.length) {
    root.innerHTML = '<div class="empty">还没有 CLI 连接</div>';
    return;
  }
  for (const item of state.items) {
    const button = document.createElement("button");
    button.className = "connection" + (item.id === state.selected ? " active" : "");
    button.innerHTML = `<strong>${escapeHtml(item.name)}</strong><span>${escapeHtml(item.url)}</span>`;
    button.onclick = async () => {
      state.selected = item.id;
      await api().SelectConnection(item.id);
      renderList();
      loadEditor(item.id);
    };
    root.appendChild(button);
  }
}

function loadEditor(id) {
  const item = state.items.find(x => x.id === id);
  if (!item) return clearEditor();
  $("name").value = item.name;
  $("url").value = item.url;
  $("password").value = "";
  $("pageTitle").textContent = item.name;
  resetStatus();
}

function clearEditor() {
  state.selected = "";
  $("name").value = "";
  $("url").value = "";
  $("password").value = "";
  $("pageTitle").textContent = "添加连接";
  resetStatus();
}

function resetStatus() {
  $("statusBadge").textContent = "未连接";
  $("statusBadge").className = "badge";
  $("hostValue").textContent = "—";
  $("versionValue").textContent = "—";
  $("runtimeValue").textContent = "—";
  $("message").textContent = "保存后可测试 CLI 连接。";
}

$("newBtn").onclick = clearEditor;

$("saveBtn").onclick = async () => {
  try {
    const saved = await api().SaveConnection({
      id: state.selected,
      name: $("name").value,
      url: $("url").value,
      password: $("password").value
    });
    state.selected = saved.id;
    await api().SelectConnection(saved.id);
    $("password").value = "";
    await refresh();
    $("message").textContent = "连接已保存。密码已写入 Desktop Kit secureconfig。";
  } catch (err) {
    $("message").textContent = String(err);
  }
};

$("testBtn").onclick = async () => {
  if (!state.selected) return $("message").textContent = "请先保存连接。";
  $("statusBadge").textContent = "检测中";
  $("message").textContent = "正在连接 CLI…";
  try {
    const result = await api().TestConnection(state.selected);
    $("statusBadge").textContent = result.ok ? "正常" : "失败";
    $("statusBadge").className = "badge " + (result.ok ? "ok" : "bad");
    $("hostValue").textContent = result.hostname || "—";
    $("versionValue").textContent = result.version || "—";
    $("runtimeValue").textContent = result.runtime || "—";
    $("message").textContent = result.message || "";
  } catch (err) {
    $("statusBadge").textContent = "失败";
    $("statusBadge").className = "badge bad";
    $("message").textContent = String(err);
  }
};

$("deleteBtn").onclick = async () => {
  if (!state.selected) return;
  if (!confirm("删除这个 CLI 连接？保存的密码也会一起删除。")) return;
  await api().DeleteConnection(state.selected);
  state.selected = "";
  await refresh();
};

function escapeHtml(value) {
  return String(value).replace(/[&<>"']/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));
}

refresh();
