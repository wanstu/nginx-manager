const state = {
  items: [],
  selected: "",
  view: "overview",
  sites: [],
  layout: null,
  snapshots: [],
  certificates: [],
  certbot: null,
  logs: [],
  editingSiteID: ""
};

function api() {
  return window.go?.main?.App;
}

function $(id) {
  return document.getElementById(id);
}

async function refresh() {
  const app = api();
  if (!app) return setTimeout(refresh, 100);

  state.items = await app.ListConnections();
  state.selected = await app.GetSelectedID();
  renderConnections();

  if (state.selected) {
    loadEditor(state.selected);
    if (state.view === "sites") await loadSites();
    if (state.view === "snapshots") await loadSnapshots();
    if (state.view === "certificates") await loadCertificates();
    if (state.view === "logs") await loadLogs();
  } else {
    clearEditor(false);
  }
}

function renderConnections() {
  const root = $("connections");
  root.innerHTML = "";

  if (!state.items.length) {
    root.innerHTML = '<div class="empty">还没有 CLI 连接</div>';
    return;
  }

  for (const item of state.items) {
    const button = document.createElement("button");
    button.className = "connection" + (item.id === state.selected ? " active" : "");
    button.innerHTML =
      "<strong>" + escapeHtml(item.name) + "</strong>" +
      "<span>" + escapeHtml(item.url) + "</span>";

    button.onclick = async () => {
      state.selected = item.id;
      await api().SelectConnection(item.id);
      renderConnections();
      loadEditor(item.id);
      resetProxyEditor();
      if (state.view === "sites") await loadSites();
      if (state.view === "snapshots") await loadSnapshots();
      if (state.view === "certificates") await loadCertificates();
      if (state.view === "logs") await loadLogs();
    };
    root.appendChild(button);
  }
}

function loadEditor(id) {
  const item = state.items.find((x) => x.id === id);
  if (!item) return clearEditor(false);

  $("name").value = item.name;
  $("url").value = item.url;
  $("password").value = "";
  resetStatus();
  updateHeader();
}

function clearEditor(resetSelection = true) {
  if (resetSelection) state.selected = "";
  $("name").value = "";
  $("url").value = "";
  $("password").value = "";
  resetStatus();
  resetProxyEditor();
  updateHeader();
}

function resetStatus() {
  $("statusBadge").textContent = state.selected ? "未检测" : "未连接";
  $("statusBadge").className = "badge";
  $("hostValue").textContent = "—";
  $("versionValue").textContent = "—";
  $("runtimeValue").textContent = "—";
  $("privilegeValue").textContent = "—";
  $("message").textContent = state.selected ? "可测试当前 CLI 连接。" : "选择或新增一个 CLI 连接。";
}

function updateHeader() {
  const item = state.items.find((x) => x.id === state.selected);
  if (state.view === "sites") {
    $("pageTitle").textContent = item ? item.name + " · 站点" : "站点";
    $("pageSubtitle").textContent = "读取和管理当前 CLI 所在服务器的 Nginx / OpenResty 站点。";
  } else if (state.view === "snapshots") {
    $("pageTitle").textContent = item ? item.name + " · 快照" : "快照";
    $("pageSubtitle").textContent = "查看 Manager 变更快照，并通过事务恢复历史配置。";
  } else if (state.view === "certificates") {
    $("pageTitle").textContent = item ? item.name + " · HTTPS" : "HTTPS / 证书";
    $("pageSubtitle").textContent = "管理 Manager 站点的 ACME 证书与 HTTPS。";
  } else if (state.view === "logs") {
    $("pageTitle").textContent = item ? item.name + " · 日志" : "访问与错误日志";
    $("pageSubtitle").textContent = "安全读取当前 Nginx / OpenResty 的日志尾部内容。";
  } else {
    $("pageTitle").textContent = item ? item.name : "连接管理";
    $("pageSubtitle").textContent = "保存多个 CLI Endpoint，并切换当前服务器。";
  }
}

function setView(view) {
  state.view = view;
  document.querySelectorAll(".nav-item").forEach((item) => {
    item.classList.toggle("active", item.dataset.view === view);
  });
  $("overviewView").classList.toggle("hidden", view !== "overview");
  $("sitesView").classList.toggle("hidden", view !== "sites");
  $("snapshotsView").classList.toggle("hidden", view !== "snapshots");
  $("certificatesView").classList.toggle("hidden", view !== "certificates");
  $("logsView").classList.toggle("hidden", view !== "logs");
  updateHeader();
  if (view === "sites") loadSites();
  if (view === "snapshots") loadSnapshots();
  if (view === "certificates") loadCertificates();
  if (view === "logs") loadLogs();
}

document.querySelectorAll(".nav-item").forEach((item) => {
  item.onclick = () => setView(item.dataset.view);
});

$("newBtn").onclick = () => {
  setView("overview");
  clearEditor();
};

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
    $("message").textContent = cleanError(err);
  }
};

$("testBtn").onclick = async () => {
  if (!state.selected) {
    $("message").textContent = "请先保存连接。";
    return;
  }

  $("statusBadge").textContent = "检测中";
  $("message").textContent = "正在连接 CLI…";

  try {
    const result = await api().TestConnection(state.selected);
    $("statusBadge").textContent = result.ok ? "正常" : "失败";
    $("statusBadge").className = "badge " + (result.ok ? "ok" : "bad");
    $("hostValue").textContent = result.hostname || "—";
    $("versionValue").textContent = result.version || "—";
    $("runtimeValue").textContent = result.runtime || "—";
    $("privilegeValue").textContent = result.privilege_ready ? "正常" : "未就绪";
    $("message").textContent = (result.message || "") + (result.privilege_message ? "\n管理权限：" + result.privilege_message : "");
  } catch (err) {
    $("statusBadge").textContent = "失败";
    $("statusBadge").className = "badge bad";
    $("message").textContent = cleanError(err);
  }
};

$("deleteBtn").onclick = async () => {
  if (!state.selected) return;
  if (!confirm("删除这个 CLI 连接？保存的密码也会一起删除。")) return;

  await api().DeleteConnection(state.selected);
  state.selected = "";
  state.sites = [];
  state.snapshots = [];
  state.certificates = [];
  state.certbot = null;
  state.logs = [];
  await refresh();
};

$("refreshSitesBtn").onclick = loadSites;

async function loadSites() {
  if (!state.selected) {
    state.sites = [];
    state.layout = null;
    renderSites();
    $("siteMessage").textContent = "请先选择一个 CLI 连接。";
    return;
  }

  $("sitesList").innerHTML = '<div class="empty large">正在读取站点…</div>';
  try {
    const result = await api().ListSites(state.selected);
    state.sites = result.sites || [];
    state.layout = result.layout || null;
    renderSites();
    $("siteMessage").textContent = "已读取 " + state.sites.length + " 个站点。";
  } catch (err) {
    state.sites = [];
    state.layout = null;
    renderSites();
    $("siteMessage").textContent = cleanError(err);
  }
}

function renderSites() {
  const root = $("sitesList");
  root.innerHTML = "";

  if (state.layout) {
    $("layoutValue").textContent =
      state.layout.mode + " · " + (state.layout.available_dir || state.layout.main_config || "");
  } else {
    $("layoutValue").textContent = "连接 CLI 后读取服务器配置。";
  }

  if (!state.sites.length) {
    root.innerHTML = '<div class="empty large">没有发现站点配置</div>';
    return;
  }

  for (const site of state.sites) {
    const item = document.createElement("div");
    item.className = "site-row";
    const title = site.server_name || site.id;
    const proxy = site.proxy_pass || "非反向代理 / 未识别";
    item.innerHTML =
      '<div class="site-main">' +
        '<strong>' + escapeHtml(title) + '</strong>' +
        '<span>' + escapeHtml(proxy) + '</span>' +
      '</div>' +
      '<div class="site-side">' +
        '<div class="site-tags">' +
          '<span class="mini-badge ' + (site.enabled ? "ok" : "") + '">' + (site.enabled ? "已启用" : "未启用") + '</span>' +
          '<span class="mini-badge ' + (site.managed ? "managed" : "") + '">' + (site.managed ? "Manager 管理" : "外部配置") + '</span>' +
          (site.https ? '<span class="mini-badge https">HTTPS' + (site.redirect_https ? ' · 强制' : '') + '</span>' : '') +
        '</div>' +
      '</div>';

    if (site.managed) {
      const actions = document.createElement("div");
      actions.className = "site-actions";

      if (site.proxy_pass) {
        const edit = document.createElement("button");
        edit.textContent = "编辑";
        edit.onclick = () => {
          state.editingSiteID = site.id;
          $("proxyFormTitle").textContent = "编辑反向代理";
          $("createProxyBtn").textContent = "保存修改";
          $("cancelEditProxyBtn").classList.remove("hidden");
          $("proxyServerName").value = site.server_name || "";
          $("proxyUpstream").value = site.proxy_pass || "";
          $("proxyWebSocket").checked = Boolean(site.websocket);
          $("siteMessage").textContent = "正在编辑：" + title;
          $("proxyServerName").focus();
        };
        actions.appendChild(edit);
      }

      const toggle = document.createElement("button");
      toggle.textContent = site.enabled ? "停用" : "启用";
      toggle.onclick = async () => {
        toggle.disabled = true;
        $("siteMessage").textContent = "正在" + (site.enabled ? "停用" : "启用") + " " + title + "…";
        try {
          await api().SetSiteEnabled(state.selected, site.id, !site.enabled);
          await loadSites();
        } catch (err) {
          $("siteMessage").textContent = cleanError(err);
        } finally {
          toggle.disabled = false;
        }
      };

      const remove = document.createElement("button");
      remove.className = "danger";
      remove.textContent = "删除";
      remove.onclick = async () => {
        if (!confirm("删除 Manager 管理的站点 “" + title + "”？删除前会保存快照。")) return;
        remove.disabled = true;
        $("siteMessage").textContent = "正在删除 " + title + "…";
        try {
          await api().DeleteSite(state.selected, site.id);
          await loadSites();
        } catch (err) {
          $("siteMessage").textContent = cleanError(err);
        } finally {
          remove.disabled = false;
        }
      };

      actions.appendChild(toggle);
      actions.appendChild(remove);
      item.querySelector(".site-side").appendChild(actions);
    }

    root.appendChild(item);
  }
}

$("refreshSnapshotsBtn").onclick = loadSnapshots;

async function loadSnapshots() {
  if (!state.selected) {
    state.snapshots = [];
    renderSnapshots();
    $("snapshotMessage").textContent = "请先选择一个 CLI 连接。";
    return;
  }

  $("snapshotsList").innerHTML = '<div class="empty large">正在读取快照…</div>';
  try {
    state.snapshots = await api().ListSnapshots(state.selected);
    state.snapshots = state.snapshots || [];
    renderSnapshots();
    $("snapshotMessage").textContent = "已读取最近 " + state.snapshots.length + " 个快照。";
  } catch (err) {
    state.snapshots = [];
    renderSnapshots();
    $("snapshotMessage").textContent = cleanError(err);
  }
}

function renderSnapshots() {
  const root = $("snapshotsList");
  root.innerHTML = "";

  if (!state.snapshots.length) {
    root.innerHTML = '<div class="empty large">还没有配置快照</div>';
    return;
  }

  for (const snapshot of state.snapshots) {
    const item = document.createElement("div");
    item.className = "snapshot-row";

    const operation = formatSnapshotOperation(snapshot.operation);
    const time = formatSnapshotTime(snapshot.created_at);
    item.innerHTML =
      '<div class="snapshot-main">' +
        '<strong>' + escapeHtml(snapshot.site_id) + '</strong>' +
        '<span>' + escapeHtml(operation + " · " + time) + '</span>' +
      '</div>' +
      '<div class="snapshot-side">' +
        '<span class="mini-badge ' + (snapshot.enabled ? "ok" : "") + '">' +
          (snapshot.enabled ? "快照时已启用" : "快照时未启用") +
        '</span>' +
      '</div>';

    const restore = document.createElement("button");
    restore.textContent = "恢复";
    restore.onclick = async () => {
      if (!confirm("恢复此快照？当前 Manager 配置会先自动创建一个“恢复前”快照。")) return;
      restore.disabled = true;
      $("snapshotMessage").textContent = "正在验证并恢复 " + snapshot.site_id + "…";
      try {
        const site = await api().RestoreSnapshot(state.selected, snapshot.id);
        $("snapshotMessage").textContent = "恢复成功：" + (site.server_name || site.id);
        await loadSnapshots();
      } catch (err) {
        $("snapshotMessage").textContent = cleanError(err);
      } finally {
        restore.disabled = false;
      }
    };

    item.querySelector(".snapshot-side").appendChild(restore);
    root.appendChild(item);
  }
}

function formatSnapshotOperation(value) {
  if (value === "delete") return "删除前";
  if (value === "set_enabled") return "启停前";
  if (value === "restore_before") return "恢复前";
  if (value === "update") return "编辑前";
  return value || "未知操作";
}

function formatSnapshotTime(value) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value || "未知时间";
  return date.toLocaleString("zh-CN", { hour12: false });
}

$("refreshCertificatesBtn").onclick = loadCertificates;

async function loadCertificates() {
  if (!state.selected) {
    state.certificates = [];
    state.certbot = null;
    renderCertificates();
    renderCertificateSites();
    $("certificateMessage").textContent = "请先选择一个 CLI 连接。";
    return;
  }

  $("certificatesList").innerHTML = '<div class="empty large">正在读取证书…</div>';
  try {
    const certificateResult = await api().ListCertificates(state.selected);
    const siteResult = await api().ListSites(state.selected);
    state.certificates = certificateResult.certificates || [];
    state.certbot = certificateResult.certbot || null;
    state.sites = siteResult.sites || [];
    renderCertificates();
    renderCertificateSites();
    $("certificateMessage").textContent = "已读取 " + state.certificates.length + " 张证书。";
  } catch (err) {
    state.certificates = [];
    state.certbot = null;
    renderCertificates();
    renderCertificateSites();
    $("certificateMessage").textContent = cleanError(err);
  }
}

function renderCertificates() {
  const root = $("certificatesList");
  root.innerHTML = "";

  const certbotReady = Boolean(state.certbot?.available);
  $("issueCertificateBtn").disabled = !certbotReady;
  $("renewCertificatesBtn").disabled = !certbotReady;
  if (certbotReady) {
    $("certbotValue").textContent = "Certbot 已就绪 · " + (state.certbot.version || state.certbot.path || "可用");
  } else {
    $("certbotValue").textContent = "Certbot 未安装或不在 PATH 中；签发和续期功能不可用。";
  }

  if (!state.certificates.length) {
    root.innerHTML = '<div class="empty large">没有发现 Let’s Encrypt / Certbot 证书</div>';
    return;
  }

  for (const certificate of state.certificates) {
    const item = document.createElement("div");
    item.className = "certificate-row";

    const domains = (certificate.domains || []).join(", ") || certificate.name;
    const expiry = certificateExpiry(certificate.not_after);
    item.innerHTML =
      '<div class="certificate-main">' +
        '<strong>' + escapeHtml(certificate.name) + '</strong>' +
        '<span>' + escapeHtml(domains) + '</span>' +
      '</div>' +
      '<div class="certificate-side">' +
        '<span class="mini-badge ' + (expiry.days >= 30 ? "ok" : "warn") + '">' +
          escapeHtml(expiry.label) +
        '</span>' +
        '<span class="certificate-date">' + escapeHtml(formatSnapshotTime(certificate.not_after)) + '</span>' +
      '</div>';
    root.appendChild(item);
  }
}

function renderCertificateSites() {
  const select = $("certificateSite");
  select.innerHTML = "";

  const candidates = state.sites.filter((site) =>
    site.managed && site.enabled && site.proxy_pass && site.server_name && site.server_name !== "_"
  );

  if (!candidates.length) {
    const option = document.createElement("option");
    option.value = "";
    option.textContent = "没有可签发 HTTPS 的 Manager 站点";
    select.appendChild(option);
    return;
  }

  for (const site of candidates) {
    const option = document.createElement("option");
    option.value = site.id;
    option.textContent =
      site.server_name + (site.https ? " · 已启用 HTTPS" : " · HTTP");
    select.appendChild(option);
  }
}

function certificateExpiry(value) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return { days: -1, label: "到期时间未知" };
  const days = Math.ceil((date.getTime() - Date.now()) / 86400000);
  if (days < 0) return { days, label: "已过期 " + Math.abs(days) + " 天" };
  return { days, label: days + " 天后到期" };
}

$("issueCertificateBtn").onclick = async () => {
  if (!state.selected) {
    $("certificateMessage").textContent = "请先选择一个 CLI 连接。";
    return;
  }
  const siteID = $("certificateSite").value;
  const email = $("certificateEmail").value.trim();
  if (!siteID) {
    $("certificateMessage").textContent = "请选择一个 Manager 站点。";
    return;
  }
  if (!email) {
    $("certificateMessage").textContent = "请输入 ACME 联系邮箱。";
    return;
  }

  $("issueCertificateBtn").disabled = true;
  $("certificateMessage").textContent = "正在准备 HTTP-01 Challenge、签发证书并应用 HTTPS…";
  try {
    const site = await api().IssueCertificate(state.selected, siteID, {
      email: email,
      redirect_https: $("redirectHTTPS").checked
    });
    $("certificateMessage").textContent = "HTTPS 已启用：" + (site.server_name || site.id);
    await loadCertificates();
  } catch (err) {
    $("certificateMessage").textContent = cleanError(err);
  } finally {
    $("issueCertificateBtn").disabled = false;
  }
};

$("renewCertificatesBtn").onclick = async () => {
  if (!state.selected) {
    $("certificateMessage").textContent = "请先选择一个 CLI 连接。";
    return;
  }
  $("renewCertificatesBtn").disabled = true;
  $("certificateMessage").textContent = "正在执行 Certbot 续期检查…";
  try {
    const result = await api().RenewCertificates(state.selected);
    $("certificateMessage").textContent = "续期检查完成。" + (result.output ? "\n" + result.output : "");
    await loadCertificates();
  } catch (err) {
    $("certificateMessage").textContent = cleanError(err);
  } finally {
    $("renewCertificatesBtn").disabled = false;
  }
};


$("refreshLogsBtn").onclick = loadLogs;
$("refreshLogContentBtn").onclick = loadLogTail;
$("logFileSelect").onchange = loadLogTail;
$("logLines").onchange = loadLogTail;

async function loadLogs() {
  if (!state.selected) {
    state.logs = [];
    renderLogOptions();
    $("logContent").textContent = "请先选择一个 CLI 连接。";
    $("logMeta").textContent = "尚未读取日志。";
    return;
  }

  $("logMessage").textContent = "正在扫描当前 Nginx 配置中的日志…";
  try {
    state.logs = await api().ListLogs(state.selected);
    state.logs = state.logs || [];
    renderLogOptions();
    if (state.logs.length) {
      await loadLogTail();
    } else {
      $("logContent").textContent = "没有发现可读取的 Nginx 日志文件。";
      $("logMeta").textContent = "0 个日志文件";
      $("logMessage").textContent = "Nginx 配置中没有发现允许读取的常规日志文件。";
    }
  } catch (err) {
    state.logs = [];
    renderLogOptions();
    $("logContent").textContent = "";
    $("logMeta").textContent = "读取失败";
    $("logMessage").textContent = cleanError(err);
  }
}

function renderLogOptions() {
  const select = $("logFileSelect");
  const previous = select.value;
  select.innerHTML = "";

  if (!state.logs.length) {
    const option = document.createElement("option");
    option.value = "";
    option.textContent = "没有可读取的日志";
    select.appendChild(option);
    return;
  }

  for (const log of state.logs) {
    const option = document.createElement("option");
    option.value = log.id;
    option.textContent = (log.kind === "error" ? "错误" : "访问") + " · " + log.path;
    select.appendChild(option);
  }
  if (state.logs.some((log) => log.id === previous)) {
    select.value = previous;
  }
}

async function loadLogTail() {
  if (!state.selected) {
    $("logMessage").textContent = "请先选择一个 CLI 连接。";
    return;
  }
  const logID = $("logFileSelect").value;
  if (!logID) {
    $("logMessage").textContent = "没有可读取的日志文件。";
    return;
  }
  const lines = Number($("logLines").value || 200);
  $("refreshLogContentBtn").disabled = true;
  $("logMessage").textContent = "正在读取日志尾部…";
  try {
    const result = await api().TailLog(state.selected, logID, lines);
    $("logContent").textContent = result.content || "日志为空。";
    const kind = result.file?.kind === "error" ? "错误日志" : "访问日志";
    const path = result.file?.path || "";
    $("logMeta").textContent =
      kind + " · " + path + " · 返回 " + (result.lines || 0) + " 行" +
      (result.truncated ? " · 已按安全上限截断" : "");
    $("logMessage").textContent =
      result.truncated
        ? "内容已按 1000 行 / 512 KiB 安全上限截断。"
        : "日志读取完成。";
    $("logContent").scrollTop = $("logContent").scrollHeight;
  } catch (err) {
    $("logMessage").textContent = cleanError(err);
  } finally {
    $("refreshLogContentBtn").disabled = false;
  }
}

function resetProxyEditor() {
  state.editingSiteID = "";
  $("proxyFormTitle").textContent = "新建反向代理";
  $("createProxyBtn").textContent = "创建并应用";
  $("cancelEditProxyBtn").classList.add("hidden");
  $("proxyServerName").value = "";
  $("proxyUpstream").value = "";
  $("proxyWebSocket").checked = true;
}

$("cancelEditProxyBtn").onclick = () => {
  resetProxyEditor();
  $("siteMessage").textContent = "已取消编辑。";
};

$("createProxyBtn").onclick = async () => {
  if (!state.selected) {
    $("siteMessage").textContent = "请先选择一个 CLI 连接。";
    return;
  }

  const serverName = $("proxyServerName").value.trim();
  const upstream = $("proxyUpstream").value.trim();
  if (!serverName || !upstream) {
    $("siteMessage").textContent = "域名和上游都不能为空。";
    return;
  }

  $("createProxyBtn").disabled = true;
  $("siteMessage").textContent = state.editingSiteID ? "正在验证并更新反向代理…" : "正在生成候选配置并执行 nginx -t…";

  try {
    const request = {
      server_name: serverName,
      upstream: upstream,
      websocket: $("proxyWebSocket").checked
    };
    const editing = state.editingSiteID;
    const result = editing
      ? await api().UpdateReverseProxy(state.selected, editing, request)
      : await api().CreateReverseProxy(state.selected, request);
    $("siteMessage").textContent =
      (editing ? "修改成功：" : "创建成功：") + (result.site?.server_name || serverName) +
      (result.test_output ? "\n" + result.test_output : "");
    resetProxyEditor();
    await loadSites();
  } catch (err) {
    $("siteMessage").textContent = cleanError(err);
  } finally {
    $("createProxyBtn").disabled = false;
  }
};

function cleanError(err) {
  return String(err || "未知错误").replace(/^Error:\s*/, "");
}

function escapeHtml(value) {
  return String(value).replace(/[&<>"']/g, (ch) => ({
    "&": "&amp;",
    "<": "&lt;",
    ">": "&gt;",
    '"': "&quot;",
    "'": "&#39;"
  })[ch]);
}

refresh();
