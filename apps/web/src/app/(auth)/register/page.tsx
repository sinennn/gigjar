import { ScreenScaffold } from "@/components/screen-scaffold";

export const metadata = { title: "Create your jar" };

export default function RegisterPage() {
  return (
    <ScreenScaffold
      title={"Create your jar"}
      summary={"One account, one jar, every gig."}
      mustShow={["Display name, email and password (10+ characters)", "Duplicate-email error from 409", "Link to log in"]}
      endpoints={["POST /v1/auth/register"]}
    />
  );
}
