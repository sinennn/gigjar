import { GoogleSignInButton } from "@/components/google-sign-in";
import { ScreenScaffold } from "@/components/screen-scaffold";

export const metadata = { title: "Create your jar" };

export default function RegisterPage() {
  return (
    <div className="space-y-6">
      <div className="max-w-sm">
        <GoogleSignInButton />
      </div>
      <ScreenScaffold
        title={"Create your jar"}
        summary={"One account, one jar, every gig."}
        mustShow={["Display name, email and password (10+ characters)", "Duplicate-email error from 409", "Link to log in", "Continue with Google, which creates the account and its jar"]}
        endpoints={["POST /v1/auth/register", "GET /v1/auth/providers", "GET /v1/auth/google/start (full-page navigation)"]}
      />
    </div>
  );
}
