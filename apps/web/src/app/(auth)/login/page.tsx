import { ScreenScaffold } from "@/components/screen-scaffold";

export const metadata = { title: "Log in" };

export default function LoginPage() {
  return (
    <ScreenScaffold
      title={"Log in"}
      summary={"Welcome back. Your jar is waiting."}
      mustShow={["Email and password form", "Field-level errors from the API error envelope", "Link to registration"]}
      endpoints={["POST /v1/auth/login", "GET /v1/me"]}
    />
  );
}
