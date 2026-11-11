"use client";

import { useCallback, useEffect, useReducer, useState } from "react";
import { ApiError, apiRequest } from "./client";

export type Page<T> = { items: T[]; next_cursor?: string | null };

type Failure = ApiError | Error;

function isAbort(error: unknown): boolean {
  return error instanceof DOMException && error.name === "AbortError";
}

/** A 401 anywhere in the signed-in app means the session ended: go back to the login page. */
function handleFailure(error: unknown): Failure {
  if (error instanceof ApiError && error.status === 401) {
    window.location.assign("/login");
  }
  return error instanceof Error ? error : new Error("request failed");
}

export function errorText(error: Failure | null): string {
  if (!error) {
    return "";
  }
  if (error instanceof ApiError) {
    return error.message;
  }
  return "We couldn't reach the server. Check your connection and try again.";
}

type ApiState<T> = { path: string | null; data: T | null; error: Failure | null };

/**
 * Loads one GET endpoint in the browser. `pollMs` re-fetches on a timer (paused while the tab is
 * hidden). Changing `path` starts a fresh load; `reload()` fetches again on demand.
 */
export function useApi<T>(path: string | null, options: { pollMs?: number } = {}) {
  const { pollMs } = options;
  const [state, setState] = useState<ApiState<T>>({ path: null, data: null, error: null });
  const [tick, setTick] = useState(0);

  useEffect(() => {
    if (path === null) {
      return;
    }
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;

    const load = async () => {
      if (!document.hidden || timer === undefined) {
        try {
          const data = await apiRequest<T>(path, { signal: controller.signal });
          setState({ path, data, error: null });
        } catch (error) {
          if (isAbort(error) || controller.signal.aborted) {
            return;
          }
          const failure = handleFailure(error);
          setState((previous) => ({ path, data: previous.path === path ? previous.data : null, error: failure }));
        }
      }
      if (pollMs && !controller.signal.aborted) {
        timer = setTimeout(load, pollMs);
      }
    };
    void load();

    return () => {
      controller.abort();
      if (timer !== undefined) {
        clearTimeout(timer);
      }
    };
  }, [path, pollMs, tick]);

  const current = state.path === path;
  return {
    data: current ? state.data : null,
    error: current ? state.error : null,
    loading: path !== null && (!current || (state.data === null && state.error === null)),
    reload: useCallback(() => setTick((value) => value + 1), []),
  };
}

type ListState<T> = {
  path: string | null;
  items: T[];
  next: string | null;
  error: Failure | null;
  done: boolean;
  more: boolean;
};

type ListAction<T> =
  | { type: "reset"; path: string | null }
  | { type: "page"; path: string; page: Page<T>; append: boolean }
  | { type: "error"; path: string; error: Failure }
  | { type: "more" };

function listReducer<T>(state: ListState<T>, action: ListAction<T>): ListState<T> {
  switch (action.type) {
    case "reset":
      return { path: action.path, items: [], next: null, error: null, done: false, more: false };
    case "page":
      if (state.path !== action.path) return state;
      return {
        ...state,
        items: action.append ? [...state.items, ...action.page.items] : action.page.items,
        next: action.page.next_cursor ?? null,
        error: null,
        done: true,
        more: false,
      };
    case "error":
      if (state.path !== action.path) return state;
      return { ...state, error: action.error, done: true, more: false };
    case "more":
      return { ...state, more: true, error: null };
  }
}

/** A cursor-paginated list endpoint: loads the first page, then appends more on request. */
export function useCursorList<T>(path: string | null) {
  const [state, dispatch] = useReducer(
    listReducer<T>,
    path,
    (initial): ListState<T> => ({ path: initial, items: [], next: null, error: null, done: false, more: false }),
  );
  const [tick, setTick] = useState(0);

  // Reset during render when the path or the refresh counter changes, so no effect sets state.
  const key = `${path ?? ""}#${tick}`;
  const [seenKey, setSeenKey] = useState(key);
  if (seenKey !== key) {
    setSeenKey(key);
    dispatch({ type: "reset", path });
  }

  const ready = state.path === path && seenKey === key;

  useEffect(() => {
    if (path === null) {
      return;
    }
    const controller = new AbortController();
    apiRequest<Page<T>>(path, { signal: controller.signal })
      .then((page) => dispatch({ type: "page", path, page, append: false }))
      .catch((error) => {
        if (!isAbort(error)) dispatch({ type: "error", path, error: handleFailure(error) });
      });
    return () => controller.abort();
  }, [path, tick]);

  const loadMore = useCallback(() => {
    if (path === null || state.next === null || state.more) {
      return;
    }
    dispatch({ type: "more" });
    const separator = path.includes("?") ? "&" : "?";
    apiRequest<Page<T>>(`${path}${separator}cursor=${encodeURIComponent(state.next)}`)
      .then((page) => dispatch({ type: "page", path, page, append: true }))
      .catch((error) => dispatch({ type: "error", path, error: handleFailure(error) }));
  }, [path, state.next, state.more]);

  return {
    items: ready ? state.items : [],
    hasMore: ready && state.next !== null,
    loading: path !== null && (!ready || !state.done),
    loadingMore: ready && state.more,
    error: ready ? state.error : null,
    loadMore,
    reload: useCallback(() => setTick((value) => value + 1), []),
  };
}
