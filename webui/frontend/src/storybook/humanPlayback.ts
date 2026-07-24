const DEFAULT_HUMAN_STEP_MS = 900;

// Story play functions are both executable QA and an in-browser explanation.
// Keep CI/browser automation instant, but leave enough time between meaningful
// UI transitions for a person replaying the Story to see what changed.
export async function humanPause(
  delayMs = DEFAULT_HUMAN_STEP_MS,
): Promise<void> {
  if (
    import.meta.env.MODE === "test" ||
    globalThis.navigator?.webdriver === true
  ) {
    return;
  }
  await new Promise<void>((resolve) => {
    globalThis.setTimeout(resolve, delayMs);
  });
}
