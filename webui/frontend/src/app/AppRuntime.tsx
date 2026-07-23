import { useEffect, type ReactNode } from "react";
import { normalizeRoute } from "../routeHash";
import {
  AppStoreProvider,
  appStore,
  type AppStore,
  useAppStoreApi,
  useStore,
} from "../store";
import { AppShell } from "./AppShell";
import {
  AppServicesProvider,
  productionAppServices,
  type AppServices,
  useAppServices,
} from "./appServices";

export interface AppRuntimeProps {
  store?: AppStore;
  services?: AppServices;
  children?: ReactNode;
}

function RuntimeController({ children }: { children: ReactNode }) {
  const services = useAppServices();
  const store = useAppStoreApi();
  const refreshHealth = useStore((state) => state.refreshHealth);
  const refreshSessions = useStore((state) => state.refreshSessions);
  const refreshRuns = useStore((state) => state.refreshRuns);
  const refreshProjects = useStore((state) => state.refreshProjects);
  const select = useStore((state) => state.select);
  const selectRun = useStore((state) => state.selectRun);
  const showPage = useStore((state) => state.showPage);
  const showScheduledDetail = useStore((state) => state.showScheduledDetail);

  useEffect(() => {
    void refreshHealth();
    void refreshSessions();
    void refreshRuns();
    void refreshProjects();

    const healthTimer = services.clock.setInterval(refreshHealth, 5000);
    const sessionsTimer = services.clock.setInterval(refreshSessions, 4000);
    const runsTimer = services.clock.setInterval(refreshRuns, 4000);
    const projectsTimer = services.clock.setInterval(refreshProjects, 8000);

    const route = (rawValue: string) => {
      const raw = normalizeRoute(rawValue);
      const state = store.getState();
      if (raw === "scheduled") {
        showPage(raw);
      } else if (raw.startsWith("scheduled:")) {
        const sid = raw.slice("scheduled:".length);
        if (sid) showScheduledDetail(sid);
      } else if (raw.startsWith("run:")) {
        const rid = raw.slice(4);
        if (rid && rid !== state.currentRunId) selectRun(rid);
      } else if (raw && raw !== state.currentSid) {
        select(raw);
      } else if (
        !raw &&
        (state.currentSid || state.currentRunId || state.currentPage !== "home")
      ) {
        showPage("home");
      }
    };

    const initialHash = services.navigation.hash();
    if (initialHash) route(initialHash);
    const stopListening = services.navigation.listen(() => {
      route(services.navigation.hash());
    });

    // Browser notification permission must follow a user gesture. Runtime owns
    // this production-only side effect; isolated AppShell stories never ask.
    const requestPermission = () => services.notifications.requestPermission();
    window.addEventListener("pointerdown", requestPermission, { once: true });

    return () => {
      services.clock.clearInterval(healthTimer);
      services.clock.clearInterval(sessionsTimer);
      services.clock.clearInterval(runsTimer);
      services.clock.clearInterval(projectsTimer);
      stopListening();
      window.removeEventListener("pointerdown", requestPermission);
    };
  }, [
    refreshHealth,
    refreshProjects,
    refreshRuns,
    refreshSessions,
    select,
    selectRun,
    services,
    showPage,
    showScheduledDetail,
    store,
  ]);

  return children;
}

export function AppRuntime({
  store = appStore,
  services = productionAppServices,
  children,
}: AppRuntimeProps) {
  return (
    <AppServicesProvider services={services}>
      <AppStoreProvider store={store}>
        <RuntimeController>
          {children ?? <AppShell />}
        </RuntimeController>
      </AppStoreProvider>
    </AppServicesProvider>
  );
}
