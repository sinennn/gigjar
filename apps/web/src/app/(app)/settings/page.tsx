import { ScreenScaffold } from "@/components/screen-scaffold";

export const metadata = { title: "Account settings" };

export default function SettingsPage() {
  return (
    <ScreenScaffold
      title={"Account settings"}
      summary={"Your profile and session."}
      mustShow={["Display name", "Email (read only)", "Log out"]}
      endpoints={["GET /v1/me", "PATCH /v1/me", "POST /v1/auth/logout"]}
    />
  );
}
