import { useMemo } from "react";
import { useAppServices } from "../../app/appServices";

export type SessionGoalAction = "pause" | "resume" | "cancel";

/**
 * Typed command boundary for SessionView.
 *
 * The page decides how results are presented (toast, modal, focus), while this
 * hook owns the remote API vocabulary and session-id binding.
 */
export function useSessionCommands(sid: string) {
  const { api } = useAppServices();

  return useMemo(
    () => ({
      updateGoal: (goal: string) =>
        api.goal(sid, { action: "update", goal }),
      goal: (action: SessionGoalAction) => api.goal(sid, { action }),
      interrupt: () => api.interrupt(sid),
      // Stop ONE running thing — a background command by handle, or
      // everything one subagent is doing by its session id. The session and
      // its schedule are untouched.
      kill: (target: { id?: string; agent?: string }) => api.kill(sid, target),
      resume: () => api.resume(sid),
      retry: () => api.retry(sid),
      schedule: (action: "pause" | "resume") => api.schedule(sid, action),
      barrier: () => api.barrier(sid),
      inspect: () => api.inspect(sid),
      promote: () => api.promote(sid),
      artifact: (stream: string, version: number) =>
        api.artifact(sid, stream, version),
    }),
    [api, sid],
  );
}
