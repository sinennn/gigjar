import { ScreenScaffold } from "@/components/screen-scaffold";

export const metadata = { title: "Choose a new password" };

export default function ResetPasswordPage() {
  return (
    <ScreenScaffold
      title={"Choose a new password"}
      summary={"Opened from the reset email. The token is in the ?token= query parameter."}
      mustShow={[
        "New password field (10 to 128 characters) and a Save button",
        "On success the user is signed in (session cookie set): go to /dashboard",
        "400 reset_token_invalid: \"This link is invalid, used or expired\", with a link to /forgot-password",
        "Note that saving signs out every other device",
      ]}
      endpoints={["POST /v1/auth/password/reset"]}
    />
  );
}
