"use client";

import { useEffect, useState } from "react";

/**
 * True once something has been busy for `afterMs`. The first request after a quiet spell can take
 * up to a minute while the API wakes up; saying so keeps people from thinking the form is broken
 * and pressing the button again.
 */
export function useSlow(busy: boolean, afterMs = 4000): boolean {
  const [slow, setSlow] = useState(false);
  useEffect(() => {
    if (!busy) {
      return;
    }
    const timer = setTimeout(() => setSlow(true), afterMs);
    return () => {
      clearTimeout(timer);
      setSlow(false);
    };
  }, [busy, afterMs]);
  return slow;
}
