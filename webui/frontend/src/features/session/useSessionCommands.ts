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
      resume: () => api.resume(sid),
      retry: () => api.retry(sid),
      barrier: () => api.barrier(sid),
      inspect: () => api.inspect(sid),
      promote: () => api.promote(sid),
      artifact: (stream: string, version: number) =>
        api.artifact(sid, stream, version),
    }),
    [api, sid],
  );
}
