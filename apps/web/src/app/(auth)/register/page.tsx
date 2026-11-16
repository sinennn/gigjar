import { Suspense } from "react";
import { RegisterForm } from "@/components/auth/auth-forms";
import { AuthHeading } from "@/components/auth/auth-heading";
import { GoogleGate } from "@/components/auth/google-gate";
import { SignedInRedirect } from "@/components/auth/signed-in-redirect";
import { safeReturnTo } from "@/lib/return-to";

export const metadata = { title: "Create your jar" };

export default async function RegisterPage({ searchParams }: PageProps<"/register">) {
  const { return_to } = await searchParams;
  const returnTo = safeReturnTo(return_to);
  return (
    <div className="space-y-6">
      <Suspense>
        <SignedInRedirect to={returnTo} />
      </Suspense>
      <AuthHeading title="Create your jar" summary="One account, one jar, every gig." />
      <GoogleGate divider="or with email" returnTo={returnTo} />
      <RegisterForm returnTo={returnTo} />
    </div>
  );
}
