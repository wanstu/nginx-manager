const state = {
  items: [],
  connectionHealth: {},
  upstreamHealth: {},
  selected: "",
  view: "overview",
  sites: [],
  layout: null,
  snapshots: [],
  certificates: [],
  certbot: null,
  renewalTimer: null,
  logs: [],
  logTail: null,
  logAutoTimer: null,
  overview: null,
  diagnostics: null,
  pendingLogSite: "",
  pendingLogPath: "",
  pendingCertificateSiteID: "",
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
    if (state.view === "overview") await loadOverview();
    if (state.view === "sites") await loadSites();
    if (state.view === "snapshots") await loadSnapshots();
    if (state.view === "certificates") await loadCertificates();
    if (state.view === "logs") await loadLogs();
    if (state.view === "diagnostics") await loadDiagnostics();
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
    const health = state.connectionHealth[item.id];
    const healthClass = !health
      ? "unknown"
      : (health.ok && health.privilege_ready ? "ok" : (health.ok ? "warn" : "bad"));
    const healthText = !health
      ? "未检测"
      : (health.ok && health.privilege_ready ? "正常" : (health.ok ? "权限" : "失败"));
    button.innerHTML =
      '<div class="connection-head">' +
        "<strong>" + escapeHtml(item.name) + "</strong>" +
        '<span class="connection-status ' + healthClass + '">' +
          '<i class="connection-health-dot"></i>' + escapeHtml(healthText) +
        '</span>' +
      "</div>" +
      "<span>" + escapeHtml(item.url) + "</span>";
    if (health?.message) {
      button.querySelector(".connection-status").title = health.message;
    }

    button.onclick = async () => {
      state.selected = item.id;
      state.upstreamHealth = {};
      await api().SelectConnection(item.id);
      renderConnections();
      loadEditor(item.id);
      resetProxyEditor();
      state.pendingLogSite = "";
      state.pendingLogPath = "";
      state.pendingCertificateSiteID = "";
      if (state.view === "overview") await loadOverview();
      if (state.view === "sites") await loadSites();
      if (state.view === "snapshots") await loadSnapshots();
      if (state.view === "certificates") await loadCertificates();
      if (state.view === "logs") await loadLogs();
    if (state.view === "diagnostics") await loadDiagnostics();
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
  if (resetSelection) {
    state.selected = "";
    state.upstreamHealth = {};
  }
  $("name").value = "";
  $("url").value = "";
  $("password").value = "";
  resetStatus();
  resetOverview();
  resetDiagnostics();
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
  } else if (state.view === "diagnostics") {
    $("pageTitle").textContent = item ? item.name + " · 系统诊断" : "系统诊断";
    $("pageSubtitle").textContent = "检查 systemd 服务、运行时、Certbot 和自动续期配置。";
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
  $("diagnosticsView").classList.toggle("hidden", view !== "diagnostics");
  updateHeader();
  if (view === "overview") loadOverview();
  if (view === "sites") loadSites();
  if (view === "snapshots") loadSnapshots();
  if (view === "certificates") loadCertificates();
  if (view === "diagnostics") loadDiagnostics();
  if (view === "logs") {
    loadLogs();
    syncLogAutoRefresh();
  } else {
    stopLogAutoRefresh();
  }
}

document.querySelectorAll(".nav-item").forEach((item) => {
  item.onclick = () => setView(item.dataset.view);
});

$("newBtn").onclick = () => {
  setView("overview");
  clearEditor();
};

$("checkAllBtn").onclick = async () => {
  const button = $("checkAllBtn");
  button.disabled = true;
  button.textContent = "正在检查…";
  try {
    const results = await api().CheckConnections();
    const next = {};
    for (const result of results || []) {
      next[result.id] = result;
    }
    state.connectionHealth = next;
    renderConnections();
  } catch (err) {
    $("message").textContent = "批量检查失败：" + cleanError(err);
  } finally {
    button.disabled = false;
    button.textContent = "检查全部连接";
  }
};

$("saveBtn").onclick = async () => {
  try {
    const saved = await api().SaveConnection({
      id: state.selected,
      name: $("name").value,
      url: $("url").value,
      password: $("password").value
    });
    delete state.connectionHealth[saved.id];
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
    state.connectionHealth[state.selected] = {
      id: state.selected,
      ok: Boolean(result.ok),
      privilege_ready: Boolean(result.privilege_ready),
      message: result.privilege_message || result.message || "",
      runtime: result.runtime || ""
    };
    renderConnections();
    if (result.ok && result.privilege_ready) {
      $("statusBadge").textContent = "正常";
      $("statusBadge").className = "badge ok";
    } else if (result.ok) {
      $("statusBadge").textContent = "需关注";
      $("statusBadge").className = "badge warn";
    } else {
      $("statusBadge").textContent = "失败";
      $("statusBadge").className = "badge bad";
    }
    $("hostValue").textContent = result.hostname || "—";
    $("versionValue").textContent = result.version || "—";
    $("runtimeValue").textContent = result.runtime || "—";
    $("privilegeValue").textContent = result.privilege_ready ? "正常" : "未就绪";
    $("message").textContent = (result.message || "") + (result.privilege_message ? "\n管理权限：" + result.privilege_message : "");
  } catch (err) {
    state.connectionHealth[state.selected] = {
      id: state.selected,
      ok: false,
      privilege_ready: false,
      message: cleanError(err)
    };
    renderConnections();
    $("statusBadge").textContent = "失败";
    $("statusBadge").className = "badge bad";
    $("message").textContent = cleanError(err);
  }
};

$("deleteBtn").onclick = async () => {
  if (!state.selected) return;
  if (!confirm("删除这个 CLI 连接？保存的密码也会一起删除。")) return;

  const deletedID = state.selected;
  stopLogAutoRefresh();
  await api().DeleteConnection(deletedID);
  delete state.connectionHealth[deletedID];
  state.selected = "";
  state.upstreamHealth = {};
  state.sites = [];
  state.snapshots = [];
  state.certificates = [];
  state.certbot = null;
  state.renewalTimer = null;
  state.logs = [];
  state.logTail = null;
  state.overview = null;
  state.diagnostics = null;
  state.pendingLogSite = "";
  state.pendingLogPath = "";
  state.pendingCertificateSiteID = "";
  await refresh();
};

$("refreshOverviewBtn").onclick = loadOverview;
$("overviewTrafficWindow").onchange = loadOverview;
$("safeReloadBtn").onclick = async () => {
  if (!state.selected) {
    $("overviewMessage").textContent = "请先选择一个 CLI 连接。";
    return;
  }

  $("safeReloadBtn").disabled = true;
  $("overviewMessage").textContent = "正在执行 nginx -t；只有配置通过才会 reload…";
  try {
    const result = await api().SafeReload(state.selected);
    $("overviewMessage").textContent =
      "Reload 完成。" +
      (result.test_output ? "\n配置测试：" + result.test_output : "") +
      (result.reload_output ? "\nReload：" + result.reload_output : "");
    await loadOverview();
  } catch (err) {
    $("overviewMessage").textContent = cleanError(err);
    await loadOverview();
  } finally {
    $("safeReloadBtn").disabled = false;
  }
};

async function loadOverview() {
  if (!state.selected) {
    state.overview = null;
    resetOverview();
    $("overviewMessage").textContent = "请先选择一个 CLI 连接。";
    return;
  }

  $("overviewMessage").textContent = "正在汇总站点、证书、日志与 Nginx 状态…";
  try {
    const trafficWindowMinutes = Number($("overviewTrafficWindow").value || 60);
    const result = await api().LoadOverviewWindow(state.selected, trafficWindowMinutes);
    state.overview = result;

    $("hostValue").textContent = result.hostname || "—";
    $("versionValue").textContent = result.cli_version || "—";
    $("runtimeValue").textContent = result.runtime || "—";
    $("privilegeValue").textContent = result.privilege_ready ? "正常" : "未就绪";

    const issues = result.issues || [];
    const warnings = result.warnings || [];
    if (issues.length) {
      $("statusBadge").textContent = "异常";
      $("statusBadge").className = "badge bad";
    } else if (warnings.length) {
      $("statusBadge").textContent = "需关注";
      $("statusBadge").className = "badge warn";
    } else {
      $("statusBadge").textContent = "正常";
      $("statusBadge").className = "badge ok";
    }

    $("overviewConfig").textContent = result.config_ok ? "nginx -t 通过" : "异常";
    $("overviewSites").textContent =
      (result.enabled_sites || 0) + " / " + (result.total_sites || 0) + " 已启用";
    $("overviewManaged").textContent = String(result.managed_sites || 0);
    $("overviewHTTPS").textContent = String(result.https_sites || 0);
    const certificateParts = [String(result.certificates || 0)];
    if (result.certificates_expired) certificateParts.push(result.certificates_expired + " 张已过期");
    if (result.certificates_expiring) certificateParts.push(result.certificates_expiring + " 张 30 天内到期");
    if (result.https_without_certificate) certificateParts.push(result.https_without_certificate + " 个站点未匹配");
    $("overviewCertificates").textContent = certificateParts.join(" · ");
    $("overviewExpiry").textContent = formatOverviewExpiry(result.nearest_expiry);
    $("overviewLogs").textContent = String(result.log_files || 0);
    $("overviewRenewal").textContent = formatRenewalTimerStatus(result.renewal_timer);
    $("overviewRequests").textContent =
      (result.traffic_parsed_lines || 0) + " / " + (result.traffic_sample_lines || 0) + " 已识别";
    $("overviewSuccess").textContent =
      (result.status_2xx || 0) + " / " + (result.status_3xx || 0);
    $("overviewErrors").textContent =
      (result.status_4xx || 0) + " / " + (result.status_5xx || 0);
    $("overviewBytes").textContent = formatBytes(result.response_bytes || 0);
    const trafficSources = result.traffic_sources || [];
    $("overviewTrafficSource").textContent = result.traffic_logs_available
      ? "访问概况：聚合 " + (result.traffic_logs_used || 0) + " / " + result.traffic_logs_available + " 个 access log" +
        " · 读取 " + (result.traffic_read_lines || 0) + " 行" +
        " · 窗口内 " + (result.traffic_sample_lines || 0) + " 行" +
        (result.traffic_timestamp_unknown ? " · 时间戳未知 " + result.traffic_timestamp_unknown + " 行未纳入窗口" : "") +
        (result.traffic_unparsed_lines ? " · 状态码未识别 " + result.traffic_unparsed_lines + " 行" : "") +
        (trafficSources.length === 1 ? " · " + trafficSources[0] : "")
      : "未发现可用于访问概况的 access log。";
    renderOverviewSiteTraffic(result.site_traffic || [], result.traffic_window_minutes || trafficWindowMinutes);

    const layout = result.layout || {};
    $("overviewLayout").textContent =
      (layout.mode ? layout.mode + " · " : "") +
      (layout.available_dir || layout.main_config || "未识别配置布局");

    if (issues.length || warnings.length) {
      const sections = [];
      if (issues.length) sections.push("异常：\n- " + issues.join("\n- "));
      if (warnings.length) sections.push("维护提醒：\n- " + warnings.join("\n- "));
      $("overviewMessage").textContent = sections.join("\n\n");
    } else {
      $("overviewMessage").textContent =
        "状态正常。" + (result.config_output ? "\n" + result.config_output : "");
    }
    $("message").textContent =
      "连接正常" + (result.privilege_message ? "\n管理权限：" + result.privilege_message : "");
  } catch (err) {
    state.overview = null;
    resetOverview();
    $("statusBadge").textContent = "失败";
    $("statusBadge").className = "badge bad";
    $("overviewMessage").textContent = cleanError(err);
  }
}

function resetOverview() {
  $("overviewConfig").textContent = "—";
  $("overviewSites").textContent = "—";
  $("overviewManaged").textContent = "—";
  $("overviewHTTPS").textContent = "—";
  $("overviewCertificates").textContent = "—";
  $("overviewExpiry").textContent = "—";
  $("overviewLogs").textContent = "—";
  $("overviewRenewal").textContent = "—";
  $("overviewRequests").textContent = "—";
  $("overviewSuccess").textContent = "—";
  $("overviewErrors").textContent = "—";
  $("overviewBytes").textContent = "—";
  $("overviewTrafficSource").textContent = "";
  renderOverviewSiteTraffic([]);
  $("overviewLayout").textContent = "选择连接后读取 Nginx 配置、站点、证书和日志状态。";
}

function renderOverviewSiteTraffic(items, windowMinutes = 0) {
  const root = $("overviewSiteTraffic");
  root.innerHTML = "";
  const visible = (items || []).slice(0, 8);
  if (!visible.length) {
    root.classList.add("hidden");
    return;
  }

  root.classList.remove("hidden");
  const title = document.createElement("div");
  title.className = "site-traffic-title";
  title.textContent =
    "站点访问样本 · " + formatTrafficWindow(windowMinutes) + "（独立日志直接归属；其他日志按域名匹配）";
  root.appendChild(title);

  for (const item of visible) {
    const row = document.createElement("div");
    const parsed = Number(item.parsed_lines || 0);
    const status5xx = Number(item.status_5xx || 0);
    const status4xx = Number(item.status_4xx || 0);
    const errorRate = parsed > 0 ? (status5xx / parsed) * 100 : 0;
    row.className =
      "site-traffic-row" +
      (status5xx > 0 ? " bad" : (status4xx > 0 ? " warn" : ""));
    row.innerHTML =
      '<strong>' + escapeHtml(item.server_name || "未知站点") + '</strong>' +
      '<span>' + (item.matched_lines || 0) + ' 行</span>' +
      '<span>2xx ' + (item.status_2xx || 0) + '</span>' +
      '<span>3xx ' + (item.status_3xx || 0) + '</span>' +
      '<span>4xx ' + status4xx + '</span>' +
      '<span>5xx ' + status5xx + (status5xx ? ' · ' + errorRate.toFixed(errorRate >= 10 ? 1 : 2) + '%' : '') + '</span>' +
      '<span>' + escapeHtml(formatBytes(item.response_bytes || 0)) + '</span>';
    root.appendChild(row);
  }
}

function formatTrafficWindow(minutes) {
  const value = Number(minutes || 0);
  if (value === 15) return "15 分钟";
  if (value === 60) return "1 小时";
  if (value === 360) return "6 小时";
  if (value === 1440) return "24 小时";
  return "最近样本";
}

function formatBytes(value) {
  const bytes = Number(value || 0);
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const units = ["B", "KiB", "MiB", "GiB"];
  let size = bytes;
  let unit = 0;
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit++;
  }
  return (unit === 0 ? Math.round(size) : size.toFixed(size >= 10 ? 1 : 2)) + " " + units[unit];
}

function formatOverviewExpiry(value) {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  const days = Math.ceil((date.getTime() - Date.now()) / 86400000);
  if (days < 0) return "已过期 " + Math.abs(days) + " 天";
  return days + " 天 · " + date.toLocaleDateString("zh-CN");
}

$("refreshDiagnosticsBtn").onclick = loadDiagnostics;

async function loadDiagnostics() {
  if (!state.selected) {
    state.diagnostics = null;
    resetDiagnostics();
    $("diagnosticsMessage").textContent = "请先选择一个 CLI 连接。";
    return;
  }

  $("diagnosticsMessage").textContent = "正在检查固定 systemd unit、Certbot 与运行时…";
  try {
    const [result, connection] = await Promise.all([
      api().LoadDiagnostics(state.selected),
      api().TestConnection(state.selected)
    ]);
    state.diagnostics = result;

    $("diagManagerService").textContent =
      formatServiceUnit(result.services?.manager, result.services?.systemd_available);
    $("diagRuntimeService").textContent =
      formatServiceUnit(result.services?.runtime, result.services?.systemd_available);
    $("diagCertbot").textContent = result.certbot?.available
      ? (result.certbot.version || "已安装")
      : "未检测到";
    $("diagRenewal").textContent = formatRenewalTimerStatus(result.renewal_timer);

    $("diagManagerExecutable").textContent = result.executable || "—";
    $("diagRuntimeKind").textContent =
      (result.runtime?.kind || "—") + (result.runtime?.version ? " · " + result.runtime.version : "");
    $("diagRuntimePath").textContent = result.runtime?.path || "—";
    $("diagLayoutMode").textContent = result.layout?.mode || "—";
    $("diagMainConfig").textContent = result.layout?.main_config || "—";
    $("diagAvailableDir").textContent = result.layout?.available_dir || "—";
    $("diagEnabledDir").textContent = result.layout?.enabled_dir || "—";
    $("diagPathsConfig").textContent = result.paths_configured
      ? (result.paths_config_path || "/etc/nginx-manager/paths.json") + " · 已加载"
      : (result.paths_config_path || "/etc/nginx-manager/paths.json") + " · 使用默认值";
    $("diagSiteLogDir").textContent =
      result.paths_config?.site_log_dir || "/var/log/nginx/nginx-manager";
    $("diagSnapshotDir").textContent =
      result.paths_config?.snapshot_dir || "/var/lib/nginx-manager/snapshots";
    renderReadiness(result, connection);
    renderDeploymentCommands(result, connection);

    const notes = [];
    if (!result.services?.systemd_available) {
      notes.push("当前环境未检测到 systemctl；可能不是 systemd 主机或 systemctl 不在 PATH。");
    } else {
      const managerUnit = result.services?.manager;
      if (!managerUnit?.installed) {
        notes.push("未检测到 nginx-manager.service；当前 CLI 可能由手工命令或其他进程管理器启动。");
      } else {
        if (!managerUnit.active) notes.push("nginx-manager.service 已安装但当前不是 active。");
        if (!managerUnit.enabled) notes.push("nginx-manager.service 未设置为开机启用。");
      }

      const runtimeUnit = result.services?.runtime;
      if (!runtimeUnit?.installed) {
        notes.push("未识别到 nginx.service / openresty.service；Nginx 可能由容器或其他方式管理。");
      } else {
        if (!runtimeUnit.active) notes.push(runtimeUnit.name + " 已安装但当前不是 active。");
        if (!runtimeUnit.enabled) notes.push(runtimeUnit.name + " 未设置为开机启用。");
      }
    }
    if (!result.certbot?.available) {
      notes.push("Certbot 未安装或不在 PATH；ACME 签发/续期不可用。");
    }
    if (result.certbot?.available && !result.renewal_timer?.active) {
      notes.push("Certbot 可用，但 nginx-manager-renew.timer 当前未运行。");
    }

    $("diagnosticsMessage").textContent = notes.length
      ? "诊断提示：\n- " + notes.join("\n- ")
      : "systemd 服务、Certbot 与自动续期状态正常。";
  } catch (err) {
    state.diagnostics = null;
    resetDiagnostics();
    renderReadiness(null, null);
    $("diagnosticsMessage").textContent = cleanError(err);
  }
}

function formatServiceUnit(unit, systemdAvailable) {
  if (!systemdAvailable) return "systemd 不可用";
  if (!unit?.installed) return (unit?.name || "unit") + " · 未安装";
  const states = [unit.name || "unit"];
  states.push(unit.active ? "active" : "inactive");
  states.push(unit.enabled ? "enabled" : "disabled");
  return states.join(" · ");
}

function renderReadiness(result, connection) {
  const root = $("readinessList");
  root.innerHTML = "";

  if (!result || !connection) {
    root.innerHTML = '<div class="empty large">尚未运行部署就绪检查</div>';
    return;
  }

  const items = [
    {
      label: "CLI 连接与认证",
      ok: Boolean(connection.ok),
      required: true,
      detail: connection.ok ? "API 可访问，管理密码有效" : (connection.message || "连接失败")
    },
    {
      label: "受限 root helper + nginx -t",
      ok: Boolean(connection.privilege_ready),
      required: true,
      detail: connection.privilege_message || "未就绪"
    },
    {
      label: "可信路径配置",
      ok: Boolean(result.paths_configured),
      required: true,
      detail: result.paths_configured
        ? (result.paths_config_path || "/etc/nginx-manager/paths.json")
        : "未安装；当前使用默认/兼容 fallback"
    },
    {
      label: "nginx-manager systemd 托管",
      ok: Boolean(result.services?.manager?.active),
      required: false,
      detail: formatServiceUnit(result.services?.manager, result.services?.systemd_available)
    },
    {
      label: "Nginx / OpenResty 服务",
      ok: Boolean(result.services?.runtime?.active),
      required: false,
      detail: formatServiceUnit(result.services?.runtime, result.services?.systemd_available)
    },
    {
      label: "Certbot",
      ok: Boolean(result.certbot?.available),
      required: false,
      detail: result.certbot?.available ? (result.certbot.version || "已安装") : "HTTPS ACME 功能不可用"
    },
    {
      label: "自动证书续期",
      ok: Boolean(result.renewal_timer?.active && result.renewal_timer?.enabled),
      required: false,
      detail: formatRenewalTimerStatus(result.renewal_timer)
    }
  ];

  for (const item of items) {
    const row = document.createElement("div");
    row.className = "readiness-row " + (item.ok ? "ok" : (item.required ? "bad" : "warn"));
    row.innerHTML =
      '<span class="readiness-state">' + (item.ok ? "✓" : (item.required ? "!" : "·")) + '</span>' +
      '<div class="readiness-main">' +
        '<strong>' + escapeHtml(item.label) + '</strong>' +
        '<span>' + escapeHtml(item.detail) + '</span>' +
      '</div>' +
      '<span class="readiness-kind">' + (item.required ? "核心" : "可选") + '</span>';
    root.appendChild(row);
  }
}

function shellQuote(value) {
  return "'" + String(value || "").replace(/'/g, "'\"'\"'") + "'";
}

function renderDeploymentCommands(result, connection) {
  const target = $("deploymentCommands");
  if (!result || !connection) {
    target.textContent = "运行诊断后生成部署辅助命令。";
    return;
  }

  const executable = result.executable || "/usr/local/bin/nginx-manager";
  const q = shellQuote(executable);
  const commands = [];

  if (!connection.privilege_ready) {
    commands.push(
      "# 安装 / 修复最小 sudoers",
      q + " privileged sudoers --service-user nginx-manager --helper " + q + " | sudo tee /tmp/nginx-manager.sudoers >/dev/null",
      "sudo visudo -cf /tmp/nginx-manager.sudoers",
      "sudo install -o root -g root -m 0440 /tmp/nginx-manager.sudoers /etc/sudoers.d/nginx-manager",
      "sudo rm -f /tmp/nginx-manager.sudoers",
      ""
    );
  }

  if (!result.paths_configured) {
    commands.push(
      "# 安装可信路径配置",
      "sudo install -d -o root -g root -m 0755 /etc/nginx-manager",
      q + " config paths-template | sudo tee /etc/nginx-manager/paths.json >/dev/null",
      "sudo chown root:root /etc/nginx-manager/paths.json",
      "sudo chmod 0644 /etc/nginx-manager/paths.json",
      ""
    );
  }

  const managerUnit = result.services?.manager;
  if (!managerUnit?.installed || !managerUnit?.active || !managerUnit?.enabled) {
    commands.push(
      "# 安装 / 修复 nginx-manager.service",
      q + " service systemd --service-user nginx-manager --binary " + q + " --listen 127.0.0.1:8020 | sudo tee /etc/systemd/system/nginx-manager.service >/dev/null",
      "sudo systemctl daemon-reload",
      "sudo systemctl enable --now nginx-manager.service",
      ""
    );
  }

  if (result.certbot?.available &&
      (!result.renewal_timer?.installed || !result.renewal_timer?.enabled || !result.renewal_timer?.active)) {
    commands.push(
      "# 安装 / 修复自动证书续期",
      q + " service renewal-service --binary " + q + " | sudo tee /etc/systemd/system/nginx-manager-renew.service >/dev/null",
      q + " service renewal-timer | sudo tee /etc/systemd/system/nginx-manager-renew.timer >/dev/null",
      "sudo systemctl daemon-reload",
      "sudo systemctl enable --now nginx-manager-renew.timer",
      ""
    );
  }

  if (!commands.length) {
    target.textContent = "# 当前没有需要生成的部署修复命令。";
    return;
  }
  target.textContent = commands.join("\n").trim();
}

$("copyDeploymentCommandsBtn").onclick = async () => {
  const content = $("deploymentCommands").textContent || "";
  if (!content || content.startsWith("运行诊断后")) return;
  try {
    await navigator.clipboard.writeText(content);
    $("diagnosticsMessage").textContent = "部署辅助命令已复制。";
  } catch (err) {
    $("diagnosticsMessage").textContent = "复制失败：" + cleanError(err);
  }
};

function resetDiagnostics() {
  $("diagManagerExecutable").textContent = "—";
  $("diagManagerService").textContent = "—";
  $("diagRuntimeService").textContent = "—";
  $("diagCertbot").textContent = "—";
  $("diagRenewal").textContent = "—";
  $("diagRuntimeKind").textContent = "—";
  $("diagRuntimePath").textContent = "—";
  $("diagLayoutMode").textContent = "—";
  $("diagMainConfig").textContent = "—";
  $("diagAvailableDir").textContent = "—";
  $("diagEnabledDir").textContent = "—";
  $("diagPathsConfig").textContent = "—";
  $("diagSiteLogDir").textContent = "—";
  $("diagSnapshotDir").textContent = "—";
  renderReadiness(null, null);
  renderDeploymentCommands(null, null);
}

$("checkAllUpstreamsBtn").onclick = async () => {
  if (!state.selected) {
    $("siteMessage").textContent = "请先选择一个 CLI 连接。";
    return;
  }

  const connectionID = state.selected;
  const targets = (state.sites || []).filter((site) => site.managed && site.proxy_pass);
  if (!targets.length) {
    $("siteMessage").textContent = "当前没有可检查的 Manager 反向代理站点。";
    return;
  }

  const button = $("checkAllUpstreamsBtn");
  button.disabled = true;
  const results = {};
  let cursor = 0;
  let completed = 0;
  let healthy = 0;
  let degraded = 0;
  let unreachable = 0;

  $("siteMessage").textContent = "正在检查 " + targets.length + " 个上游…";

  const worker = async () => {
    while (true) {
      const index = cursor++;
      if (index >= targets.length) return;
      const site = targets[index];
      try {
        const health = await api().CheckUpstream(connectionID, site.id);
        results[site.id] = health;
        if (health.healthy) healthy++;
        else if (health.reachable) degraded++;
        else unreachable++;
      } catch (err) {
        results[site.id] = {
          site_id: site.id,
          server_name: site.server_name,
          upstream: site.proxy_pass,
          reachable: false,
          healthy: false,
          error: cleanError(err)
        };
        unreachable++;
      } finally {
        completed++;
        if (state.selected === connectionID) {
          $("siteMessage").textContent =
            "正在检查上游：" + completed + " / " + targets.length;
        }
      }
    }
  };

  try {
    const workerCount = Math.min(4, targets.length);
    await Promise.all(Array.from({ length: workerCount }, () => worker()));

    if (state.selected === connectionID) {
      state.upstreamHealth = { ...state.upstreamHealth, ...results };
      renderSites();
      $("siteMessage").textContent =
        "上游检查完成：" +
        healthy + " 正常 · " +
        degraded + " 可达但 HTTP 异常 · " +
        unreachable + " 不可达";
    }
  } finally {
    button.disabled = false;
  }
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

function upstreamHealthBadge(result) {
  if (!result) return "";
  if (result.healthy) {
    return '<span class="mini-badge ok">上游正常 · ' + (result.latency_ms || 0) + 'ms</span>';
  }
  if (result.reachable) {
    return '<span class="mini-badge warn">HTTP ' + (result.status_code || "?") + ' · ' + (result.latency_ms || 0) + 'ms</span>';
  }
  return '<span class="mini-badge bad">上游不可达</span>';
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
    const optionParts = [];
    if (site.max_body_size_mb) optionParts.push("请求体 " + site.max_body_size_mb + " MB");
    if (site.connect_timeout_seconds) optionParts.push("连接 " + site.connect_timeout_seconds + "s");
    if (site.read_timeout_seconds) optionParts.push("读取 " + site.read_timeout_seconds + "s");
    item.innerHTML =
      '<div class="site-main">' +
        '<strong>' + escapeHtml(title) + '</strong>' +
        '<span>' + escapeHtml(proxy) + '</span>' +
        (optionParts.length ? '<span class="site-options">' + escapeHtml(optionParts.join(" · ")) + '</span>' : '') +
      '</div>' +
      '<div class="site-side">' +
        '<div class="site-tags">' +
          '<span class="mini-badge ' + (site.enabled ? "ok" : "") + '">' + (site.enabled ? "已启用" : "未启用") + '</span>' +
          '<span class="mini-badge ' + (site.managed ? "managed" : "") + '">' + (site.managed ? "Manager 管理" : "外部配置") + '</span>' +
          (site.https ? '<span class="mini-badge https">HTTPS' + (site.redirect_https ? ' · 强制' : '') + '</span>' : '') +
          (site.access_log ? '<span class="mini-badge log">独立日志</span>' : '') +
          upstreamHealthBadge(state.upstreamHealth[site.id]) +
        '</div>' +
      '</div>';

    const actions = document.createElement("div");
    actions.className = "site-actions";

    const viewConfig = document.createElement("button");
    viewConfig.textContent = "配置";
    viewConfig.onclick = () => openSiteConfig(site);
    actions.appendChild(viewConfig);

    if (site.managed) {
      if (site.server_name && site.server_name !== "_") {
        const logs = document.createElement("button");
        logs.textContent = "日志";
        logs.onclick = () => {
          state.pendingLogPath = site.access_log || "";
          state.pendingLogSite = site.access_log ? "" : site.server_name;
          setView("logs");
        };
        actions.appendChild(logs);

        if (site.proxy_pass) {
          const certificate = document.createElement("button");
          certificate.textContent = site.https ? "证书" : "HTTPS";
          certificate.onclick = () => {
            state.pendingCertificateSiteID = site.id;
            setView("certificates");
          };
          actions.appendChild(certificate);
        }
      }

      if (site.proxy_pass) {
        const checkUpstream = document.createElement("button");
        checkUpstream.textContent = "测试上游";
        checkUpstream.onclick = async () => {
          checkUpstream.disabled = true;
          $("siteMessage").textContent = "正在从服务器检查 " + title + " 的上游…";
          try {
            const health = await api().CheckUpstream(state.selected, site.id);
            state.upstreamHealth[site.id] = health;
            renderSites();
            if (health.healthy) {
              $("siteMessage").textContent =
                title + " 上游正常 · HTTP " + health.status_code + " · " + health.latency_ms + "ms";
            } else if (health.reachable) {
              $("siteMessage").textContent =
                title + " 上游可达，但返回 HTTP " + health.status_code + " · " + health.latency_ms + "ms";
            } else {
              $("siteMessage").textContent =
                title + " 上游不可达" + (health.error ? "：" + health.error : "");
            }
          } catch (err) {
            $("siteMessage").textContent = cleanError(err);
          } finally {
            checkUpstream.disabled = false;
          }
        };
        actions.appendChild(checkUpstream);
      }

      if (site.proxy_pass && !site.access_log) {
        const migrateLogs = document.createElement("button");
        migrateLogs.textContent = "接入独立日志";
        migrateLogs.onclick = async () => {
          migrateLogs.disabled = true;
          $("siteMessage").textContent = "正在为 " + title + " 接入独立访问/错误日志…";
          try {
            await api().UpdateReverseProxy(state.selected, site.id, {
              server_name: site.server_name,
              upstream: site.proxy_pass,
              websocket: Boolean(site.websocket),
              max_body_size_mb: Number(site.max_body_size_mb || 0),
              connect_timeout_seconds: Number(site.connect_timeout_seconds || 0),
              read_timeout_seconds: Number(site.read_timeout_seconds || 0)
            });
            $("siteMessage").textContent = "已为 " + title + " 接入独立日志。";
            await loadSites();
          } catch (err) {
            $("siteMessage").textContent = cleanError(err);
          } finally {
            migrateLogs.disabled = false;
          }
        };
        actions.appendChild(migrateLogs);
      }

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
          $("proxyMaxBodySize").value = site.max_body_size_mb || "";
          $("proxyConnectTimeout").value = site.connect_timeout_seconds || "";
          $("proxyReadTimeout").value = site.read_timeout_seconds || "";
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
    }

    item.querySelector(".site-side").appendChild(actions);
    root.appendChild(item);
  }
}

async function openSiteConfig(site) {
  if (!state.selected || !site?.id) return;

  $("siteConfigTitle").textContent = (site.server_name || site.id) + " · 配置";
  $("siteConfigMeta").textContent =
    (site.managed ? "Manager 管理" : "外部配置") + " · 正在读取…";
  $("siteConfigContent").textContent = "正在读取配置…";
  $("siteConfigModal").classList.remove("hidden");
  $("siteConfigModal").setAttribute("aria-hidden", "false");

  try {
    const view = await api().GetSiteConfig(state.selected, site.id);
    $("siteConfigContent").textContent = view.content || "";
    $("siteConfigMeta").textContent =
      (view.site?.managed ? "Manager 管理" : "外部配置") +
      " · " + (view.size_bytes || 0) + " bytes" +
      " · " + (view.site?.path || site.path || "");
  } catch (err) {
    $("siteConfigContent").textContent = cleanError(err);
    $("siteConfigMeta").textContent = "读取失败";
  }
}

function closeSiteConfig() {
  $("siteConfigModal").classList.add("hidden");
  $("siteConfigModal").setAttribute("aria-hidden", "true");
}

$("closeSiteConfigBtn").onclick = closeSiteConfig;
document.querySelectorAll("[data-close-site-config]").forEach((element) => {
  element.onclick = closeSiteConfig;
});
$("copySiteConfigBtn").onclick = async () => {
  const content = $("siteConfigContent").textContent || "";
  if (!content) return;
  try {
    await navigator.clipboard.writeText(content);
    $("siteConfigMeta").textContent += " · 已复制";
  } catch (err) {
    $("siteConfigMeta").textContent += " · 复制失败：" + cleanError(err);
  }
};
document.addEventListener("keydown", (event) => {
  if (event.key === "Escape" && !$("siteConfigModal").classList.contains("hidden")) {
    closeSiteConfig();
  }
});

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
  if (value === "enable_https") return "启用 HTTPS 前";
  if (value === "disable_https") return "关闭 HTTPS 前";
  if (value === "tls_settings") return "修改 HTTPS 设置前";
  if (value === "enable_existing_https") return "复用证书启用 HTTPS 前";
  return value || "未知操作";
}

function formatSnapshotTime(value) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value || "未知时间";
  return date.toLocaleString("zh-CN", { hour12: false });
}

$("refreshCertificatesBtn").onclick = loadCertificates;
$("certificateSite").onchange = syncCertificateSiteState;

async function loadCertificates() {
  if (!state.selected) {
    state.certificates = [];
    state.certbot = null;
    state.renewalTimer = null;
    state.sites = [];
    state.snapshots = [];
    renderCertificates();
    renderCertificateSites();
    renderTLSHistory();
    $("certificateMessage").textContent = "请先选择一个 CLI 连接。";
    return;
  }

  $("certificatesList").innerHTML = '<div class="empty large">正在读取证书…</div>';
  try {
    const certificateResult = await api().ListCertificates(state.selected);
    const siteResult = await api().ListSites(state.selected);
    let snapshots = [];
    try {
      snapshots = await api().ListSnapshots(state.selected);
    } catch (_) {
      snapshots = [];
    }
    state.certificates = certificateResult.certificates || [];
    state.certbot = certificateResult.certbot || null;
    state.renewalTimer = certificateResult.renewal_timer || null;
    state.sites = siteResult.sites || [];
    state.snapshots = snapshots || [];
    renderCertificates();
    renderCertificateSites();
    renderTLSHistory();
    $("certificateMessage").textContent = "已读取 " + state.certificates.length + " 张证书。";
  } catch (err) {
    state.certificates = [];
    state.certbot = null;
    state.renewalTimer = null;
    state.sites = [];
    state.snapshots = [];
    renderCertificates();
    renderCertificateSites();
    renderTLSHistory();
    $("certificateMessage").textContent = cleanError(err);
  }
}

function renderCertificates() {
  const root = $("certificatesList");
  root.innerHTML = "";

  const certbotReady = Boolean(state.certbot?.available);
  const renewalStatus = formatRenewalTimerStatus(state.renewalTimer);
  $("issueCertificateBtn").disabled = !certbotReady;
  $("renewCertificatesBtn").disabled = !certbotReady;
  if (certbotReady) {
    $("certbotValue").textContent =
      "Certbot 已就绪 · " + (state.certbot.version || state.certbot.path || "可用") +
      " · 自动续期：" + renewalStatus;
  } else {
    $("certbotValue").textContent =
      "Certbot 未安装或不在 PATH 中；签发和续期功能不可用。 · 自动续期：" + renewalStatus;
  }
  renderCertificateHealth();

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

function renderTLSHistory() {
  const root = $("tlsHistoryList");
  root.innerHTML = "";

  const tlsOperations = new Set([
    "enable_https",
    "disable_https",
    "tls_settings",
    "enable_existing_https"
  ]);
  const items = (state.snapshots || [])
    .filter((snapshot) => tlsOperations.has(snapshot.operation))
    .slice(0, 12);

  if (!items.length) {
    root.innerHTML = '<div class="empty large">没有发现 HTTPS 相关变更快照</div>';
    return;
  }

  for (const snapshot of items) {
    const site = (state.sites || []).find((item) => item.id === snapshot.site_id);
    const row = document.createElement("div");
    row.className = "tls-history-row";
    row.innerHTML =
      '<div class="tls-history-main">' +
        '<strong>' + escapeHtml(site?.server_name || snapshot.site_id || "未知站点") + '</strong>' +
        '<span>' + escapeHtml(snapshotOperationLabel(snapshot.operation)) + '</span>' +
      '</div>' +
      '<time>' + escapeHtml(formatSnapshotTime(snapshot.created_at)) + '</time>';
    root.appendChild(row);
  }
}

function formatRenewalTimerStatus(timer) {
  if (!timer?.systemd_available) return "systemd 不可用";
  if (timer.active && timer.enabled) return "运行中";
  if (timer.installed && timer.enabled) return "已启用但未运行";
  if (timer.installed) return "已安装未启用";
  return "未安装";
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
    syncCertificateSiteState();
    return;
  }

  for (const site of candidates) {
    const option = document.createElement("option");
    option.value = site.id;
    option.textContent =
      site.server_name + (site.https ? " · 已启用 HTTPS" : " · HTTP");
    select.appendChild(option);
  }

  if (state.pendingCertificateSiteID && candidates.some((site) => site.id === state.pendingCertificateSiteID)) {
    select.value = state.pendingCertificateSiteID;
    state.pendingCertificateSiteID = "";
  }
  syncCertificateSiteState();
}

function selectedCertificateSite() {
  const siteID = $("certificateSite").value;
  return state.sites.find((site) => site.id === siteID) || null;
}

function certificateNameCoversHost(pattern, host) {
  pattern = String(pattern || "").trim().toLowerCase();
  host = String(host || "").trim().toLowerCase();
  if (!pattern || !host) return false;
  if (pattern === host) return true;
  if (!pattern.startsWith("*.")) return false;

  const suffix = pattern.slice(1);
  if (!host.endsWith(suffix)) return false;
  const prefix = host.slice(0, host.length - suffix.length);
  return Boolean(prefix) && !prefix.includes(".");
}

function certificateCoversSite(certificate, site) {
  if (!site?.server_name) return false;
  if (certificateNameCoversHost(certificate?.name, site.server_name)) return true;
  return (certificate?.domains || []).some((domain) =>
    certificateNameCoversHost(domain, site.server_name)
  );
}

function hasCertificateForSite(site) {
  return (state.certificates || []).some((certificate) => certificateCoversSite(certificate, site));
}

function renderCertificateHealth() {
  const root = $("certificateHealth");
  const certificates = state.certificates || [];
  const sites = state.sites || [];
  const warnings = [];
  const now = Date.now();
  let expired = 0;
  let expiring = 0;

  for (const certificate of certificates) {
    const expires = new Date(certificate.not_after).getTime();
    if (!Number.isFinite(expires)) continue;
    const remaining = expires - now;
    if (remaining < 0) expired++;
    else if (remaining < 30 * 86400000) expiring++;
  }

  const missing = sites.filter((site) =>
    site.https &&
    site.server_name &&
    site.server_name !== "_" &&
    !hasCertificateForSite(site)
  ).length;

  if (expired) warnings.push(expired + " 张证书已过期");
  if (expiring) warnings.push(expiring + " 张证书将在 30 天内到期");
  if (missing) warnings.push(missing + " 个 HTTPS 站点未找到匹配证书");
  if (certificates.length && !state.certbot?.available) {
    warnings.push("Certbot 当前不可用");
  }
  if (certificates.length && (!state.renewalTimer?.enabled || !state.renewalTimer?.active)) {
    warnings.push("自动续期 Timer 未正常运行");
  }

  if (warnings.length) {
    root.className = "health-summary warn";
    root.textContent = "维护提醒：" + warnings.join("；");
  } else {
    root.className = "health-summary ok";
    root.textContent = certificates.length
      ? "证书维护状态正常。"
      : "当前没有发现 Certbot / Let’s Encrypt 证书。";
  }
}

function syncCertificateSiteState() {
  const site = selectedCertificateSite();
  const certbotReady = Boolean(state.certbot?.available);
  const hasExistingCertificate = hasCertificateForSite(site);

  $("issueCertificateBtn").disabled = !certbotReady || !site;
  $("applyTLSSettingsBtn").disabled = !site || (!site.https && !hasExistingCertificate);
  $("disableHTTPSBtn").disabled = !site?.https;

  if (!site) {
    $("applyTLSSettingsBtn").textContent = "保存 HTTPS 设置";
    $("certificateSiteStatus").textContent = "当前没有可管理的 Manager 反向代理站点。";
    return;
  }
  if (site.https) {
    $("applyTLSSettingsBtn").textContent = "保存 HTTPS 设置";
    $("redirectHTTPS").checked = Boolean(site.redirect_https);
    $("certificateSiteStatus").textContent =
      "当前已启用 HTTPS" +
      (site.certificate ? " · 证书：" + site.certificate : "") +
      (site.redirect_https ? " · HTTP 强制跳转已开启" : " · HTTP 仍可直接访问");
    return;
  }

  $("redirectHTTPS").checked = true;
  if (hasExistingCertificate) {
    $("applyTLSSettingsBtn").textContent = "使用已有证书启用";
    $("certificateSiteStatus").textContent =
      "当前仅 HTTP，但服务器仍有匹配证书；可直接重新启用 HTTPS，也可以重新申请/更新证书。";
  } else {
    $("applyTLSSettingsBtn").textContent = "保存 HTTPS 设置";
    $("certificateSiteStatus").textContent =
      "当前仅 HTTP，未发现匹配证书；需要先申请证书。关闭 HTTPS 不会删除服务器上的证书文件。";
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
    syncCertificateSiteState();
  }
};

$("applyTLSSettingsBtn").onclick = async () => {
  if (!state.selected) {
    $("certificateMessage").textContent = "请先选择一个 CLI 连接。";
    return;
  }
  const site = selectedCertificateSite();
  if (!site) {
    $("certificateMessage").textContent = "请选择一个 Manager 站点。";
    return;
  }

  $("applyTLSSettingsBtn").disabled = true;
  $("certificateMessage").textContent = site.https
    ? "正在验证并应用 HTTPS 跳转设置…"
    : "正在验证已有证书并重新启用 HTTPS…";
  try {
    const updated = await api().UpdateSiteTLS(state.selected, site.id, {
      enabled: true,
      redirect_https: $("redirectHTTPS").checked
    });
    $("certificateMessage").textContent =
      "HTTPS 设置已更新：" + (updated.server_name || updated.id) +
      (updated.redirect_https ? " · 已强制跳转 HTTPS" : " · 保留 HTTP 访问");
    await loadCertificates();
  } catch (err) {
    $("certificateMessage").textContent = cleanError(err);
  } finally {
    syncCertificateSiteState();
  }
};

$("disableHTTPSBtn").onclick = async () => {
  if (!state.selected) {
    $("certificateMessage").textContent = "请先选择一个 CLI 连接。";
    return;
  }
  const site = selectedCertificateSite();
  if (!site?.https) {
    $("certificateMessage").textContent = "当前站点没有启用 HTTPS。";
    return;
  }
  if (!confirm("关闭 “" + site.server_name + "” 的 HTTPS？证书文件会保留，不会删除。")) return;

  $("disableHTTPSBtn").disabled = true;
  $("certificateMessage").textContent = "正在验证并关闭 HTTPS…";
  try {
    const updated = await api().UpdateSiteTLS(state.selected, site.id, {
      enabled: false,
      redirect_https: false
    });
    $("certificateMessage").textContent =
      "HTTPS 已关闭：" + (updated.server_name || updated.id) + "。证书文件仍保留在服务器。";
    await loadCertificates();
  } catch (err) {
    $("certificateMessage").textContent = cleanError(err);
  } finally {
    syncCertificateSiteState();
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
    $("renewCertificatesBtn").disabled = !Boolean(state.certbot?.available);
  }
};


$("refreshLogsBtn").onclick = loadLogs;
$("refreshLogContentBtn").onclick = loadLogTail;
$("logFileSelect").onchange = loadLogTail;
$("logLines").onchange = loadLogTail;
$("logAutoRefresh").onchange = syncLogAutoRefresh;

function stopLogAutoRefresh() {
  if (state.logAutoTimer) {
    clearInterval(state.logAutoTimer);
    state.logAutoTimer = null;
  }
}

function syncLogAutoRefresh() {
  stopLogAutoRefresh();
  if (!$("logAutoRefresh").checked || state.view !== "logs" || !state.selected) {
    return;
  }
  state.logAutoTimer = setInterval(() => {
    if (state.view !== "logs" || !state.selected || $("refreshLogContentBtn").disabled) {
      return;
    }
    loadLogTail();
  }, 10000);
}

$("logSiteFilter").onchange = applyLogFilter;
$("logStatusFilter").onchange = applyLogFilter;
$("logKeyword").oninput = applyLogFilter;
$("clearLogFilterBtn").onclick = () => {
  $("logSiteFilter").value = "";
  $("logStatusFilter").value = "";
  $("logKeyword").value = "";
  applyLogFilter();
};

async function loadLogs() {
  if (!state.selected) {
    stopLogAutoRefresh();
    state.logs = [];
    state.logTail = null;
    renderLogOptions();
    renderLogSiteOptions();
    $("logContent").textContent = "请先选择一个 CLI 连接。";
    $("logMeta").textContent = "尚未读取日志。";
    return;
  }

  $("logMessage").textContent = "正在扫描当前 Nginx 配置中的日志…";
  try {
    state.logs = await api().ListLogs(state.selected);
    state.logs = state.logs || [];
    try {
      const siteResult = await api().ListSites(state.selected);
      state.sites = siteResult.sites || [];
    } catch (_) {
      state.sites = state.sites || [];
    }
    renderLogOptions();
    renderLogSiteOptions();
    if (state.logs.length) {
      await loadLogTail();
    } else {
      state.logTail = null;
      $("logContent").textContent = "没有发现可读取的 Nginx 日志文件。";
      $("logMeta").textContent = "0 个日志文件";
      $("logMessage").textContent = "Nginx 配置中没有发现允许读取的常规日志文件。";
    }
  } catch (err) {
    state.logs = [];
    state.logTail = null;
    renderLogOptions();
    renderLogSiteOptions();
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
    const site = (state.sites || []).find((item) =>
      item.access_log === log.path || item.error_log === log.path
    );
    const kind = log.kind === "error" ? "错误" : "访问";
    option.textContent = site?.server_name
      ? site.server_name + " · " + kind + " · " + log.path
      : kind + " · " + log.path;
    select.appendChild(option);
  }
  if (state.pendingLogPath) {
    const target = state.logs.find((log) => log.path === state.pendingLogPath);
    if (target) {
      select.value = target.id;
      state.pendingLogPath = "";
      $("logSiteFilter").value = "";
      return;
    }
  }
  if (state.logs.some((log) => log.id === previous)) {
    select.value = previous;
  }
}

function renderLogSiteOptions() {
  const select = $("logSiteFilter");
  const previous = select.value;
  const names = [...new Set(
    (state.sites || [])
      .map((site) => String(site.server_name || "").trim())
      .filter((name) => name && name !== "_")
  )].sort();

  select.innerHTML = '<option value="">全部站点</option>';
  for (const name of names) {
    const option = document.createElement("option");
    option.value = name;
    option.textContent = name;
    select.appendChild(option);
  }
  if (state.pendingLogSite && names.includes(state.pendingLogSite)) {
    select.value = state.pendingLogSite;
    state.pendingLogSite = "";
  } else if (names.includes(previous)) {
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
    state.logTail = result;
    applyLogFilter();
    $("logContent").scrollTop = $("logContent").scrollHeight;
  } catch (err) {
    $("logMessage").textContent = cleanError(err);
  } finally {
    $("refreshLogContentBtn").disabled = false;
  }
}

function applyLogFilter() {
  const result = state.logTail;
  if (!result) {
    return;
  }

  const site = $("logSiteFilter").value.trim().toLowerCase();
  const statusClass = $("logStatusFilter").value.trim();
  const keyword = $("logKeyword").value.trim().toLowerCase();
  const source = String(result.content || "");
  const sourceLines = source ? source.split("\n") : [];
  const filtered = sourceLines.filter((line) => {
    const normalized = line.toLowerCase();
    if (site && !normalized.includes(site)) return false;
    if (statusClass) {
      const match = line.match(/"\s+(\d{3})\s+/);
      if (!match || !match[1].startsWith(statusClass)) return false;
    }
    if (keyword && !normalized.includes(keyword)) return false;
    return true;
  });

  $("logContent").textContent = filtered.length ? filtered.join("\n") : "没有匹配的日志行。";

  const kind = result.file?.kind === "error" ? "错误日志" : "访问日志";
  const path = result.file?.path || "";
  const filtering = Boolean(site || statusClass || keyword);
  $("logMeta").textContent =
    kind + " · " + path +
    " · 返回 " + (result.lines || 0) + " 行" +
    (filtering ? " · 匹配 " + filtered.length + " 行" : "") +
    (result.truncated ? " · 已按安全上限截断" : "");

  const notes = [];
  if (result.truncated) {
    notes.push("原始内容已按 1000 行 / 512 KiB 安全上限截断。");
  }
  if (site) {
    notes.push("站点过滤依赖日志行中实际包含域名；当前 log_format 若不含 $host / $http_host，可能无匹配。");
  }
  if (statusClass) {
    notes.push("HTTP 状态过滤按 common/combined access log 的状态码位置匹配；错误日志通常不会命中。");
  }
  if (!notes.length) {
    notes.push(filtering ? "过滤仅在本地 Desktop 中进行。" : "日志读取完成。");
  }
  $("logMessage").textContent = notes.join("\n");
}

function resetProxyEditor() {
  state.editingSiteID = "";
  $("proxyFormTitle").textContent = "新建反向代理";
  $("createProxyBtn").textContent = "创建并应用";
  $("cancelEditProxyBtn").classList.add("hidden");
  $("proxyServerName").value = "";
  $("proxyUpstream").value = "";
  $("proxyWebSocket").checked = true;
  $("proxyMaxBodySize").value = "";
  $("proxyConnectTimeout").value = "";
  $("proxyReadTimeout").value = "";
}

function readProxyInteger(id, label, max) {
  const raw = $(id).value.trim();
  if (!raw) return 0;
  const value = Number(raw);
  if (!Number.isInteger(value) || value < 0 || value > max) {
    throw new Error(label + "必须是 0～" + max + " 的整数");
  }
  return value;
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
      websocket: $("proxyWebSocket").checked,
      max_body_size_mb: readProxyInteger("proxyMaxBodySize", "最大请求体", 10240),
      connect_timeout_seconds: readProxyInteger("proxyConnectTimeout", "连接超时", 300),
      read_timeout_seconds: readProxyInteger("proxyReadTimeout", "读取超时", 86400)
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
