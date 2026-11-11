import { ScreenScaffold } from "@/components/screen-scaffold";

export const metadata = { title: "Forgot password" };

export default function ForgotPasswordPage() {
  return (
    <ScreenScaffold
      title={"Forgot your password?"}
      summary={"Enter your email and we'll send you a link to choose a new one."}
      mustShow={[
        "Email field and a Send link button",
        "After submitting, always the same message: \"If an account exists for that email, we've sent a link\". Never reveal whether the account exists",
        "429 too_many_attempts: \"Too many requests, try again later\"",
        "Link back to log in",
      ]}
      endpoints={["POST /v1/auth/password/forgot"]}
    />
  );
}
