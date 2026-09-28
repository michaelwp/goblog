import { useSyncExternalStore } from "react";

const subscribe = () => () => {};

// False on the server and during hydration, true afterwards. Components use
// it to render exactly what the server rendered first, then switch to
// browser-only UI (the visual editor, tag chips, local times) without a
// hydration mismatch or a setState-in-effect round trip.
export function useHydrated(): boolean {
  return useSyncExternalStore(
    subscribe,
    () => true,
    () => false,
  );
}
