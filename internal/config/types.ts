// Public type surface for Pando config files. Authoring a pando.config.ts
// against these types gives full editor autocomplete and compile-time checks.
// The shapes mirror the Go resource model one-to-one.

export type Duration = number | `${number}ms` | `${number}s` | `${number}m` | `${number}h`;

export interface BuildSecret {
  id: string;
  src: string;
}

export interface Build {
  context: string;
  dockerfile?: string;
  args?: Record<string, string>;
  /** Named stage in a multi-stage Dockerfile. One file, many environments. */
  target?: string;
  /** BuildKit secret mounts. Never baked into image layers. */
  secrets?: BuildSecret[];
}

export interface ComposeSpec {
  image?: string;
  ports?: string[];
  env?: Record<string, string>;
  dependsOn?: string[];
  volumes?: string[];
  command?: string[];
}

export interface LocalSpec {
  cmd: string;
  cwd?: string;
  env?: Record<string, string>;
  watch?: string[];
}

export interface TaskSpec {
  cmd: string;
  cwd?: string;
  env?: Record<string, string>;
}

export type ReadyProbe =
  | { httpGet: string; timeout?: Duration; interval?: Duration }
  | { tcp: string; timeout?: Duration; interval?: Duration }
  | { logMatch: string; timeout?: Duration }
  | { exit0: true; timeout?: Duration };

export type RunWhen = "once" | "always" | "manual" | { onChange: string[] };

export interface SyncRule {
  sync: { local: string; container: string };
}

export interface RunStep {
  run: string;
  trigger?: string[];
}

export interface RestartStep {
  restart: true;
}

export type LiveUpdateStep = SyncRule | RunStep | RestartStep;

export interface Hooks {
  postStart?: string;
  preStop?: string;
}

export interface Service {
  deps?: string[];
  runWhen?: RunWhen;
  readyWhen?: ReadyProbe;
  build?: Build;
  compose?: ComposeSpec;
  local?: LocalSpec;
  task?: TaskSpec;
  liveUpdate?: LiveUpdateStep[];
  hooks?: Hooks;
}

export interface Stack {
  name?: string;
  services: Record<string, Service>;
}

export declare function defineStack(spec: Stack): Stack;
export declare function sync(local: string, container: string): SyncRule;
export declare function run(cmd: string, opts?: { trigger?: string | string[] }): RunStep;
export declare function restart(): RestartStep;
