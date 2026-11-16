import { Suspense } from "react";
import { LoginForm } from "@/components/auth/auth-forms";
import { AuthHeading } from "@/components/auth/auth-heading";
import { GoogleGate } from "@/components/auth/google-gate";
import { SignedInRedirect } from "@/components/auth/signed-in-redirect";
import { GoogleSignInError } from "@/components/google-sign-in";
import { safeReturnTo } from "@/lib/return-to";

export const metadata = { title: "Log in" };

export default async function LoginPage({ searchParams }: PageProps<"/login">) {
  const { error, return_to } = await searchParams;
  const returnTo = safeReturnTo(return_to);
  return (
    <div className="space-y-6">
      <Suspense>
        <SignedInRedirect to={returnTo} />
      </Suspense>
      <AuthHeading title="Log in" summary="Welcome back. Your jar is waiting." />
      <GoogleSignInError code={typeof error === "string" ? error : undefined} />
      <GoogleGate divider="or with email" returnTo={returnTo} />
      <LoginForm returnTo={returnTo} />
    </div>
  );
}
