import { GlobalRegistrator } from "@happy-dom/global-registrator";

// Register a DOM so component tests can render React. requestAnimationFrame is
// stubbed to fire synchronously: the log store batches flushes on rAF, and tests
// assert on the flushed result without waiting a real frame.
GlobalRegistrator.register();

globalThis.requestAnimationFrame = ((cb: FrameRequestCallback) => {
  cb(0);
  return 0;
}) as typeof requestAnimationFrame;
globalThis.cancelAnimationFrame = (() => {}) as typeof cancelAnimationFrame;
