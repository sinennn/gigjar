import { LoginForm } from "@/components/auth/auth-forms";
import { AuthHeading } from "@/components/auth/auth-heading";
import { GoogleGate } from "@/components/auth/google-gate";
import { GoogleSignInError } from "@/components/google-sign-in";

export const metadata = { title: "Log in" };

export default async function LoginPage({ searchParams }: PageProps<"/login">) {
  const { error } = await searchParams;
  return (
    <div className="space-y-6">
      <AuthHeading title="Log in" summary="Welcome back. Your jar is waiting." />
      <GoogleSignInError code={typeof error === "string" ? error : undefined} />
      <GoogleGate divider="or with email" />
      <LoginForm />
    </div>
  );
}
