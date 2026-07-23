import { useState, type ReactNode } from "react";
import { AppServicesProvider } from "../app/appServices";
import {
  AppStoreProvider,
  createAppStore,
  type AppState,
} from "../store";
import { createStoryAppServices, type StoryAppServicesOptions } from "./appServices";

export interface StoryAppFrameProps {
  children: ReactNode;
  initialState?: Partial<AppState>;
  services?: StoryAppServicesOptions;
}

// A fresh frame owns a fresh service harness and Zustand store. Story remount,
// Demo Reset, and two canvases on the same page therefore cannot share request
// coalescing, ids, preferences, drafts, navigation, or stream listeners.
export function StoryAppFrame({
  children,
  initialState,
  services: serviceOptions,
}: StoryAppFrameProps) {
  const [runtime] = useState(() => {
    const harness = createStoryAppServices(serviceOptions);
    const store = createAppStore(harness.services);
    if (initialState) store.setState(initialState);
    return { harness, store };
  });

  return (
    <AppServicesProvider services={runtime.harness.services}>
      <AppStoreProvider store={runtime.store}>
        {children}
      </AppStoreProvider>
    </AppServicesProvider>
  );
}
