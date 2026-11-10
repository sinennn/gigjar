import { GoogleSignInButton, GoogleSignInError } from "@/components/google-sign-in";
import { ScreenScaffold } from "@/components/screen-scaffold";

export const metadata = { title: "Log in" };

export default async function LoginPage({ searchParams }: PageProps<"/login">) {
  const { error } = await searchParams;
  return (
    <div className="space-y-6">
      <div className="max-w-sm space-y-3">
        <GoogleSignInError code={typeof error === "string" ? error : undefined} />
        <GoogleSignInButton />
      </div>
      <ScreenScaffold
        title={"Log in"}
        summary={"Welcome back. Your jar is waiting."}
        mustShow={["Email and password form", "Field-level errors from the API error envelope", "Link to registration", "Continue with Google (shown only when GET /v1/auth/providers says google: true)", "The ?error= message after a failed Google sign-in"]}
        endpoints={["POST /v1/auth/login", "GET /v1/me", "GET /v1/auth/providers", "GET /v1/auth/google/start (full-page navigation)"]}
      />
    </div>
  );
}
