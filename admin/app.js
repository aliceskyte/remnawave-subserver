const els = {
  loginPanel: document.getElementById("login-panel"),
  adminContent: document.getElementById("admin-content"),
  tokenInput: document.getElementById("token"),
  tokenCheck: document.getElementById("token-check"),
  loginStatus: document.getElementById("login-status"),
  toastStack: document.getElementById("toast-stack"),
  configList: document.getElementById("config-list"),
  configName: document.getElementById("config-name"),
  configNameChip: document.getElementById("config-name-chip"),
  configNameDisplay: document.getElementById("config-name-display"),
  configNameEditor: document.getElementById("config-name-editor"),
  configRename: document.getElementById("config-rename"),
  configRenameApply: document.getElementById("config-rename-apply"),
  configRenameCancel: document.getElementById("config-rename-cancel"),
  configJson: document.getElementById("config-json"),
  configNew: document.getElementById("config-new"),
  configFormat: document.getElementById("config-format"),
  configSave: document.getElementById("config-save"),
  configDelete: document.getElementById("config-delete"),
  configExpand: document.getElementById("config-expand"),
  configCount: document.getElementById("config-count"),
  configSwitcher: document.getElementById("config-switcher"),
  configSwitcherTrigger: document.getElementById("config-switcher-trigger"),
  configSwitcherValue: document.getElementById("config-switcher-value"),
  configSwitcherNote: document.getElementById("config-switcher-note"),
  configPanelTitle: document.getElementById("config-panel-title"),
  configPanelSubtitle: document.getElementById("config-panel-subtitle"),
  configFieldLabel: document.getElementById("config-field-label"),
  headerPanelTitle: document.getElementById("header-panel-title"),
  headerPanelSubtitle: document.getElementById("header-panel-subtitle"),
  coreSwitcher: document.getElementById("core-switcher"),
  coreSwitcherTrigger: document.getElementById("core-switcher-trigger"),
  coreSwitcherValue: document.getElementById("core-switcher-value"),
  coreOptions: document.getElementById("core-options"),
  coreSelect: document.getElementById("core-select"),
  squadSwitcher: document.getElementById("squad-switcher"),
  squadSwitcherTrigger: document.getElementById("squad-switcher-trigger"),
  squadSwitcherValue: document.getElementById("squad-switcher-value"),
  squadOptions: document.getElementById("squad-options"),
  squadSelect: document.getElementById("squad-select"),
  headerList: document.getElementById("header-list"),
  headerAdd: document.getElementById("header-add"),
  headerSave: document.getElementById("header-save"),
  headerFetch: document.getElementById("header-fetch"),
  headerUuid: document.getElementById("header-uuid"),
  modalOverlay: document.getElementById("modal-overlay"),
  modalContainer: document.getElementById("modal-container"),
  modalClose: document.getElementById("modal-close"),
  modalFormat: document.getElementById("modal-format"),
  modalSave: document.getElementById("modal-save"),
  modalDelete: document.getElementById("modal-delete"),
  modalConfigName: document.getElementById("modal-config-name"),
  modalConfigJson: document.getElementById("modal-config-json"),
  modalSubtitle: document.getElementById("modal-subtitle"),
  codeHighlight: document.getElementById("code-highlight"),
  lineNumbers: document.getElementById("line-numbers"),
  editorStatus: document.getElementById("editor-status"),
  editorLangBadge: document.getElementById("editor-lang-badge"),
};

// Keep the admin token scoped to the active tab and migrate old localStorage values once.
const TOKEN_KEY = "subserver_admin_token";

function createEmptyScopedSet() {
  return { default: [], squads: {} };
}

const state = {
  currentCore: "xray",
  coreMenuOpen: false,
  configSets: { xray: createEmptyScopedSet(), mihomo: createEmptyScopedSet() },
  headerSets: { xray: createEmptyScopedSet(), mihomo: createEmptyScopedSet() },
  actualHeaders: {},
  actualHeadersLower: {},
  currentConfigIndex: null,
  configMenuOpen: false,
  configNameEditorOpen: false,
  configNameBeforeEdit: "",
  squadMenuOpen: false,
  currentSquad: "default",
  squads: [],
};

const entryKeyMap = new WeakMap();
let entryKeySeed = 1;
const dragState = { index: null, key: null };

const DEFAULT_CORE_KEY = "xray";
const DEFAULT_SQUAD_KEY = "default";
const DEFAULT_SQUAD_LABEL = "Неизвестный";
const CORE_OPTIONS = [
  {
    value: "xray",
    label: "Xray",
    hint: "JSON-конфиги и Xray-специфичные шаблоны",
    format: "json",
  },
  {
    value: "mihomo",
    label: "Mihomo",
    hint: "YAML-конфиги и Mihomo-специфичные шаблоны",
    format: "yaml",
  },
];

const TOAST_LIMIT = 4;
const TOAST_DURATION = 3000;
const TOAST_ERROR_DURATION = 3000;
const TOAST_EXIT_DELAY = 300;

function detectToastTone(message) {
  const text = String(message || "").toLowerCase();
  if (!text) return "info";
  if (text.includes("ошибка") || text.includes("error") || text.includes("failed")) {
    return "error";
  }
  if (
    text.includes("сохран") ||
    text.includes("готов") ||
    text.includes("удален") ||
    text.includes("получен") ||
    text.includes("обнов") ||
    text.includes("успеш")
  ) {
    return "success";
  }
  return "info";
}

function showToast(message, tone) {
  if (!els.toastStack) return;
  const text = String(message || "").trim();
  if (!text) return;
  const finalTone = tone || detectToastTone(text);
  const toast = document.createElement("div");
  toast.className = `toast ${finalTone}`;

  const dot = document.createElement("span");
  dot.className = "toast-dot";
  const textNode = document.createElement("span");
  textNode.className = "toast-text";
  textNode.textContent = text;
  toast.appendChild(dot);
  toast.appendChild(textNode);

  const existing = Array.from(els.toastStack.children);
  if (existing.length >= TOAST_LIMIT) {
    existing[0].remove();
  }

  els.toastStack.appendChild(toast);
  requestAnimationFrame(() => {
    toast.classList.add("show");
  });

  const ttl = finalTone === "error" ? TOAST_ERROR_DURATION : TOAST_DURATION;
  window.setTimeout(() => {
    toast.classList.add("hide");
    window.setTimeout(() => {
      toast.remove();
    }, TOAST_EXIT_DELAY);
  }, ttl);
}

function setStatus(message, tone) {
  showToast(message, tone);
}

function setLoginStatus(message) {
  if (els.loginStatus) {
    els.loginStatus.textContent = message;
  }
}

function getToken() {
  return els.tokenInput.value.trim();
}

function saveToken(token) {
  if (!token) return;
  sessionStorage.setItem(TOKEN_KEY, token);
  localStorage.removeItem(TOKEN_KEY);
}

function loadToken() {
  let token = sessionStorage.getItem(TOKEN_KEY);
  if (!token) {
    token = localStorage.getItem(TOKEN_KEY);
    if (token) {
      sessionStorage.setItem(TOKEN_KEY, token);
      localStorage.removeItem(TOKEN_KEY);
    }
  }
  if (token) {
    els.tokenInput.value = token;
  }
  return token;
}

function getCoreMeta(core = state.currentCore) {
  return CORE_OPTIONS.find((item) => item.value === core) || CORE_OPTIONS[0];
}

function isMihomoCore(core = state.currentCore) {
  return core === "mihomo";
}

function getCoreScopedSet(source, core = state.currentCore) {
  if (!source[core]) {
    source[core] = createEmptyScopedSet();
  }
  if (!source[core].default) {
    source[core].default = [];
  }
  if (!source[core].squads || typeof source[core].squads !== "object") {
    source[core].squads = {};
  }
  return source[core];
}

function getCurrentConfigList() {
  const scoped = getCoreScopedSet(state.configSets);
  if (state.currentSquad === DEFAULT_SQUAD_KEY) {
    return scoped.default;
  }
  if (!scoped.squads[state.currentSquad]) {
    scoped.squads[state.currentSquad] = [];
  }
  return scoped.squads[state.currentSquad];
}

function getCurrentConfigEntry() {
  if (state.currentConfigIndex === null) return null;
  return getCurrentConfigList()[state.currentConfigIndex] || null;
}

function setConfigMenuOpen(open) {
  const finalState = Boolean(open);
  state.configMenuOpen = finalState;
  if (els.configSwitcher) {
    els.configSwitcher.classList.toggle("open", finalState);
  }
  if (els.configSwitcherTrigger) {
    els.configSwitcherTrigger.setAttribute("aria-expanded", finalState ? "true" : "false");
  }
}

function setCoreMenuOpen(open) {
  const finalState = Boolean(open);
  state.coreMenuOpen = finalState;
  if (els.coreSwitcher) {
    els.coreSwitcher.classList.toggle("open", finalState);
  }
  if (els.coreSwitcherTrigger) {
    els.coreSwitcherTrigger.setAttribute("aria-expanded", finalState ? "true" : "false");
  }
}

function getDraftConfigName() {
  if (!els.configName) return "";
  return els.configName.value.trim();
}

function syncConfigNameDisplay() {
  if (els.configNameDisplay) {
    els.configNameDisplay.textContent = getDraftConfigName() || "Новый конфиг";
  }
  if (els.configNameChip) {
    els.configNameChip.classList.toggle("empty", !getDraftConfigName());
  }
}

function setConfigNameEditorOpen(open, { focus = false } = {}) {
  const finalState = Boolean(open);
  state.configNameEditorOpen = finalState;
  if (els.configNameEditor) {
    els.configNameEditor.classList.toggle("hidden", !finalState);
  }
  if (els.configNameChip) {
    els.configNameChip.classList.toggle("editing", finalState);
  }
  if (finalState && focus && els.configName) {
    requestAnimationFrame(() => {
      els.configName.focus();
      els.configName.select();
    });
  }
}

function beginConfigRename() {
  state.configNameBeforeEdit = els.configName ? els.configName.value : "";
  setConfigNameEditorOpen(true, { focus: true });
}

function applyConfigRename() {
  syncConfigNameDisplay();
  syncConfigSwitcher();
  setConfigNameEditorOpen(false);
}

function cancelConfigRename() {
  if (els.configName) {
    els.configName.value = state.configNameBeforeEdit || "";
  }
  syncConfigNameDisplay();
  syncConfigSwitcher();
  setConfigNameEditorOpen(false);
}

function syncConfigSwitcher() {
  const configs = getCurrentConfigList();
  const entry = getCurrentConfigEntry();
  const draftName = getDraftConfigName();

  if (els.configSwitcherValue) {
    if (entry) {
      els.configSwitcherValue.textContent = entry.name || "Без названия";
    } else if (draftName) {
      els.configSwitcherValue.textContent = draftName;
    } else if (configs.length) {
      els.configSwitcherValue.textContent = "Выбери конфиг";
    } else {
      els.configSwitcherValue.textContent = "Новый конфиг";
    }
  }

  if (els.configSwitcherNote) {
    if (entry) {
      els.configSwitcherNote.textContent = "Редактируется сейчас";
    } else if (draftName) {
      els.configSwitcherNote.textContent = "Черновик";
    } else if (configs.length) {
      els.configSwitcherNote.textContent = `${configs.length} в списке`;
    } else {
      els.configSwitcherNote.textContent = "Список пуст";
    }
  }
}

function getCoreLabel(value) {
  return getCoreMeta(value).label;
}

function syncCoreSwitcher() {
  if (!els.coreSwitcherValue) return;
  els.coreSwitcherValue.textContent = getCoreLabel(state.currentCore);
}

function updateCoreUI() {
  const meta = getCoreMeta();
  if (els.configPanelTitle) {
    els.configPanelTitle.textContent = `Конфиги ${meta.label}`;
  }
  if (els.configPanelSubtitle) {
    els.configPanelSubtitle.textContent =
      meta.value === "mihomo"
        ? "Управление YAML-шаблонами и профилями Mihomo"
        : "Управление JSON-конфигурациями серверов";
  }
  if (els.headerPanelTitle) {
    els.headerPanelTitle.textContent = `Хедеры ${meta.label}`;
  }
  if (els.headerPanelSubtitle) {
    els.headerPanelSubtitle.textContent =
      meta.value === "mihomo"
        ? "Переопределение HTTP-заголовков подписки Mihomo"
        : "Переопределение HTTP-заголовков подписки Xray";
  }
  if (els.configFieldLabel) {
    els.configFieldLabel.textContent = meta.value === "mihomo" ? "YAML-конфиг" : "JSON-конфиг";
  }
  if (els.editorLangBadge) {
    els.editorLangBadge.textContent = meta.format.toUpperCase();
  }
  if (els.configJson) {
    els.configJson.placeholder =
      meta.value === "mihomo" ? "type: vless\nserver: example.com" : '{ "outbounds": [ ... ] }';
  }
  if (els.modalConfigJson) {
    els.modalConfigJson.placeholder =
      meta.value === "mihomo" ? "type: vless\nserver: example.com" : '{ "outbounds": [ ... ] }';
  }
  updateHighlight();
}

function selectCore(value) {
  const validValue = CORE_OPTIONS.some((item) => item.value === value) ? value : DEFAULT_CORE_KEY;
  const changed = state.currentCore !== validValue;
  state.currentCore = validValue;
  if (els.coreSelect) {
    els.coreSelect.value = validValue;
  }
  setCoreMenuOpen(false);
  renderCoreSelect();
  updateCoreUI();
  if (!changed) return;
  clearConfigEditor();
  renderHeaders();
}

function setSquadMenuOpen(open) {
  const finalState = Boolean(open);
  state.squadMenuOpen = finalState;
  if (els.squadSwitcher) {
    els.squadSwitcher.classList.toggle("open", finalState);
  }
  if (els.squadSwitcherTrigger) {
    els.squadSwitcherTrigger.setAttribute("aria-expanded", finalState ? "true" : "false");
  }
}

function getSquadLabel(value) {
  if (!value || value === DEFAULT_SQUAD_KEY) {
    return DEFAULT_SQUAD_LABEL;
  }
  const squad = state.squads.find((item) => item.uuid === value);
  if (!squad) {
    return DEFAULT_SQUAD_LABEL;
  }
  return squad.name || squad.uuid;
}

function syncSquadSwitcher() {
  if (!els.squadSwitcherValue) return;
  els.squadSwitcherValue.textContent = getSquadLabel(state.currentSquad);
}

function selectSquad(value) {
  const validValue =
    value === DEFAULT_SQUAD_KEY || state.squads.some((squad) => squad.uuid === value)
      ? value
      : DEFAULT_SQUAD_KEY;
  const changed = state.currentSquad !== validValue;
  state.currentSquad = validValue;
  if (els.squadSelect) {
    els.squadSelect.value = validValue;
  }
  setSquadMenuOpen(false);
  renderSquadSelect();
  if (!changed) return;
  clearConfigEditor();
  renderHeaders();
}

function getCurrentHeaderList() {
  const scoped = getCoreScopedSet(state.headerSets);
  if (state.currentSquad === DEFAULT_SQUAD_KEY) {
    return scoped.default;
  }
  if (!scoped.squads[state.currentSquad]) {
    scoped.squads[state.currentSquad] = [];
  }
  return scoped.squads[state.currentSquad];
}

function renderCoreSelect() {
  if (els.coreSelect) {
    els.coreSelect.innerHTML = "";
    CORE_OPTIONS.forEach((option) => {
      const item = document.createElement("option");
      item.value = option.value;
      item.textContent = option.label;
      els.coreSelect.appendChild(item);
    });
    els.coreSelect.value = state.currentCore || DEFAULT_CORE_KEY;
  }

  if (els.coreOptions) {
    els.coreOptions.innerHTML = "";
    CORE_OPTIONS.forEach((option, index) => {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "squad-option";
      button.dataset.value = option.value;
      button.setAttribute("role", "option");
      button.setAttribute("aria-selected", option.value === state.currentCore ? "true" : "false");
      button.style.setProperty("--item-delay", `${index * 28}ms`);
      if (option.value === state.currentCore) {
        button.classList.add("active");
      }

      const copy = document.createElement("span");
      copy.className = "squad-option-copy";
      const title = document.createElement("span");
      title.className = "squad-option-title";
      title.textContent = option.label;
      const hint = document.createElement("span");
      hint.className = "squad-option-hint";
      hint.textContent = option.hint;
      copy.appendChild(title);
      copy.appendChild(hint);

      button.appendChild(copy);
      button.addEventListener("click", () => {
        selectCore(option.value);
      });
      els.coreOptions.appendChild(button);
    });
  }

  syncCoreSwitcher();
}

function renderSquadSelect() {
  const hasCurrent =
    state.currentSquad &&
    (state.currentSquad === DEFAULT_SQUAD_KEY ||
      state.squads.some((squad) => squad.uuid === state.currentSquad));
  if (!hasCurrent) {
    state.currentSquad = DEFAULT_SQUAD_KEY;
  }

  if (els.squadSelect) {
    els.squadSelect.innerHTML = "";
    const unknownOption = document.createElement("option");
    unknownOption.value = DEFAULT_SQUAD_KEY;
    unknownOption.textContent = DEFAULT_SQUAD_LABEL;
    els.squadSelect.appendChild(unknownOption);

    state.squads.forEach((squad) => {
      const option = document.createElement("option");
      option.value = squad.uuid;
      option.textContent = squad.name || squad.uuid;
      els.squadSelect.appendChild(option);
    });

    els.squadSelect.value = state.currentSquad || DEFAULT_SQUAD_KEY;
  }

  if (els.squadOptions) {
    els.squadOptions.innerHTML = "";
    const options = [
      { value: DEFAULT_SQUAD_KEY, label: DEFAULT_SQUAD_LABEL, hint: "Для неизвестных пользователей" },
      ...state.squads.map((squad) => ({
        value: squad.uuid,
        label: squad.name || squad.uuid,
        hint: squad.uuid,
      })),
    ];

    options.forEach((option, index) => {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "squad-option";
      button.dataset.value = option.value;
      button.setAttribute("role", "option");
      button.setAttribute("aria-selected", option.value === state.currentSquad ? "true" : "false");
      button.style.setProperty("--item-delay", `${index * 28}ms`);
      if (option.value === state.currentSquad) {
        button.classList.add("active");
      }

      const copy = document.createElement("span");
      copy.className = "squad-option-copy";
      const title = document.createElement("span");
      title.className = "squad-option-title";
      title.textContent = option.label;
      const hint = document.createElement("span");
      hint.className = "squad-option-hint";
      hint.textContent = option.hint;
      copy.appendChild(title);
      copy.appendChild(hint);

      button.appendChild(copy);
      button.addEventListener("click", () => {
        selectSquad(option.value);
      });
      els.squadOptions.appendChild(button);
    });
  }

  syncSquadSwitcher();
}

const USERINFO_KEYS = ["upload", "download", "total", "expire"];

function normalizeMode(mode) {
  if (!mode) return "custom";
  const clean = String(mode).trim().toLowerCase();
  return clean === "actual" ? "actual" : "custom";
}

function isUserinfoKey(key) {
  return String(key || "").trim().toLowerCase() === "subscription-userinfo";
}

function setActualHeaders(headers) {
  state.actualHeaders = headers || {};
  state.actualHeadersLower = {};
  Object.entries(state.actualHeaders).forEach(([key, value]) => {
    if (!key) return;
    state.actualHeadersLower[String(key).toLowerCase()] = value;
  });
}

function getActualHeaderValue(key) {
  if (!key) return "";
  const lowered = String(key).toLowerCase();
  if (state.actualHeadersLower && lowered in state.actualHeadersLower) {
    return state.actualHeadersLower[lowered] || "";
  }
  return "";
}

function parseUserinfo(value) {
  const result = {};
  if (!value) return result;
  String(value)
    .split(";")
    .forEach((part) => {
      const trimmed = part.trim();
      if (!trimmed) return;
      const pair = trimmed.split("=");
      if (pair.length < 2) return;
      const key = pair.shift().trim().toLowerCase();
      const val = pair.join("=").trim();
      if (!key) return;
      result[key] = val;
    });
  return result;
}

function ensureUserinfoParams(entry, actualParams) {
  if (!entry.params) entry.params = {};
  USERINFO_KEYS.forEach((key) => {
    const existing = entry.params[key];
    const hasActual = actualParams && actualParams[key] !== undefined;
    if (existing && typeof existing === "object") {
      if (!existing.mode) {
        existing.mode = hasActual ? "actual" : "custom";
      } else {
        existing.mode = normalizeMode(existing.mode);
      }
      if (existing.value === undefined || existing.value === null) {
        existing.value = "";
      }
      return;
    }
    entry.params[key] = { mode: hasActual ? "actual" : "custom", value: "" };
  });
}

function buildUserinfoValue(params, actualParams) {
  const parts = [];
  USERINFO_KEYS.forEach((key) => {
    const param = params && params[key];
    if (!param) return;
    const mode = normalizeMode(param.mode);
    let value = "";
    if (mode === "actual") {
      value = actualParams ? actualParams[key] || "" : "";
    } else {
      value = param.value ?? "";
    }
    value = String(value).trim();
    if (!value) return;
    parts.push(`${key}=${value}`);
  });
  return parts.join("; ");
}

function hasActualParams(params) {
  return USERINFO_KEYS.some((key) => normalizeMode(params?.[key]?.mode) === "actual");
}

function getUserinfoModeLabel(params) {
  const modes = USERINFO_KEYS.map((key) => normalizeMode(params?.[key]?.mode));
  if (modes.every((mode) => mode === "actual")) return "актуальные";
  if (modes.every((mode) => mode === "custom")) return "кастом";
  return "микс";
}

function getActualUserinfoParams(entry) {
  const actualValue = getActualHeaderValue(entry.key);
  let params = parseUserinfo(actualValue);
  if (!Object.keys(params).length && entry.value) {
    const fallback = parseUserinfo(entry.value);
    if (Object.keys(fallback).length) {
      params = fallback;
    }
  }
  return params;
}

async function api(path, options = {}) {
  const headers = new Headers(options.headers || {});
  const token = getToken();
  if (token) {
    headers.set("Authorization", `Bearer ${token}`);
  }
  const response = await fetch(path, { ...options, headers });
  const text = await response.text();
  let payload = null;
  if (text) {
    try {
      payload = JSON.parse(text);
    } catch (err) {
      payload = text;
    }
  }
  if (!response.ok) {
    const message = payload && payload.error ? payload.error : response.statusText;
    throw new Error(message);
  }
  return payload;
}

function normalizeConfigEntries(entries, core) {
  if (!Array.isArray(entries)) return [];
  return entries
    .filter((entry) => entry && typeof entry === "object")
    .map((entry) => {
      const name = String(entry.name || "").trim();
      if (core === "mihomo") {
        return {
          name,
          content: typeof entry.content === "string" ? entry.content : "",
        };
      }
      if (!("config" in entry)) {
        const derivedName = String(entry.remarks || entry.name || "").trim();
        const config = { ...entry };
        delete config.remarks;
        delete config.name;
        return {
          name: derivedName,
          config,
        };
      }
      return {
        name,
        config: entry.config && typeof entry.config === "object" ? entry.config : {},
      };
    });
}

function normalizeConfigScopedSet(raw, core) {
  const source = raw && typeof raw === "object" ? raw : {};
  const scoped = createEmptyScopedSet();
  scoped.default = normalizeConfigEntries(source.default || source.configs || [], core);
  Object.entries(source.squads && typeof source.squads === "object" ? source.squads : {}).forEach(
    ([key, value]) => {
      if (!key) return;
      scoped.squads[key] = normalizeConfigEntries(value || [], core);
    }
  );
  return scoped;
}

function normalizeConfigPayload(data) {
  if (data && typeof data === "object" && ("xray" in data || "mihomo" in data)) {
    return {
      xray: normalizeConfigScopedSet(data.xray, "xray"),
      mihomo: normalizeConfigScopedSet(data.mihomo, "mihomo"),
    };
  }
  return {
    xray: normalizeConfigScopedSet(data, "xray"),
    mihomo: createEmptyScopedSet(),
  };
}

function buildConfigPayload() {
  const payload = {};
  CORE_OPTIONS.forEach((core) => {
    const scoped = getCoreScopedSet(state.configSets, core.value);
    payload[core.value] = {
      default: scoped.default,
      squads: scoped.squads,
    };
  });
  return payload;
}

function normalizeHeaderScopedSet(raw, mapper) {
  const scoped = createEmptyScopedSet();
  const source = raw && typeof raw === "object" ? raw : {};
  const defaultOverrides = source.default || source.overrides || {};
  scoped.default = mapper(defaultOverrides);
  Object.entries(source.squads && typeof source.squads === "object" ? source.squads : {}).forEach(
    ([key, value]) => {
      if (!key) return;
      scoped.squads[key] = mapper(value || {});
    }
  );
  return scoped;
}

function normalizeHeaderPayload(data, mapper) {
  if (data && typeof data === "object" && ("xray" in data || "mihomo" in data)) {
    return {
      xray: normalizeHeaderScopedSet(data.xray, mapper),
      mihomo: normalizeHeaderScopedSet(data.mihomo, mapper),
    };
  }
  return {
    xray: normalizeHeaderScopedSet(data, mapper),
    mihomo: createEmptyScopedSet(),
  };
}

function buildHeadersPayload(buildOverrides) {
  const payload = {};
  CORE_OPTIONS.forEach((core) => {
    const scoped = getCoreScopedSet(state.headerSets, core.value);
    const entry = { default: buildOverrides(scoped.default), squads: {} };
    Object.entries(scoped.squads).forEach(([key, entries]) => {
      const overrides = buildOverrides(entries || []);
      if (Object.keys(overrides).length) {
        entry.squads[key] = overrides;
      }
    });
    payload[core.value] = entry;
  });
  return payload;
}

function showAdmin() {
  els.loginPanel.classList.add("hidden");
  els.adminContent.classList.remove("hidden");
}

async function checkToken() {
  const token = getToken();
  if (!token) {
    setLoginStatus("Токен пустой.");
    return false;
  }
  try {
    await api("./api/auth/check");
    saveToken(token);
    setLoginStatus("Токен принят.");
    showAdmin();
    await loadSquads();
    await loadConfigs();
    await loadHeaders();
    return true;
  } catch (err) {
    setLoginStatus(`Ошибка: ${err.message}`);
    return false;
  }
}

function renderConfigList() {
  if (!els.configList) return;
  els.configList.innerHTML = "";
  const configs = getCurrentConfigList();
  if (state.currentConfigIndex !== null && !configs[state.currentConfigIndex]) {
    state.currentConfigIndex = null;
  }
  if (els.configCount) {
    els.configCount.textContent = String(configs.length);
  }
  if (!configs.length) {
    const empty = document.createElement("div");
    empty.className = "hint";
    empty.textContent = isMihomoCore() ? "Mihomo-конфиги еще не добавлены." : "Конфиги еще не добавлены.";
    els.configList.appendChild(empty);
    syncConfigSwitcher();
    return;
  }
  configs.forEach((entry, index) => {
    const btn = document.createElement("button");
    btn.textContent = entry.name || "(без названия)";
    btn.classList.add("config-item");
    btn.type = "button";
    btn.draggable = true;
    btn.dataset.index = String(index);
    btn.dataset.key = getEntryKey(entry);
    btn.setAttribute("role", "option");
    btn.setAttribute("aria-selected", index === state.currentConfigIndex ? "true" : "false");
    btn.title = entry.name || "(без названия)";
    if (index === state.currentConfigIndex) {
      btn.classList.add("active");
    }
    btn.addEventListener("click", () => {
      selectConfig(index);
    });
    btn.addEventListener("dragstart", onConfigDragStart);
    btn.addEventListener("dragend", onConfigDragEnd);
    btn.addEventListener("dragover", onConfigDragOver);
    btn.addEventListener("dragleave", onConfigDragLeave);
    btn.addEventListener("drop", onConfigDrop);
    els.configList.appendChild(btn);
  });
  syncConfigSwitcher();
}

function selectConfig(index) {
  const entry = getCurrentConfigList()[index];
  if (!entry) return;
  state.currentConfigIndex = index;
  els.configName.value = entry.name || "";
  els.configJson.value = isMihomoCore()
    ? entry.content || ""
    : JSON.stringify(entry.config || {}, null, 2);
  syncConfigNameDisplay();
  setConfigNameEditorOpen(false);
  setConfigMenuOpen(false);
  updateCoreUI();
  renderConfigList();
}

function clearConfigEditor() {
  state.currentConfigIndex = null;
  els.configName.value = "";
  els.configJson.value = "";
  syncConfigNameDisplay();
  setConfigNameEditorOpen(false);
  setConfigMenuOpen(false);
  updateCoreUI();
  renderConfigList();
}

function getEntryKey(entry) {
  if (!entry || typeof entry !== "object") return "";
  let key = entryKeyMap.get(entry);
  if (!key) {
    key = `cfg-${entryKeySeed++}`;
    entryKeyMap.set(entry, key);
  }
  return key;
}

function captureConfigPositions() {
  const positions = new Map();
  if (!els.configList) return positions;
  const items = els.configList.querySelectorAll(".config-item");
  items.forEach((item) => {
    positions.set(item.dataset.key, item.getBoundingClientRect());
  });
  return positions;
}

function animateConfigReorder(prevPositions) {
  if (!prevPositions || prevPositions.size === 0 || !els.configList) return;
  const items = els.configList.querySelectorAll(".config-item");
  items.forEach((item) => {
    const prev = prevPositions.get(item.dataset.key);
    if (!prev) return;
    const next = item.getBoundingClientRect();
    const deltaY = prev.top - next.top;
    if (!deltaY) return;
    item.style.transition = "transform 0s";
    item.style.transform = `translateY(${deltaY}px)`;
    requestAnimationFrame(() => {
      item.style.transition = "";
      item.style.transform = "";
    });
  });
}

function moveConfigItem(fromIndex, toIndex) {
  const list = getCurrentConfigList();
  if (!list.length) return;
  if (fromIndex < 0 || toIndex < 0) return;
  if (fromIndex >= list.length || toIndex >= list.length) return;
  if (fromIndex === toIndex) return;
  const [entry] = list.splice(fromIndex, 1);
  list.splice(toIndex, 0, entry);

  if (state.currentConfigIndex === null) return;
  const current = state.currentConfigIndex;
  if (current === fromIndex) {
    state.currentConfigIndex = toIndex;
  } else if (fromIndex < toIndex && current > fromIndex && current <= toIndex) {
    state.currentConfigIndex = current - 1;
  } else if (fromIndex > toIndex && current >= toIndex && current < fromIndex) {
    state.currentConfigIndex = current + 1;
  }
}

function clearConfigDragHints() {
  if (!els.configList) return;
  els.configList.querySelectorAll(".config-item.drag-over").forEach((item) => {
    item.classList.remove("drag-over");
  });
}

function onConfigDragStart(event) {
  const item = event.currentTarget;
  dragState.index = Number(item.dataset.index);
  dragState.key = item.dataset.key;
  item.classList.add("dragging");
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = "move";
    event.dataTransfer.setData("text/plain", dragState.key || "move");
  }
}

function onConfigDragEnd(event) {
  const item = event.currentTarget;
  item.classList.remove("dragging");
  clearConfigDragHints();
  dragState.index = null;
  dragState.key = null;
}

function onConfigDragOver(event) {
  if (dragState.index === null) return;
  event.preventDefault();
  if (event.dataTransfer) {
    event.dataTransfer.dropEffect = "move";
  }
  const item = event.currentTarget;
  if (item.dataset.key !== dragState.key) {
    item.classList.add("drag-over");
  }
}

function onConfigDragLeave(event) {
  event.currentTarget.classList.remove("drag-over");
}

function onConfigDrop(event) {
  if (dragState.index === null) return;
  event.preventDefault();
  const item = event.currentTarget;
  item.classList.remove("drag-over");
  const targetIndex = Number(item.dataset.index);
  if (!Number.isFinite(targetIndex)) return;
  if (targetIndex === dragState.index) return;
  const positions = captureConfigPositions();
  moveConfigItem(dragState.index, targetIndex);
  renderConfigList();
  animateConfigReorder(positions);
}

function formatConfig() {
  const content = els.configJson.value.trim();
  if (!content) {
    setStatus(isMihomoCore() ? "YAML пустой." : "JSON пустой.");
    return;
  }
  if (isMihomoCore()) {
    setStatus("Для Mihomo форматирование вручную, автопарсер не применяется.");
    return;
  }
  try {
    const parsed = JSON.parse(content);
    els.configJson.value = JSON.stringify(parsed, null, 2);
    setStatus("Конфиг отформатирован.");
  } catch (err) {
    setStatus("Ошибка JSON.");
  }
}

async function saveConfigEntry() {
  const name = els.configName.value.trim();
  if (!name) {
    setStatus("Название сервера обязательно.");
    return;
  }
  const content = els.configJson.value.trim();
  if (!content) {
    setStatus(isMihomoCore() ? "YAML-конфиг пустой." : "JSON-конфиг пустой.");
    return;
  }
  let entry;
  if (isMihomoCore()) {
    entry = { name, content };
  } else {
    let parsed;
    try {
      parsed = JSON.parse(content);
    } catch (err) {
      setStatus("JSON-конфиг некорректен.");
      return;
    }
    if (!parsed || typeof parsed !== "object") {
      setStatus("Конфиг должен быть объектом JSON.");
      return;
    }
    if (Array.isArray(parsed)) {
      if (parsed.length === 1 && parsed[0] && typeof parsed[0] === "object" && !Array.isArray(parsed[0])) {
        parsed = parsed[0];
      } else {
        setStatus("Конфиг должен быть объектом JSON.");
        return;
      }
    }
    entry = { name, config: parsed };
  }

  const configs = getCurrentConfigList();
  if (state.currentConfigIndex === null) {
    configs.push(entry);
    state.currentConfigIndex = configs.length - 1;
  } else {
    configs[state.currentConfigIndex] = entry;
  }

  try {
    await api("./api/configs", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(buildConfigPayload()),
    });
    setStatus("Конфиги сохранены.");
    syncConfigNameDisplay();
    setConfigNameEditorOpen(false);
    renderConfigList();
  } catch (err) {
    setStatus(`Ошибка: ${err.message}`);
  }
}

async function deleteConfigEntry() {
  if (state.currentConfigIndex === null) {
    setStatus("Выберите конфиг.");
    return;
  }
  const configs = getCurrentConfigList();
  configs.splice(state.currentConfigIndex, 1);
  state.currentConfigIndex = null;
  try {
    await api("./api/configs", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(buildConfigPayload()),
    });
    setStatus("Конфиг удален.");
    clearConfigEditor();
  } catch (err) {
    setStatus(`Ошибка: ${err.message}`);
  }
}

async function loadConfigs() {
  try {
    const data = await api("./api/configs");
    state.configSets = normalizeConfigPayload(data);
    state.currentConfigIndex = null;
    clearConfigEditor();
  } catch (err) {
    setStatus(`Ошибка загрузки конфигов: ${err.message}`);
  }
}

async function loadSquads() {
  if (!els.squadSelect) return;
  state.squads = [];
  renderSquadSelect();
  try {
    const data = await api("./api/remnawave/internal-squads");
    const squads = Array.isArray(data.internalSquads) ? data.internalSquads : [];
    state.squads = squads
      .filter((item) => item && item.uuid)
      .map((item) => {
        const position = Number(item.viewPosition);
        return {
          uuid: item.uuid,
          name: item.name || "",
          viewPosition: Number.isFinite(position) ? position : 0,
        };
      })
      .sort((a, b) => {
        if (a.viewPosition === b.viewPosition) {
          return (a.name || a.uuid).localeCompare(b.name || b.uuid);
        }
        return a.viewPosition - b.viewPosition;
      });
    renderSquadSelect();
  } catch (err) {
    setStatus(`Ошибка загрузки сквадов: ${err.message}`);
  }
}

function renderHeaders() {
  els.headerList.innerHTML = "";
  const headers = getCurrentHeaderList();
  if (!headers.length) {
    const empty = document.createElement("div");
    empty.className = "hint";
    empty.textContent = isMihomoCore() ? "Хедеры для Mihomo не заданы." : "Хедеры не заданы.";
    els.headerList.appendChild(empty);
    return;
  }
  headers.forEach((entry, index) => {
    els.headerList.appendChild(createHeaderItem(entry, index, headers));
  });
}

function createHeaderItem(entry, index, headers) {
  const item = document.createElement("div");
  item.className = "header-item";
  if (entry.open) item.classList.add("open");

  const summary = document.createElement("div");
  summary.className = "header-summary";

  const toggleBtn = document.createElement("button");
  toggleBtn.type = "button";
  toggleBtn.className = "header-toggle";

  const summaryMain = document.createElement("div");
  summaryMain.className = "header-summary-main";

  const keyText = document.createElement("span");
  keyText.className = "header-key";

  const modeText = document.createElement("span");
  modeText.className = "header-mode";

  summaryMain.appendChild(keyText);
  summaryMain.appendChild(modeText);

  const preview = document.createElement("span");
  preview.className = "header-preview";

  const caret = document.createElement("span");
  caret.className = "header-caret";
  caret.textContent = "▾";

  toggleBtn.appendChild(summaryMain);
  toggleBtn.appendChild(preview);
  toggleBtn.appendChild(caret);

  const removeBtn = document.createElement("button");
  removeBtn.className = "header-remove";
  removeBtn.type = "button";
  removeBtn.textContent = "×";

  summary.appendChild(toggleBtn);
  summary.appendChild(removeBtn);

  const body = document.createElement("div");
  body.className = "header-body";
  if (!entry.open) body.classList.add("hidden");

  const keyField = document.createElement("label");
  keyField.className = "field";
  const keyLabel = document.createElement("span");
  keyLabel.textContent = "Header";
  const keyInput = document.createElement("input");
  keyInput.value = entry.key || "";
  keyInput.placeholder = "header-name";
  keyField.appendChild(keyLabel);
  keyField.appendChild(keyInput);
  body.appendChild(keyField);

  const setupNormalControls = () => {
    entry.mode = normalizeMode(entry.mode);

    const controlRow = document.createElement("div");
    controlRow.className = "header-control-row";

    const modeWrap = document.createElement("div");
    modeWrap.className = "mode-toggle";

    const actualBtn = document.createElement("button");
    actualBtn.type = "button";
    actualBtn.textContent = "Актуальные";

    const customBtn = document.createElement("button");
    customBtn.type = "button";
    customBtn.textContent = "Кастом";

    modeWrap.appendChild(actualBtn);
    modeWrap.appendChild(customBtn);

    const valueInput = document.createElement("input");
    valueInput.value = entry.value || "";
    valueInput.placeholder = "header-value";

    const applyMode = (mode) => {
      entry.mode = mode;
      actualBtn.classList.toggle("active", mode === "actual");
      customBtn.classList.toggle("active", mode === "custom");
      const actualValue = getActualHeaderValue(entry.key);
      if (mode === "actual") {
        valueInput.disabled = true;
        valueInput.value = actualValue || "";
      } else {
        valueInput.disabled = false;
        valueInput.value = entry.value || "";
        if (actualValue) valueInput.placeholder = actualValue;
      }
      updateSummary();
    };

    valueInput.addEventListener("input", () => {
      entry.value = valueInput.value;
      updateSummary();
    });

    actualBtn.addEventListener("click", () => applyMode("actual"));
    customBtn.addEventListener("click", () => applyMode("custom"));

    controlRow.appendChild(modeWrap);
    controlRow.appendChild(valueInput);
    body.appendChild(controlRow);

    applyMode(entry.mode || "custom");
  };

  const setupUserinfoControls = () => {
    const actualParams = getActualUserinfoParams(entry);
    ensureUserinfoParams(entry, actualParams);

    const paramGrid = document.createElement("div");
    paramGrid.className = "param-grid";

    USERINFO_KEYS.forEach((paramKey) => {
      const row = document.createElement("div");
      row.className = "param-row";

      const label = document.createElement("span");
      label.className = "param-label";
      label.textContent = paramKey;

      const modeWrap = document.createElement("div");
      modeWrap.className = "mode-toggle";

      const actualBtn = document.createElement("button");
      actualBtn.type = "button";
      actualBtn.textContent = "Актуальные";

      const customBtn = document.createElement("button");
      customBtn.type = "button";
      customBtn.textContent = "Кастом";

      modeWrap.appendChild(actualBtn);
      modeWrap.appendChild(customBtn);

      const input = document.createElement("input");
      const actualValue = actualParams[paramKey] || "";
      input.placeholder = actualValue;

      const getParam = () => entry.params[paramKey];
      const applyMode = (mode) => {
        const param = getParam();
        param.mode = mode;
        actualBtn.classList.toggle("active", mode === "actual");
        customBtn.classList.toggle("active", mode === "custom");
        if (mode === "actual") {
          input.disabled = true;
          input.value = actualValue;
        } else {
          input.disabled = false;
          input.value = param.value ?? "";
          if (actualValue) input.placeholder = actualValue;
        }
        updateSummary();
      };

      input.addEventListener("input", () => {
        getParam().value = input.value;
        updateSummary();
      });

      actualBtn.addEventListener("click", () => applyMode("actual"));
      customBtn.addEventListener("click", () => applyMode("custom"));

      row.appendChild(label);
      row.appendChild(modeWrap);
      row.appendChild(input);
      paramGrid.appendChild(row);

      applyMode(normalizeMode(getParam().mode));
    });

    body.appendChild(paramGrid);
  };

  const updateSummary = () => {
    const cleanKey = (entry.key || "").trim();
    const displayKey = cleanKey || "Новый хедер";
    keyText.textContent = displayKey;
    if (isUserinfoKey(cleanKey)) {
      const actualParams = getActualUserinfoParams(entry);
      ensureUserinfoParams(entry, actualParams);
      modeText.textContent = getUserinfoModeLabel(entry.params);
      const value = buildUserinfoValue(entry.params, actualParams);
      preview.textContent = value || "—";
      return;
    }
    const actualValue = getActualHeaderValue(cleanKey);
    entry.mode = normalizeMode(entry.mode);
    modeText.textContent = entry.mode === "actual" ? "актуальные" : "кастом";
    const value = entry.mode === "actual" ? actualValue : entry.value || "";
    preview.textContent = value || "—";
  };

  keyInput.addEventListener("input", () => {
    const wasUserinfo = isUserinfoKey(entry.key);
    entry.key = keyInput.value.trim();
    const nowUserinfo = isUserinfoKey(entry.key);
    if (wasUserinfo !== nowUserinfo) {
      entry.open = true;
      renderHeaders();
      return;
    }
    updateSummary();
  });

  toggleBtn.addEventListener("click", () => {
    entry.open = !entry.open;
    item.classList.toggle("open", entry.open);
    body.classList.toggle("hidden", !entry.open);
  });

  removeBtn.addEventListener("click", () => {
    headers.splice(index, 1);
    renderHeaders();
  });

  if (isUserinfoKey(entry.key)) {
    setupUserinfoControls();
  } else {
    setupNormalControls();
  }

  updateSummary();

  item.appendChild(summary);
  item.appendChild(body);

  return item;
}

async function loadHeaders() {
  try {
    const data = await api("./api/headers");
    const mapOverrides = (overrides) => {
      const safe = overrides && typeof overrides === "object" ? overrides : {};
      return Object.entries(safe).map(([key, value]) => {
        const isString = typeof value === "string";
        const entry = {
          key,
          mode: normalizeMode(isString ? "custom" : value?.mode),
          value: isString ? value : value?.value || "",
          open: false,
        };
        if (isUserinfoKey(key)) {
          if (!isString && value && typeof value === "object" && value.params && typeof value.params === "object") {
            entry.params = {};
            Object.entries(value.params).forEach(([paramKey, paramValue]) => {
              if (!paramKey) return;
              entry.params[paramKey.toLowerCase()] = {
                mode: normalizeMode(paramValue?.mode),
                value: paramValue?.value ?? "",
              };
            });
          } else if (entry.value) {
            const parsed = parseUserinfo(entry.value);
            entry.params = {};
            Object.entries(parsed).forEach(([paramKey, paramValue]) => {
              entry.params[paramKey.toLowerCase()] = {
                mode: "custom",
                value: paramValue,
              };
            });
          }
        }
        return entry;
      });
    };
    state.headerSets = normalizeHeaderPayload(data, mapOverrides);
    renderHeaders();
  } catch (err) {
    setStatus(`Ошибка загрузки хедеров: ${err.message}`);
  }
}

function addHeaderRow() {
  getCurrentHeaderList().push({ key: "", mode: "custom", value: "", open: true });
  renderHeaders();
}

async function saveHeaders() {
  const buildOverrides = (entries) => {
    const overrides = {};
    entries.forEach((entry) => {
      const key = entry.key.trim();
      if (!key) return;
      if (isUserinfoKey(key) && entry.params) {
        const params = {};
        USERINFO_KEYS.forEach((paramKey) => {
          const param = entry.params[paramKey];
          if (!param) return;
          params[paramKey] = {
            mode: normalizeMode(param.mode),
            value: param.value ?? "",
          };
        });
        const actualParams = getActualUserinfoParams(entry);
        const combinedValue = buildUserinfoValue(entry.params, actualParams);
        const payload = { mode: "custom", params };
        if (combinedValue) {
          payload.value = combinedValue;
        } else if (!hasActualParams(entry.params) && entry.value) {
          payload.value = entry.value;
        }
        overrides[key] = payload;
      } else {
        overrides[key] = {
          mode: normalizeMode(entry.mode),
          value: entry.value || "",
        };
      }
    });
    return overrides;
  };
  const payload = buildHeadersPayload(buildOverrides);
  try {
    await api("./api/headers", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    setStatus("Хедеры сохранены.");
    await loadHeaders();
  } catch (err) {
    setStatus(`Ошибка: ${err.message}`);
  }
}

async function fetchHeaders() {
  const uuid = els.headerUuid.value.trim();
  if (!uuid) {
    setStatus("Укажите Short UUID.");
    return;
  }
  try {
    const data = await api(`./api/remnawave/headers?uuid=${encodeURIComponent(uuid)}`);
    setActualHeaders(data.headers || {});

    const headers = getCurrentHeaderList();
    Object.entries(state.actualHeaders).forEach(([key, value]) => {
      const existing = headers.find(
        (item) => item.key && item.key.toLowerCase() === key.toLowerCase()
      );
      if (existing) {
        if (!isUserinfoKey(existing.key) && normalizeMode(existing.mode) === "actual") {
          existing.value = value;
        }
      } else {
        headers.push({ key, mode: "actual", value, open: false });
      }
    });

    renderHeaders();
    setStatus("Хедеры получены.");
  } catch (err) {
    setStatus(`Ошибка: ${err.message}`);
  }
}

/* ── JSON Syntax Highlighting ──────────────────────────────── */

function escapeHtml(str) {
  return str
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
}

function highlightJson(text) {
  if (!text) return "";
  // Tokenize JSON with regex covering keys, strings, numbers, bools, null, structural chars
  const tokenRegex = /("(?:[^"\\]|\\.)*")\s*(?=:)|("(?:[^"\\]|\\.)*")|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)\b|(true|false)\b|(null)\b|([{}])|([[\]])|(:)|(,)|(\S)/g;
  let result = "";
  let lastIndex = 0;

  let match;
  while ((match = tokenRegex.exec(text)) !== null) {
    // Add any unmatched text (whitespace) between tokens
    if (match.index > lastIndex) {
      result += escapeHtml(text.slice(lastIndex, match.index));
    }
    lastIndex = tokenRegex.lastIndex;

    if (match[1] !== undefined) {
      // Key (string before colon)
      result += `<span class="tok-key">${escapeHtml(match[1])}</span>`;
    } else if (match[2] !== undefined) {
      // String value
      result += `<span class="tok-string">${escapeHtml(match[2])}</span>`;
    } else if (match[3] !== undefined) {
      // Number
      result += `<span class="tok-number">${escapeHtml(match[3])}</span>`;
    } else if (match[4] !== undefined) {
      // Boolean
      result += `<span class="tok-bool">${escapeHtml(match[4])}</span>`;
    } else if (match[5] !== undefined) {
      // null
      result += `<span class="tok-null">${escapeHtml(match[5])}</span>`;
    } else if (match[6] !== undefined) {
      // Braces {}
      result += `<span class="tok-brace">${escapeHtml(match[6])}</span>`;
    } else if (match[7] !== undefined) {
      // Brackets []
      result += `<span class="tok-bracket">${escapeHtml(match[7])}</span>`;
    } else if (match[8] !== undefined) {
      // Colon
      result += `<span class="tok-colon">${escapeHtml(match[8])}</span>`;
    } else if (match[9] !== undefined) {
      // Comma
      result += `<span class="tok-comma">${escapeHtml(match[9])}</span>`;
    } else if (match[10] !== undefined) {
      // Fallback for temporarily invalid JSON fragments while typing.
      result += `<span class="tok-plain">${escapeHtml(match[10])}</span>`;
    }
  }
  // Add remaining text
  if (lastIndex < text.length) {
    result += escapeHtml(text.slice(lastIndex));
  }
  // Ensure trailing newline for correct rendering
  if (text.endsWith("\n")) {
    result += "\n";
  }
  return result;
}

function highlightCode(text) {
  if (isMihomoCore()) {
    return escapeHtml(text || "");
  }
  return highlightJson(text);
}

function updateLineNumbers(text) {
  if (!els.lineNumbers) return;
  const lines = (text || "").split("\n");
  const count = lines.length;
  // Get cursor line for active highlight
  let activeLine = -1;
  if (els.modalConfigJson && document.activeElement === els.modalConfigJson) {
    const before = text.substring(0, els.modalConfigJson.selectionStart);
    activeLine = before.split("\n").length;
  }
  let html = "";
  for (let i = 1; i <= count; i++) {
    const cls = i === activeLine ? "ln active" : "ln";
    html += `<span class="${cls}">${i}</span>`;
  }
  els.lineNumbers.innerHTML = html;
}

function updateHighlight() {
  if (!els.modalConfigJson || !els.codeHighlight) return;
  const text = els.modalConfigJson.value;
  els.codeHighlight.innerHTML = highlightCode(text);
  updateLineNumbers(text);
  validateEditorStatus(text);
}

function validateEditorStatus(text) {
  if (!els.editorStatus) return;
  const trimmed = (text || "").trim();
  if (!trimmed) {
    els.editorStatus.textContent = "Пустой";
    els.editorStatus.className = "editor-status";
    return;
  }
  if (isMihomoCore()) {
    els.editorStatus.textContent = "YAML режим";
    els.editorStatus.className = "editor-status valid";
    return;
  }
  try {
    JSON.parse(trimmed);
    els.editorStatus.textContent = "JSON валиден";
    els.editorStatus.className = "editor-status valid";
  } catch (e) {
    els.editorStatus.textContent = "Ошибка синтаксиса";
    els.editorStatus.className = "editor-status error";
  }
}

function syncEditorScroll() {
  if (!els.modalConfigJson || !els.codeHighlight || !els.lineNumbers) return;
  els.codeHighlight.scrollTop = els.modalConfigJson.scrollTop;
  els.codeHighlight.scrollLeft = els.modalConfigJson.scrollLeft;
  els.lineNumbers.scrollTop = els.modalConfigJson.scrollTop;
}

/* ── Modal logic ──────────────────────────────────────────── */

let modalOpen = false;

function openModal() {
  if (!els.modalOverlay) return;
  // Sync data from inline editor to modal
  els.modalConfigName.value = els.configName.value;
  els.modalConfigJson.value = els.configJson.value;

  const entry = getCurrentConfigList()[state.currentConfigIndex];
  if (els.modalConfigName.value.trim()) {
    els.modalSubtitle.textContent = els.modalConfigName.value.trim();
  } else if (entry) {
    els.modalSubtitle.textContent = entry.name || "Без названия";
  } else {
    els.modalSubtitle.textContent = "Новый конфиг";
  }

  modalOpen = true;
  document.body.style.overflow = "hidden";
  els.modalOverlay.classList.add("open");
  updateHighlight();

  // Focus the textarea after animation
  setTimeout(() => {
    els.modalConfigJson.focus();
  }, 100);
}

function closeModal() {
  if (!els.modalOverlay) return;
  // Sync data back from modal to inline editor
  els.configName.value = els.modalConfigName.value;
  els.configJson.value = els.modalConfigJson.value;
  syncConfigNameDisplay();
  syncConfigSwitcher();

  modalOpen = false;
  document.body.style.overflow = "";
  els.modalOverlay.classList.remove("open");
}

function modalFormatConfig() {
  const content = els.modalConfigJson.value.trim();
  if (!content) {
    setStatus(isMihomoCore() ? "YAML пустой." : "JSON пустой.");
    return;
  }
  if (isMihomoCore()) {
    setStatus("Для Mihomo форматирование вручную, автопарсер не применяется.");
    return;
  }
  try {
    const parsed = JSON.parse(content);
    els.modalConfigJson.value = JSON.stringify(parsed, null, 2);
    updateHighlight();
    setStatus("Конфиг отформатирован.");
  } catch (err) {
    setStatus("Ошибка JSON.");
  }
}

async function modalSaveConfig() {
  // Sync back to inline first
  els.configName.value = els.modalConfigName.value;
  els.configJson.value = els.modalConfigJson.value;
  await saveConfigEntry();
  // Update subtitle
  const entry = getCurrentConfigList()[state.currentConfigIndex];
  if (entry && els.modalSubtitle) {
    els.modalSubtitle.textContent = entry.name || "Без названия";
  }
}

async function modalDeleteConfig() {
  await deleteConfigEntry();
  closeModal();
}

els.tokenCheck.addEventListener("click", checkToken);

els.configNew.addEventListener("click", clearConfigEditor);
els.configFormat.addEventListener("click", formatConfig);
els.configSave.addEventListener("click", saveConfigEntry);
els.configDelete.addEventListener("click", deleteConfigEntry);

// Modal buttons
if (els.configExpand) {
  els.configExpand.addEventListener("click", openModal);
}
if (els.modalClose) {
  els.modalClose.addEventListener("click", closeModal);
}
if (els.modalFormat) {
  els.modalFormat.addEventListener("click", modalFormatConfig);
}
if (els.modalSave) {
  els.modalSave.addEventListener("click", modalSaveConfig);
}
if (els.modalDelete) {
  els.modalDelete.addEventListener("click", modalDeleteConfig);
}

// Click outside modal to close
if (els.modalOverlay) {
  els.modalOverlay.addEventListener("click", (event) => {
    if (event.target === els.modalOverlay) {
      closeModal();
    }
  });
}

if (els.coreSwitcherTrigger) {
  els.coreSwitcherTrigger.addEventListener("click", () => {
    if (!state.coreMenuOpen) {
      setConfigMenuOpen(false);
      setSquadMenuOpen(false);
    }
    setCoreMenuOpen(!state.coreMenuOpen);
  });
}

if (els.configSwitcherTrigger) {
  els.configSwitcherTrigger.addEventListener("click", () => {
    if (!state.configMenuOpen) {
      setCoreMenuOpen(false);
      setSquadMenuOpen(false);
    }
    setConfigMenuOpen(!state.configMenuOpen);
  });
}

if (els.squadSwitcherTrigger) {
  els.squadSwitcherTrigger.addEventListener("click", () => {
    if (!state.squadMenuOpen) {
      setCoreMenuOpen(false);
      setConfigMenuOpen(false);
    }
    setSquadMenuOpen(!state.squadMenuOpen);
  });
}

document.addEventListener("click", (event) => {
  if (state.coreMenuOpen && els.coreSwitcher && !els.coreSwitcher.contains(event.target)) {
    setCoreMenuOpen(false);
  }
  if (state.configMenuOpen && els.configSwitcher && !els.configSwitcher.contains(event.target)) {
    setConfigMenuOpen(false);
  }
  if (state.squadMenuOpen && els.squadSwitcher && !els.squadSwitcher.contains(event.target)) {
    setSquadMenuOpen(false);
  }
});

document.addEventListener("keydown", (event) => {
  if (event.key !== "Escape") return;
  if (modalOpen) {
    event.preventDefault();
    closeModal();
    return;
  }
  if (state.configMenuOpen) {
    event.preventDefault();
    setConfigMenuOpen(false);
    return;
  }
  if (state.coreMenuOpen) {
    event.preventDefault();
    setCoreMenuOpen(false);
    return;
  }
  if (state.squadMenuOpen) {
    event.preventDefault();
    setSquadMenuOpen(false);
  }
});

// Syntax highlighting and line number sync
if (els.modalConfigJson) {
  els.modalConfigJson.addEventListener("input", updateHighlight);
  els.modalConfigJson.addEventListener("scroll", syncEditorScroll);
  els.modalConfigJson.addEventListener("click", () => {
    updateLineNumbers(els.modalConfigJson.value);
  });
  els.modalConfigJson.addEventListener("keyup", () => {
    updateLineNumbers(els.modalConfigJson.value);
  });
  els.modalConfigJson.addEventListener("keydown", (event) => {
    if (event.key === "Tab") {
      event.preventDefault();
      const start = els.modalConfigJson.selectionStart;
      const end = els.modalConfigJson.selectionEnd;
      const value = els.modalConfigJson.value;
      els.modalConfigJson.value = value.substring(0, start) + "  " + value.substring(end);
      els.modalConfigJson.selectionStart = els.modalConfigJson.selectionEnd = start + 2;
      updateHighlight();
    }
  });
}

// Modal name sync → update subtitle live
if (els.modalConfigName) {
  els.modalConfigName.addEventListener("input", () => {
    if (els.modalSubtitle) {
      els.modalSubtitle.textContent = els.modalConfigName.value.trim() || "Без названия";
    }
  });
}

if (els.configRename) {
  els.configRename.addEventListener("click", beginConfigRename);
}

if (els.configRenameApply) {
  els.configRenameApply.addEventListener("click", applyConfigRename);
}

if (els.configRenameCancel) {
  els.configRenameCancel.addEventListener("click", cancelConfigRename);
}

if (els.configName) {
  els.configName.addEventListener("input", () => {
    syncConfigNameDisplay();
    syncConfigSwitcher();
  });
  els.configName.addEventListener("keydown", (event) => {
    if (event.key === "Enter") {
      event.preventDefault();
      applyConfigRename();
      return;
    }
    if (event.key === "Escape") {
      event.preventDefault();
      cancelConfigRename();
    }
  });
}

if (els.configList) {
  els.configList.addEventListener("dragover", (event) => {
    if (dragState.index === null) return;
    event.preventDefault();
    if (event.dataTransfer) {
      event.dataTransfer.dropEffect = "move";
    }
  });
  els.configList.addEventListener("drop", (event) => {
    if (dragState.index === null) return;
    const target = event.target.closest(".config-item");
    if (target) return;
    event.preventDefault();
    const list = getCurrentConfigList();
    if (!list.length) return;
    const targetIndex = list.length - 1;
    if (dragState.index === targetIndex) return;
    const positions = captureConfigPositions();
    moveConfigItem(dragState.index, targetIndex);
    renderConfigList();
    animateConfigReorder(positions);
  });
}

if (els.squadSelect) {
  els.squadSelect.addEventListener("change", (event) => {
    selectSquad(event.target.value || DEFAULT_SQUAD_KEY);
  });
}

if (els.coreSelect) {
  els.coreSelect.addEventListener("change", (event) => {
    selectCore(event.target.value || DEFAULT_CORE_KEY);
  });
}

els.headerAdd.addEventListener("click", addHeaderRow);
els.headerSave.addEventListener("click", saveHeaders);
els.headerFetch.addEventListener("click", fetchHeaders);

renderCoreSelect();
updateCoreUI();

if (loadToken()) {
  checkToken();
}
