"use client";

import { ApiDownView } from "@/components/auth/api-down";

/**
 * A screen inside the app threw. (The session check in the layout handles its own failures, because a
 * layout's errors are not caught by this file.)
 */
export default function AppError({ reset }: { error: Error; reset: () => void }) {
  return <ApiDownView onRetry={reset} />;
}
