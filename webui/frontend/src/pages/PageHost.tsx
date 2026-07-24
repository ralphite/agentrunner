import { ErrorBoundary } from "../components/ErrorBoundary";
import { Home } from "../components/Home";
import { RunView } from "../components/RunView";
import { Scheduled } from "../components/Scheduled";
import { SessionView } from "../components/SessionView";
import type { Page } from "../store";

export interface PageHostProps {
  currentRunId: string | null;
  currentSid: string | null;
  currentPage: Page;
  mobileNavigationOpen?: boolean;
}

/**
 * Owns route precedence and page-level error isolation.
 *
 * AppShell supplies application chrome and overlays; feature controllers live
 * below this boundary. Keeping route selection here prevents the shell from
 * growing another conditional each time a top-level destination is added.
 */
export function PageHost({
  currentRunId,
  currentSid,
  currentPage,
  mobileNavigationOpen = false,
}: PageHostProps) {
  const resetKey = currentRunId || currentSid || currentPage;

  return (
    <ErrorBoundary resetKey={resetKey}>
      {currentRunId ? (
        <RunView runId={currentRunId} />
      ) : currentSid ? (
        <SessionView
          sid={currentSid}
          key={currentSid}
          mobileNavigationOpen={mobileNavigationOpen}
        />
      ) : currentPage === "scheduled" ? (
        <Scheduled />
      ) : (
        <Home />
      )}
    </ErrorBoundary>
  );
}
