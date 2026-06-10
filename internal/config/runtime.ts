// Pando config runtime. Injected before the user config so defineStack and the
// liveUpdate helpers exist without an import. Types come from ./types — esbuild
// strips them at transpile time; they drive authoring and the CI tsc check.

import type {
  Duration,
  LiveUpdateStep,
  ReadyProbe,
  RestartStep,
  RunStep,
  Service,
  Stack,
  SyncRule,
} from "./types";

interface NormalizedProbe {
  kind: "" | "httpGet" | "tcp" | "logMatch" | "exit0";
  target?: string;
  pattern?: string;
  timeout?: number;
  interval?: number;
}

interface NormalizedResource {
  name: string;
  kind: "compose" | "local" | "task";
  deps?: string[];
  runWhen?: string;
  onChange?: string[];
  ready?: NormalizedProbe;
  build?: Service["build"];
  compose?: Service["compose"];
  local?: Service["local"];
  task?: Service["task"];
  liveUpdate?: LiveUpdateStep[];
  hooks?: Service["hooks"];
  every?: number;
  preview?: boolean;
}

interface NormalizedStack {
  name: string;
  resources: NormalizedResource[];
}

declare global {
  // eslint-disable-next-line no-var
  var __pando_stack: Stack | undefined;
}

export function sync(local: string, container: string): SyncRule {
  return { sync: { local, container } };
}

export function run(cmd: string, opts?: { trigger?: string | string[] }): RunStep {
  const step: RunStep = { run: cmd };
  if (opts?.trigger) {
    step.trigger = Array.isArray(opts.trigger) ? opts.trigger : [opts.trigger];
  }
  return step;
}

export function restart(): RestartStep {
  return { restart: true };
}

export function defineStack(spec: Stack): Stack {
  globalThis.__pando_stack = spec;
  return spec;
}

function kindOf(s: Service): NormalizedResource["kind"] {
  if (s.local) return "local";
  if (s.task) return "task";
  return "compose";
}

function normalizeResource(name: string, s: Service): NormalizedResource {
  const res: NormalizedResource = { name, kind: kindOf(s) };
  if (s.deps) res.deps = s.deps;
  if (s.runWhen !== undefined) {
    if (typeof s.runWhen === "string") {
      res.runWhen = s.runWhen;
    } else if (s.runWhen.onChange) {
      res.runWhen = "onChange";
      res.onChange = s.runWhen.onChange;
    }
  }
  if (s.readyWhen) res.ready = normalizeProbe(s.readyWhen);
  if (s.build) res.build = s.build;
  if (s.compose) res.compose = s.compose;
  if (s.local) res.local = s.local;
  if (s.task) res.task = s.task;
  if (s.liveUpdate) res.liveUpdate = s.liveUpdate;
  if (s.hooks) res.hooks = s.hooks;
  if (s.every !== undefined) res.every = dur(s.every);
  if (s.preview) res.preview = true;
  return res;
}

export function normalize(raw: unknown): NormalizedStack {
  if (!raw || typeof raw !== "object") {
    throw new Error("config must export a stack via defineStack(...)");
  }
  const stack = raw as Stack;
  const services = stack.services || {};
  const resources: NormalizedResource[] = [];
  for (const name of Object.keys(services)) {
    resources.push(normalizeResource(name, services[name] || {}));
  }
  return { name: stack.name || "pando", resources };
}

function normalizeProbe(p: ReadyProbe): NormalizedProbe {
  if ("httpGet" in p) {
    return { kind: "httpGet", target: p.httpGet, timeout: dur(p.timeout), interval: dur(p.interval) };
  }
  if ("tcp" in p) {
    return { kind: "tcp", target: p.tcp, timeout: dur(p.timeout), interval: dur(p.interval) };
  }
  if ("logMatch" in p) {
    return { kind: "logMatch", pattern: p.logMatch, timeout: dur(p.timeout) };
  }
  if ("exit0" in p) {
    return { kind: "exit0", timeout: dur(p.timeout) };
  }
  return { kind: "" };
}

// Durations accept a number (nanoseconds, as Go expects) or a string like
// "30s"/"500ms"/"2m" which we convert to nanoseconds.
function dur(v: Duration | undefined): number {
  if (v === undefined || v === null) return 0;
  if (typeof v === "number") return v;
  const m = /^(\d+(?:\.\d+)?)(ms|s|m|h)$/.exec(v.trim());
  if (!m) throw new Error("bad duration: " + v);
  const n = parseFloat(m[1]);
  const unit: Record<string, number> = { ms: 1e6, s: 1e9, m: 60e9, h: 3600e9 };
  return Math.round(n * unit[m[2]]);
}
