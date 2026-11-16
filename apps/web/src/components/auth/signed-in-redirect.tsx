import { redirect } from "next/navigation";
import { getCurrentUser } from "@/lib/api/server";

/**
 * Put on the log in and register pages: someone who already has a session goes straight on to
 * where they were headed instead of seeing a form they do not need. If the API cannot be reached
 * the form is shown anyway, so a slow or sleeping API never blocks the page.
 */
export async function SignedInRedirect({ to }: { to: string }) {
  let signedIn = false;
  try {
    signedIn = (await getCurrentUser()) !== null;
  } catch {
    signedIn = false;
  }
  if (signedIn) {
    redirect(to);
  }
  return null;
}
