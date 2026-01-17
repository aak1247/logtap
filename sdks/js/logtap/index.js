const SDK_NAME = "logtap-sdk";
const SDK_VERSION = "0.1.0";

/**
 * @typedef {"debug"|"info"|"warn"|"error"|"fatal"|"event"} LogtapLevel
 */

/**
 * @typedef {Object} LogtapUser
 * @property {string=} id
 * @property {string=} email
 * @property {string=} username
 * @property {Record<string, any>=} traits
 */

/**
 * @typedef {Object} LogtapLog
 * @property {LogtapLevel} level
 * @property {string} message
 * @property {string=} timestamp RFC3339
 * @property {string=} device_id
 * @property {string=} trace_id
 * @property {string=} span_id
 * @property {Record<string, any>=} fields
 * @property {Record<string, string>=} tags
 * @property {Record<string, any>=} user
 * @property {Record<string, any>=} contexts
 * @property {Record<string, any>=} extra
 * @property {Record<string, any>=} sdk
 */

/**
 * @typedef {Object} LogtapTrackEvent
 * @property {string} name
 * @property {string=} timestamp RFC3339
 * @property {string=} device_id
 * @property {string=} trace_id
 * @property {string=} span_id
 * @property {Record<string, any>=} properties
 * @property {Record<string, string>=} tags
 * @property {Record<string, any>=} user
 * @property {Record<string, any>=} contexts
 * @property {Record<string, any>=} extra
 * @property {Record<string, any>=} sdk
 */

/**
 * @typedef {Object} LogtapClientOptions
 * @property {string} baseUrl e.g. "http://localhost:8080"
 * @property {number|string} projectId
 * @property {string=} projectKey sent as X-Project-Key: pk_...
 * @property {number=} flushIntervalMs default 2000
 * @property {number=} maxBatchSize default 50
 * @property {number=} maxQueueSize default 1000 per queue
 * @property {number=} timeoutMs default 5000
 * @property {boolean=} gzip default false (browser requires CompressionStream)
 * @property {boolean=} persistDeviceId default true (browser only)
 * @property {string=} deviceId
 * @property {LogtapUser=} user
 * @property {Record<string, any>=} globalFields merged into every log fields
 * @property {Record<string, any>=} globalProperties merged into every track properties
 * @property {Record<string, string>=} globalTags merged into every payload tags
 * @property {Record<string, any>=} globalContexts merged into every payload contexts
 * @property {(payload: LogtapLog|LogtapTrackEvent) => (LogtapLog|LogtapTrackEvent|null)=} beforeSend
 */

function isBrowser() {
  return typeof window !== "undefined" && typeof window.document !== "undefined";
}

function nowISO() {
  return new Date().toISOString();
}

function normalizeBaseUrl(baseUrl) {
  const s = String(baseUrl || "").trim();
  if (!s) throw new Error("baseUrl required");
  return s.endsWith("/") ? s.slice(0, -1) : s;
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function safeJson(value) {
  try {
    return JSON.parse(JSON.stringify(value));
  } catch {
    return undefined;
  }
}

function randomHex(bytes) {
  if (typeof globalThis.crypto?.getRandomValues === "function") {
    const buf = new Uint8Array(bytes);
    globalThis.crypto.getRandomValues(buf);
    return Array.from(buf, (b) => b.toString(16).padStart(2, "0")).join("");
  }
  return Array.from({ length: bytes }, () => Math.floor(Math.random() * 256))
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}

function newDeviceId() {
  return `d_${randomHex(16)}`;
}

function mergeObj(a, b) {
  if (!a && !b) return undefined;
  /** @type {Record<string, any>} */
  const out = {};
  if (a && typeof a === "object") Object.assign(out, a);
  if (b && typeof b === "object") Object.assign(out, b);
  return out;
}

function mergeTags(a, b) {
  if (!a && !b) return undefined;
  /** @type {Record<string, string>} */
  const out = {};
  if (a && typeof a === "object") Object.assign(out, a);
  if (b && typeof b === "object") Object.assign(out, b);
  return out;
}

async function gzipIfEnabled(bodyString, enabled) {
  if (!enabled) return { body: bodyString, contentEncoding: undefined };

  if (isBrowser()) {
    if (typeof CompressionStream !== "function") {
      return { body: bodyString, contentEncoding: undefined };
    }
    const stream = new CompressionStream("gzip");
    const writer = stream.writable.getWriter();
    const enc = new TextEncoder();
    await writer.write(enc.encode(bodyString));
    await writer.close();
    const buf = await new Response(stream.readable).arrayBuffer();
    return { body: new Uint8Array(buf), contentEncoding: "gzip" };
  }

  const { gzipSync } = await import("node:zlib");
  const { Buffer } = await import("node:buffer");
  const gz = gzipSync(Buffer.from(bodyString, "utf8"));
  return { body: gz, contentEncoding: "gzip" };
}

/**
 * Browser/Node client for logtap logs + tracking.
 */
export class LogtapClient {
  /**
   * @param {LogtapClientOptions} options
   */
  constructor(options) {
    this._baseUrl = normalizeBaseUrl(options.baseUrl);
    this._projectId = String(options.projectId);
    if (!this._projectId) throw new Error("projectId required");
    this._projectKey = options.projectKey ? String(options.projectKey).trim() : "";
    this._timeoutMs = Number(options.timeoutMs ?? 5000);
    this._flushIntervalMs = Number(options.flushIntervalMs ?? 2000);
    this._maxBatchSize = Number(options.maxBatchSize ?? 50);
    this._maxQueueSize = Number(options.maxQueueSize ?? 1000);
    this._gzip = Boolean(options.gzip ?? false);
    this._persistDeviceId = Boolean(options.persistDeviceId ?? true);
    this._beforeSend = typeof options.beforeSend === "function" ? options.beforeSend : null;

    this._globalFields = options.globalFields && typeof options.globalFields === "object" ? options.globalFields : undefined;
    this._globalProperties =
      options.globalProperties && typeof options.globalProperties === "object" ? options.globalProperties : undefined;
    this._globalTags = options.globalTags && typeof options.globalTags === "object" ? options.globalTags : undefined;
    this._globalContexts = options.globalContexts && typeof options.globalContexts === "object" ? options.globalContexts : undefined;

    this._user = options.user ? safeJson(options.user) : undefined;
    this._deviceId = options.deviceId ? String(options.deviceId) : this._loadOrCreateDeviceId();

    /** @type {LogtapLog[]} */
    this._logQueue = [];
    /** @type {LogtapTrackEvent[]} */
    this._trackQueue = [];

    this._backoffMs = 0;
    this._flushing = null;
    this._timer = null;

    if (this._flushIntervalMs > 0) {
      this._timer = setInterval(() => void this.flush(), this._flushIntervalMs);
      // In Node, don't keep the process alive for the timer.
      if (!isBrowser() && typeof this._timer?.unref === "function") {
        this._timer.unref();
      }
    }
  }

  /**
   * Set/overwrite current user context (merged into every payload).
   * @param {LogtapUser|null|undefined} user
   */
  setUser(user) {
    this._user = user ? safeJson(user) : undefined;
  }

  /**
   * Convenience identity method.
   * @param {string} userId
   * @param {Record<string, any>=} traits
   */
  identify(userId, traits) {
    const id = String(userId || "").trim();
    if (!id) return;
    /** @type {LogtapUser} */
    const u = { id };
    if (traits && typeof traits === "object") u.traits = safeJson(traits);
    this.setUser(u);
  }

  clearUser() {
    this._user = undefined;
  }

  /**
   * Override device_id (used for DAU/MAU distinct).
   * @param {string} deviceId
   * @param {{persist?: boolean}=} options
   */
  setDeviceId(deviceId, options) {
    const id = String(deviceId || "").trim();
    if (!id) return;
    this._deviceId = id;
    const persist = options?.persist ?? this._persistDeviceId;
    if (persist && isBrowser()) {
      try {
        localStorage.setItem("logtap_device_id", id);
      } catch {}
    }
  }

  /**
   * Enqueue a structured log (sent to POST /api/:projectId/logs/).
   * @param {LogtapLevel} level
   * @param {string} message
   * @param {Record<string, any>=} fields
   * @param {{traceId?: string, spanId?: string, timestamp?: string|Date, tags?: Record<string,string>, deviceId?: string, user?: LogtapUser, contexts?: Record<string, any>, extra?: Record<string, any>}=} options
   */
  log(level, message, fields, options) {
    const msg = String(message || "").trim();
    if (!msg) return;

    /** @type {LogtapLog} */
    const payload = {
      level: level || "info",
      message: msg,
      device_id: options?.deviceId ? String(options.deviceId) : this._deviceId,
      trace_id: options?.traceId ? String(options.traceId) : undefined,
      span_id: options?.spanId ? String(options.spanId) : undefined,
      timestamp: toTimestamp(options?.timestamp) ?? nowISO(),
      fields: mergeObj(this._globalFields, safeJson(fields)),
      tags: mergeTags(this._globalTags, options?.tags),
      user: options?.user ? safeJson(options.user) : this._user,
      contexts: mergeObj(this._globalContexts, safeJson(options?.contexts)),
      extra: safeJson(options?.extra),
      sdk: { name: SDK_NAME, version: SDK_VERSION, runtime: isBrowser() ? "browser" : "node" },
    };

    this._enqueueLog(payload);
  }

  debug(message, fields, options) {
    this.log("debug", message, fields, options);
  }
  info(message, fields, options) {
    this.log("info", message, fields, options);
  }
  warn(message, fields, options) {
    this.log("warn", message, fields, options);
  }
  error(message, fields, options) {
    this.log("error", message, fields, options);
  }
  fatal(message, fields, options) {
    this.log("fatal", message, fields, options);
  }

  /**
   * Track an event (sent to POST /api/:projectId/track/).
   * @param {string} name
   * @param {Record<string, any>=} properties
   * @param {{traceId?: string, spanId?: string, timestamp?: string|Date, tags?: Record<string,string>, deviceId?: string, user?: LogtapUser, contexts?: Record<string, any>, extra?: Record<string, any>}=} options
   */
  track(name, properties, options) {
    const n = String(name || "").trim();
    if (!n) return;

    /** @type {LogtapTrackEvent} */
    const payload = {
      name: n,
      device_id: options?.deviceId ? String(options.deviceId) : this._deviceId,
      trace_id: options?.traceId ? String(options.traceId) : undefined,
      span_id: options?.spanId ? String(options.spanId) : undefined,
      timestamp: toTimestamp(options?.timestamp) ?? nowISO(),
      properties: mergeObj(this._globalProperties, safeJson(properties)),
      tags: mergeTags(this._globalTags, options?.tags),
      user: options?.user ? safeJson(options.user) : this._user,
      contexts: mergeObj(this._globalContexts, safeJson(options?.contexts)),
      extra: safeJson(options?.extra),
      sdk: { name: SDK_NAME, version: SDK_VERSION, runtime: isBrowser() ? "browser" : "node" },
    };

    this._enqueueTrack(payload);
  }

  /**
   * Flush queued logs + events now.
   * @returns {Promise<void>}
   */
  async flush() {
    if (this._flushing) return this._flushing;
    this._flushing = this._flushInner().finally(() => {
      this._flushing = null;
    });
    return this._flushing;
  }

  /**
   * Stop periodic flushing and try to send remaining payloads.
   * @returns {Promise<void>}
   */
  async close() {
    if (this._timer) clearInterval(this._timer);
    this._timer = null;
    await this.flush();
  }

  /**
   * Browser-only: capture window.onerror + unhandledrejection as error logs.
   * @param {{includeSource?: boolean}=} options
   */
  captureBrowserErrors(options) {
    if (!isBrowser()) return;
    const includeSource = options?.includeSource ?? true;

    window.addEventListener("error", (ev) => {
      try {
        const msg = ev.error?.message || ev.message || "window.error";
        /** @type {Record<string, any>} */
        const f = {
          kind: "window.error",
          stack: ev.error?.stack,
          filename: includeSource ? ev.filename : undefined,
          lineno: includeSource ? ev.lineno : undefined,
          colno: includeSource ? ev.colno : undefined,
        };
        this.error(msg, f);
      } catch {}
    });

    window.addEventListener("unhandledrejection", (ev) => {
      try {
        const reason = ev.reason;
        const msg = reason?.message || String(reason || "unhandledrejection");
        /** @type {Record<string, any>} */
        const f = {
          kind: "unhandledrejection",
          reason: safeJson(reason) ?? String(reason),
          stack: reason?.stack,
        };
        this.error(msg, f);
      } catch {}
    });
  }

  /**
   * Node-only: capture process unhandledRejection + uncaughtException as error logs.
   */
  captureNodeErrors() {
    if (isBrowser()) return;
    const p = globalThis.process;
    if (!p?.on) return;

    p.on("unhandledRejection", (reason) => {
      try {
        const msg = reason?.message || String(reason || "unhandledRejection");
        this.error(msg, { kind: "unhandledRejection", reason: safeJson(reason) ?? String(reason), stack: reason?.stack });
      } catch {}
    });

    p.on("uncaughtException", (err) => {
      try {
        const msg = err?.message || String(err || "uncaughtException");
        this.fatal(msg, { kind: "uncaughtException", stack: err?.stack });
      } catch {}
    });
  }

  _loadOrCreateDeviceId() {
    if (isBrowser() && this._persistDeviceId) {
      try {
        const existing = localStorage.getItem("logtap_device_id");
        if (existing && existing.trim()) return existing.trim();
      } catch {}
      const id = newDeviceId();
      try {
        localStorage.setItem("logtap_device_id", id);
      } catch {}
      return id;
    }
    return newDeviceId();
  }

  /** @param {LogtapLog} payload */
  _enqueueLog(payload) {
    const p = this._applyBeforeSend(payload);
    if (!p) return;
    this._logQueue.push(p);
    if (this._logQueue.length > this._maxQueueSize) {
      this._logQueue.splice(0, this._logQueue.length - this._maxQueueSize);
    }
  }

  /** @param {LogtapTrackEvent} payload */
  _enqueueTrack(payload) {
    const p = this._applyBeforeSend(payload);
    if (!p) return;
    this._trackQueue.push(p);
    if (this._trackQueue.length > this._maxQueueSize) {
      this._trackQueue.splice(0, this._trackQueue.length - this._maxQueueSize);
    }
  }

  _applyBeforeSend(payload) {
    if (!this._beforeSend) return payload;
    try {
      return this._beforeSend(payload);
    } catch {
      return payload;
    }
  }

  async _flushInner() {
    if (this._backoffMs > 0) {
      await sleep(this._backoffMs);
    }

    let sentAny = false;
    let failed = false;

    while (this._logQueue.length > 0) {
      const ok = await this._flushQueue("/logs/", this._logQueue);
      if (!ok) {
        failed = true;
        break;
      }
      sentAny = true;
    }

    while (this._trackQueue.length > 0) {
      const ok = await this._flushQueue("/track/", this._trackQueue);
      if (!ok) {
        failed = true;
        break;
      }
      sentAny = true;
    }

    if (sentAny && !failed) {
      this._backoffMs = 0;
      return;
    }
  }

  async _flushQueue(path, queue) {
    if (queue.length === 0) return false;

    const batch = queue.splice(0, this._maxBatchSize);
    if (batch.length === 0) return false;

    const ok = await this._postJSON(path, batch);
    if (!ok) {
      queue.unshift(...batch);
      this._backoffMs = this._backoffMs > 0 ? Math.min(this._backoffMs * 2, 30000) : 500;
      return false;
    }
    return true;
  }

  async _postJSON(path, payload) {
    if (typeof fetch !== "function") {
      throw new Error("global fetch() is required (Node 18+ or provide a polyfill)");
    }

    const url = `${this._baseUrl}/api/${encodeURIComponent(this._projectId)}${path}`;
    const json = JSON.stringify(payload);
    const { body, contentEncoding } = await gzipIfEnabled(json, this._gzip);

    /** @type {Record<string, string>} */
    const headers = { "Content-Type": "application/json" };
    if (this._projectKey) headers["X-Project-Key"] = this._projectKey;
    if (contentEncoding) headers["Content-Encoding"] = contentEncoding;

    const controller = typeof AbortController === "function" ? new AbortController() : null;
    const timeoutMs = this._timeoutMs > 0 ? this._timeoutMs : 0;
    const timer = timeoutMs > 0 ? setTimeout(() => controller?.abort(), timeoutMs) : null;

    try {
      const res = await fetch(url, {
        method: "POST",
        headers,
        body,
        keepalive: isBrowser(),
        signal: controller?.signal,
      });
      return res.status >= 200 && res.status < 300;
    } catch {
      return false;
    } finally {
      if (timer) clearTimeout(timer);
    }
  }
}

function toTimestamp(v) {
  if (!v) return undefined;
  if (typeof v === "string") return v;
  if (v instanceof Date) return v.toISOString();
  return undefined;
}
