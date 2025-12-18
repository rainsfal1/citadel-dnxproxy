let token = "";
let csrfToken = "";
let mustChange = false;
const policyView = document.getElementById("policyView");
const modal = document.getElementById("modalOverlay");
const modalPwd = document.getElementById("modalPassword");
const statusBar = document.getElementById("statusBar");
const hintDefault = document.getElementById("hintDefault");
const authStatus = document.getElementById("authStatus");

async function api(path, opts = {}) {
  const headers = opts.headers || {};
  headers["Content-Type"] = "application/json";
  if (token) {
    headers["Authorization"] = "Bearer " + token;
  }
  if (csrfToken && (!opts.method || opts.method.toUpperCase() !== "GET")) {
    headers["X-CSRF-Token"] = csrfToken;
  }
  const res = await fetch(path, { ...opts, headers, credentials: "same-origin" });
  if (!res.ok) {
    const txt = await res.text();
    throw new Error(txt || res.statusText);
  }
  if (res.status === 204) return null;
  return res.json();
}

function terminalAlert(message, isError = false, target) {
  const bar = target || statusBar;
  if (!bar) return;
  bar.textContent = message;
  bar.classList.remove("hidden", "error");
  if (isError) {
    bar.classList.add("error");
  }
  setTimeout(() => {
    bar.classList.add("hidden");
  }, 3200);
}

document.getElementById("loginBtn").onclick = async () => {
  try {
    const pwd = document.getElementById("password").value;
    const username = document.getElementById("username").value || "admin";
    if (!pwd) {
      terminalAlert("Password required", true);
      return;
    }
    const resp = await api("/login", {
      method: "POST",
      body: JSON.stringify({ username, password: pwd }),
    });
    token = resp.token || "";
    csrfToken = resp.csrf_token || "";
    mustChange = !!resp.must_change;
    document.getElementById("actions").classList.remove("hidden");
    document.getElementById("auth").style.display = "none";
    const host = window.location.host;
    document.getElementById("bindHintModal").textContent = host;
    if (hintDefault) {
      if (mustChange) hintDefault.classList.remove("hidden");
      else hintDefault.classList.add("hidden");
    }
    if (mustChange) {
      terminalAlert("Default/first-boot password in use: set a new password", true);
      showModal();
    } else {
      terminalAlert("Logged in");
    }
    refreshPolicy();
  } catch (e) {
    terminalAlert("Authentication failed: " + e.message, true, authStatus);
  }
};

// Get Started button
document.getElementById("getStartedBtn").onclick = () => {
  document.getElementById("getStartedModal").classList.remove("hidden");
};

// Settings dropdown toggle
document.getElementById("settingsBtn").onclick = (e) => {
  e.stopPropagation();
  const dropdown = document.getElementById("settingsDropdown");
  dropdown.classList.toggle("hidden");
};

// Close dropdown when clicking outside
document.addEventListener("click", () => {
  const dropdown = document.getElementById("settingsDropdown");
  if (dropdown) dropdown.classList.add("hidden");
});

// Change Password from settings
document.getElementById("changePasswordBtn").onclick = () => {
  document.getElementById("settingsDropdown").classList.add("hidden");
  document.getElementById("passwordModal").classList.remove("hidden");
};

// Logout button
document.getElementById("logoutBtn").onclick = async () => {
  document.getElementById("settingsDropdown").classList.add("hidden");
  try {
    await api("/logout", { method: "POST", body: "{}" });
  } catch (_) {}
  token = "";
  csrfToken = "";
  document.getElementById("actions").classList.add("hidden");
  document.getElementById("auth").style.display = "block";
};

// Modal openers
document.getElementById("addDeviceBtn").onclick = () => {
  resetDeviceForm();
  renderDiscovery(window._policyDevices || {});
  hideManualForm();
  document.getElementById("addDeviceModal").classList.remove("hidden");
};

document.getElementById("showManualBtn").onclick = () => {
  showManualForm();
};

document.getElementById("addUserBtn").onclick = () => {
  document.getElementById("addUserModal").classList.remove("hidden");
  primeDeviceSelect(window._policyDevices || {}, []);
  resetUserForm();
};

document.getElementById("addRuleBtn").onclick = () => {
  resetRuleForm();
  document.getElementById("addRuleModal").classList.remove("hidden");
};

document.getElementById("setPwdBtn").onclick = async () => {
  const pwd = document.getElementById("newPassword").value;
  await handlePasswordChange(pwd);
};

document.getElementById("createUserBtn").onclick = async () => {
  try {
    const name = document.getElementById("userName").value.trim();
    const budget = parseInt(document.getElementById("userBudget").value, 10) || 0;
    const allowRaw = document.getElementById("userAllow").value;
    const selectedDevices = getSelectedDevices();
    if (!name) {
      terminalAlert("User name required", true);
      return;
    }
    const allow = allowRaw
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean)
      .map((ts) => ({ timeSpan: ts, name: ts }));

    await api("/users", {
      method: "POST",
      body: JSON.stringify({
        name,
        daily_budget_minutes: budget,
        allow_windows: allow,
        device_ids: selectedDevices,
      }),
    });

    terminalAlert(`User created: ${name}`);
    resetUserForm();
    document.getElementById("addUserModal").classList.add("hidden");
    refreshPolicy();
  } catch (e) {
    terminalAlert("Failed to create user: " + e.message, true);
  }
};

function resetUserForm() {
  document.getElementById("userName").value = "";
  document.getElementById("userAllow").value = "";
  document.getElementById("userBudget").value = 60;
  document.getElementById("userModalTitle").textContent = "Create User";
  document.getElementById("createUserBtn").textContent = "Create User";
  document.getElementById("createUserBtn").onclick = defaultCreateUser;
  clearDeviceSelection();
}

const defaultCreateUser = document.getElementById("createUserBtn").onclick;

const defaultCreateDevice = document.getElementById("createDeviceBtn").onclick = async () => {
  try {
    const name = document.getElementById("devName").value.trim();
    if (!name) {
      terminalAlert("Device name required", true);
      return;
    }
    const payload = {
      name,
      ip: document.getElementById("devIP").value.trim(),
      mac: document.getElementById("devMAC").value.trim(),
      user_id: document.getElementById("devUser").value.trim(),
    };
    await api("/devices", { method: "POST", body: JSON.stringify(payload) });
    terminalAlert(`Device saved: ${name}`);
    ["devName", "devIP", "devMAC", "devUser"].forEach((id) => (document.getElementById(id).value = ""));
    document.getElementById("addDeviceModal").classList.add("hidden");
    refreshPolicy();
  } catch (e) {
    terminalAlert("Failed to add device: " + e.message, true);
  }
};

const defaultCreateRule = document.getElementById("createRuleBtn").onclick = async () => {
  try {
    const pattern = document.getElementById("rulePattern").value.trim();
    const userId = document.getElementById("ruleUser").value.trim();
    if (!pattern || !userId) {
      terminalAlert("Pattern and User ID required", true);
      return;
    }
    const payload = {
      user_id: userId,
      pattern: pattern,
      action: document.getElementById("ruleAction").value,
    };
    await api("/domainrules", { method: "POST", body: JSON.stringify(payload) });
    terminalAlert(`Rule saved: ${payload.action.toUpperCase()} ${pattern}`);
    ["rulePattern", "ruleUser"].forEach((id) => (document.getElementById(id).value = ""));
    document.getElementById("addRuleModal").classList.add("hidden");
    refreshPolicy();
  } catch (e) {
    terminalAlert("Failed to add rule: " + e.message, true);
  }
};

document.getElementById("refreshBtn").onclick = refreshPolicy;

async function refreshPolicy() {
  try {
    const data = await api("/policy");
    window._policyDevices = data.devices || {};
    renderDevices(data);
    renderUsers(data);
    renderRules(data);
    renderSessions(data);
    renderDiscovery(window._policyDevices || {});
  } catch (e) {
    terminalAlert("Failed to fetch policy data", true);
  }
}

function renderDevices(data) {
  const list = document.getElementById("deviceList");
  list.innerHTML = "";
  const devices = Object.values(data.devices || {})
    .filter((d) => !d.source || d.source === "manual")
    .sort((a, b) => (a.name || a.hostname || "").localeCompare(b.name || b.hostname || ""));

  devices.forEach((d) => {
    const li = document.createElement("li");
    li.className = "user-plain";
    const deviceName = d.name || d.hostname || d.ip || d.mac || d.id;
    li.innerHTML = `
      <span class="user-line">${escape(deviceName)}</span>
      <span class="row-actions slim">
        <button class="icon-plain" title="View" data-device-id="${d.id}" data-action="view">[VIEW]</button>
        <button class="icon-plain" title="Edit" data-device-id="${d.id}" data-action="edit">[EDIT]</button>
        <button class="icon-plain danger" title="Delete" data-device-id="${d.id}" data-action="delete">[DEL]</button>
      </span>`;
    list.appendChild(li);
  });

  list.onclick = async (e) => {
    const action = e.target.getAttribute("data-action");
    const deviceId = e.target.getAttribute("data-device-id");
    if (!action || !deviceId) return;

    if (action === "view") {
      openDeviceView(deviceId, data);
    } else if (action === "edit") {
      openDeviceEdit(deviceId, data);
    } else if (action === "delete") {
      if (!confirm("Delete this device?")) return;
      try {
        await api(`/devices/${deviceId}`, { method: "DELETE" });
        terminalAlert("Device deleted");
        refreshPolicy();
      } catch (err) {
        terminalAlert("Failed to delete device: " + err.message, true);
      }
    }
  };
}

function renderDiscovery(devicesMap) {
  const list = document.getElementById("discoveryList");
  const search = document.getElementById("discoverySearch");
  if (!list || !search) return;

  const discovered = Object.values(devicesMap || {}).filter((d) => d.source && d.source !== "manual");

  const render = (filter) => {
    list.innerHTML = "";
    const subset = discovered.filter((d) => {
      if (!filter) return true;
      const text = `${d.name} ${d.hostname} ${d.ip} ${d.mac} ${d.source}`.toLowerCase();
      return text.includes(filter.toLowerCase());
    });
    if (subset.length === 0) {
      list.innerHTML = `<div class="no-devices-msg">No LAN devices found. Add manually below.</div>`;
      return;
    }
    subset.forEach((d) => {
      const item = document.createElement("div");
      item.className = "scroll-item spaced";
      const label = d.name || d.hostname || d.ip || d.mac || d.id;
      item.innerHTML = `<div><div class="user-line">${escape(label)}</div><div class="muted">${escape(d.ip || "-")} · ${escape(d.mac || "-")} · ${escape(d.source || "")}</div></div><button class="icon-plain" data-id="${d.id}" data-action="use">Use</button>`;
      list.appendChild(item);
    });
  };

  render("");

  // Show manual form only if there are truly no discovered devices at all
  if (discovered.length === 0) {
    showManualForm();
  }

  list.onclick = (e) => {
    const id = e.target.getAttribute && e.target.getAttribute("data-id");
    if (!id) return;
    const dev = discovered.find((d) => d.id === id);
    if (!dev) return;
    showManualForm();
    document.getElementById("devName").value = dev.name || dev.hostname || "";
    document.getElementById("devIP").value = dev.ip || "";
    document.getElementById("devMAC").value = dev.mac || "";
    document.getElementById("devUser").value = dev.user_id || "";
  };

  search.oninput = (e) => {
    render(e.target.value || "");
  };
}

function openDeviceView(deviceId, data) {
  const device = (data.devices || {})[deviceId];
  if (!device) return;

  document.getElementById("viewDevName").textContent = device.name || device.hostname || "-";
  document.getElementById("viewDevIP").textContent = device.ip || "-";
  document.getElementById("viewDevMAC").textContent = device.mac || "-";
  document.getElementById("viewDevUser").textContent = device.user_id || "-";
  document.getElementById("viewDevLastSeen").textContent = device.last_seen || "-";

  document.getElementById("viewDeviceModal").classList.remove("hidden");
}

function openDeviceEdit(deviceId, data) {
  const device = (data.devices || {})[deviceId];
  if (!device) return;

  document.getElementById("deviceModalTitle").textContent = "Edit Device";
  showManualForm();
  document.getElementById("devName").value = device.name || device.hostname || "";
  document.getElementById("devIP").value = device.ip || "";
  document.getElementById("devMAC").value = device.mac || "";
  document.getElementById("devUser").value = device.user_id || "";

  const modal = document.getElementById("addDeviceModal");
  modal.classList.remove("hidden");

  const btn = document.getElementById("createDeviceBtn");
  btn.textContent = "Update Device";
  btn.onclick = async () => {
    try {
      const name = document.getElementById("devName").value.trim();
      if (!name) {
        terminalAlert("Device name required", true);
        return;
      }
      const payload = {
        name,
        ip: document.getElementById("devIP").value.trim(),
        mac: document.getElementById("devMAC").value.trim(),
        user_id: document.getElementById("devUser").value.trim(),
      };
      await api(`/devices/${deviceId}`, { method: "PUT", body: JSON.stringify(payload) });
      terminalAlert("Device updated");
      resetDeviceForm();
      document.getElementById("addDeviceModal").classList.add("hidden");
      refreshPolicy();
    } catch (err) {
      terminalAlert("Failed to update device: " + err.message, true);
    }
  };
}

function resetDeviceForm() {
  document.getElementById("deviceModalTitle").textContent = "Add Device";
  document.getElementById("devName").value = "";
  document.getElementById("devIP").value = "";
  document.getElementById("devMAC").value = "";
  document.getElementById("devUser").value = "";
  document.getElementById("createDeviceBtn").textContent = "Save Device";
  document.getElementById("createDeviceBtn").onclick = defaultCreateDevice;
  hideManualForm();
}

function showManualForm() {
  const form = document.getElementById("manualDeviceForm");
  if (form) form.classList.remove("hidden");
}

function hideManualForm() {
  const form = document.getElementById("manualDeviceForm");
  if (form) form.classList.add("hidden");
}

function renderUsers(data) {
  const list = document.getElementById("userList");
  list.innerHTML = "";
  const devMap = data.devices || {};
  const users = Object.values(data.users || {}).sort((a, b) => a.name.localeCompare(b.name));
  users.forEach((u) => {
    const li = document.createElement("li");
    const devices = (u.device_ids || [])
      .map((id) => {
        const d = devMap[id] || {};
        return d.name || d.hostname || d.ip || d.mac || id;
      })
      .join(", ");
    li.className = "user-plain";
    li.innerHTML = `
      <span class="user-line">${escape(u.name || u.id)} — budget ${u.daily_budget_minutes} min${devices ? ` · ${escape(devices)}` : ""}</span>
      <span class="row-actions slim">
        <button class="icon-plain" title="Edit" data-user-id="${u.id}" data-action="edit">[EDIT]</button>
        <button class="icon-plain danger" title="Delete" data-user-id="${u.id}" data-action="delete">[DEL]</button>
      </span>`;
    list.appendChild(li);
  });

  list.onclick = async (e) => {
    const action = e.target.getAttribute("data-action");
    const uid = e.target.getAttribute("data-user-id");
    if (!action || !uid) return;
    if (action === "delete") {
      if (!confirm("Delete this user?")) return;
      try {
        await api(`/users/${uid}`, { method: "DELETE" });
        terminalAlert("User deleted");
        refreshPolicy();
      } catch (err) {
        terminalAlert("Failed to delete user: " + err.message, true);
      }
      return;
    }
    if (action === "edit") {
      openUserEdit(uid, data);
    }
  };
}

function openUserEdit(userId, data) {
  const user = (data.users || {})[userId];
  if (!user) return;
  const modal = document.getElementById("addUserModal");
  modal.classList.remove("hidden");
  document.getElementById("userName").value = user.name || "";
  document.getElementById("userBudget").value = user.daily_budget_minutes || 0;
  document.getElementById("userAllow").value = (user.allow_windows || []).map((w) => w.timespan || w.timeSpan || "").filter(Boolean).join(", ");
  primeDeviceSelect(data.devices || {}, user.device_ids || []);
  document.getElementById("userModalTitle").textContent = "Update User";
  const btn = document.getElementById("createUserBtn");
  btn.textContent = "Update User";
  btn.onclick = async () => {
    try {
      const name = document.getElementById("userName").value.trim();
      const budget = parseInt(document.getElementById("userBudget").value, 10) || 0;
      const allowRaw = document.getElementById("userAllow").value;
      if (!name) {
        terminalAlert("User name required", true);
        return;
      }
      const allow = allowRaw
        .split(",")
        .map((s) => s.trim())
        .filter(Boolean)
        .map((ts) => ({ timeSpan: ts, name: ts }));
      const selectedDevices = getSelectedDevices();
      await api(`/users/${userId}`, {
        method: "PUT",
        body: JSON.stringify({
          name,
          daily_budget_minutes: budget,
          allow_windows: allow,
          device_ids: selectedDevices,
        }),
      });
      terminalAlert("User updated");
      resetUserForm();
      document.getElementById("addUserModal").classList.add("hidden");
      refreshPolicy();
    } catch (err) {
      terminalAlert("Failed to update user: " + err.message, true);
    }
  };
}

function renderRules(data) {
  const list = document.getElementById("rulesList");
  list.innerHTML = "";
  const rules = data.domain_rules || [];

  rules.forEach((r) => {
    const li = document.createElement("li");
    li.className = "user-plain";
    li.innerHTML = `
      <span class="user-line">${escape(r.pattern)}</span>
      <span class="row-actions slim">
        <button class="icon-plain" title="View" data-rule-id="${r.id}" data-action="view">[VIEW]</button>
        <button class="icon-plain" title="Edit" data-rule-id="${r.id}" data-action="edit">[EDIT]</button>
        <button class="icon-plain danger" title="Delete" data-rule-id="${r.id}" data-action="delete">[DEL]</button>
      </span>`;
    list.appendChild(li);
  });

  list.onclick = async (e) => {
    const action = e.target.getAttribute("data-action");
    const ruleId = e.target.getAttribute("data-rule-id");
    if (!action || !ruleId) return;

    if (action === "view") {
      openRuleView(ruleId, data);
    } else if (action === "edit") {
      openRuleEdit(ruleId, data);
    } else if (action === "delete") {
      if (!confirm("Delete this rule?")) return;
      try {
        await api(`/domainrules/${ruleId}`, { method: "DELETE" });
        terminalAlert("Rule deleted");
        refreshPolicy();
      } catch (err) {
        terminalAlert("Failed to delete rule: " + err.message, true);
      }
    }
  };
}

function openRuleView(ruleId, data) {
  const rule = (data.domain_rules || []).find(r => r.id === ruleId);
  if (!rule) return;

  const user = (data.users || {})[rule.user_id];
  const userName = user ? user.name : rule.user_id;

  document.getElementById("viewRulePattern").textContent = rule.pattern || "-";
  document.getElementById("viewRuleAction").textContent = rule.action || "-";
  document.getElementById("viewRuleUser").textContent = `${rule.user_id} (${userName})`;

  document.getElementById("viewRuleModal").classList.remove("hidden");
}

function openRuleEdit(ruleId, data) {
  const rule = (data.domain_rules || []).find(r => r.id === ruleId);
  if (!rule) return;

  document.getElementById("ruleModalTitle").textContent = "Edit Domain Rule";
  document.getElementById("ruleUser").value = rule.user_id || "";
  document.getElementById("rulePattern").value = rule.pattern || "";
  document.getElementById("ruleAction").value = rule.action || "block";

  const modal = document.getElementById("addRuleModal");
  modal.classList.remove("hidden");

  const btn = document.getElementById("createRuleBtn");
  btn.textContent = "Update Rule";
  btn.onclick = async () => {
    try {
      const pattern = document.getElementById("rulePattern").value.trim();
      const userId = document.getElementById("ruleUser").value.trim();
      if (!pattern || !userId) {
        terminalAlert("Pattern and User ID required", true);
        return;
      }
      const payload = {
        user_id: userId,
        pattern: pattern,
        action: document.getElementById("ruleAction").value,
      };
      await api(`/domainrules/${ruleId}`, { method: "PUT", body: JSON.stringify(payload) });
      terminalAlert("Rule updated");
      resetRuleForm();
      document.getElementById("addRuleModal").classList.add("hidden");
      refreshPolicy();
    } catch (err) {
      terminalAlert("Failed to update rule: " + err.message, true);
    }
  };
}

function resetRuleForm() {
  document.getElementById("ruleModalTitle").textContent = "Add Domain Rule";
  document.getElementById("ruleUser").value = "";
  document.getElementById("rulePattern").value = "";
  document.getElementById("ruleAction").value = "block";
  document.getElementById("createRuleBtn").textContent = "Add Rule";
  document.getElementById("createRuleBtn").onclick = defaultCreateRule;
}

function renderSessions(data) {
  const tbody = document.querySelector("#sessionTable tbody");
  tbody.innerHTML = "";
  const users = data.users || {};
  const sessions = data.sessions || {};
  const usage = data.usage || {};
  Object.values(sessions).forEach((s) => {
    if (!s.active) return;
    const user = users[s.user_id] || {};
    const us = usage[s.user_id] || { seconds: 0 };
    const tr = document.createElement("tr");
    const userName = user.name || s.user_id;
    const devices = (user.device_ids || []).join(", ");
    tr.innerHTML = `<td>${escape(userName)}</td><td>${escape(devices)}</td><td>${us.seconds || 0}</td><td>${(user.daily_budget_minutes || 0) * 60}</td>`;
    tbody.appendChild(tr);
  });
}

function primeDeviceSelect(devicesMap, preselectIds) {
  const container = document.getElementById("userDeviceSelect");
  const search = document.getElementById("userDeviceSearch");
  if (!container || !search) return;
  container.innerHTML = "";
  const devices = Object.values(devicesMap || {}).sort((a, b) => (a.name || "").localeCompare(b.name || ""));
  const selected = new Set(preselectIds || []);

  const renderList = (filter) => {
    container.innerHTML = "";

    // Don't show anything if search is empty
    if (!filter || filter.trim() === "") {
      container.classList.add("hidden");
      return;
    }

    container.classList.remove("hidden");

    const filtered = devices.filter((d) => {
      const text = `${d.name} ${d.hostname} ${d.ip} ${d.mac}`.toLowerCase();
      return text.includes(filter.toLowerCase());
    });

    if (filtered.length === 0) {
      container.innerHTML = `<div class="no-devices-msg">No devices found</div>`;
      return;
    }

    filtered.forEach((d) => {
      const item = document.createElement("div");
      item.className = "scroll-item device-row";
      const checked = selected.has(d.id) ? "checked" : "";
      item.innerHTML = `
        <div class="device-checkbox">
          <input type="checkbox" data-id="${d.id}" ${checked}>
        </div>
        <div class="device-info">
          <div class="device-name">${escape(d.name || d.hostname || d.ip || d.mac || d.id)}</div>
          <div class="device-meta muted">${escape(d.ip || "")} · ${escape(d.mac || "")}</div>
        </div>
      `;
      container.appendChild(item);
    });
  };

  // Initially hide the container
  container.classList.add("hidden");

  container.onclick = (e) => {
    const id = e.target.getAttribute && e.target.getAttribute("data-id");
    if (!id) return;
    if (e.target.checked) selected.add(id);
    else selected.delete(id);
  };

  search.oninput = (e) => {
    renderList(e.target.value || "");
  };

  container.dataset.selected = JSON.stringify(Array.from(selected));
}

function getSelectedDevices() {
  const container = document.getElementById("userDeviceSelect");
  if (!container) return [];
  const checks = container.querySelectorAll("input[type=checkbox]:checked");
  return Array.from(checks).map((c) => c.getAttribute("data-id"));
}

function clearDeviceSelection() {
  const container = document.getElementById("userDeviceSelect");
  const search = document.getElementById("userDeviceSearch");
  if (container) container.innerHTML = "";
  if (search) search.value = "";
}

function escape(text) {
  if (!text) return "";
  return String(text)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
}

function showModal() {
  modal.classList.remove("hidden");
  modalPwd.focus();
}

function hideModal() {
  modal.classList.add("hidden");
}

document.getElementById("modalSaveBtn").onclick = async () => {
  await handlePasswordChange(modalPwd.value.trim());
};

async function handlePasswordChange(pwd) {
  if (!pwd || pwd.length < 6) {
    terminalAlert("Password too short (min 6 chars)", true);
    return;
  }
  try {
    await api("/admin/password", { method: "POST", body: JSON.stringify({ password: pwd }) });
    terminalAlert("Password updated. Please log in with the new password.");
    document.getElementById("newPassword").value = "";
    modalPwd.value = "";
    mustChange = false;
    if (hintDefault) hintDefault.classList.add("hidden");
    hideModal();
    document.getElementById("passwordModal").classList.add("hidden");
    // Clear session to force fresh login with new credentials
    token = "";
    csrfToken = "";
    await api("/logout", { method: "POST", body: "{}" }).catch(() => {});
    document.getElementById("actions").classList.add("hidden");
    document.getElementById("auth").style.display = "block";
  } catch (e) {
    terminalAlert("Failed to update password: " + e.message, true);
  }
}
