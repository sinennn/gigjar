import { ResetPasswordForm } from "@/components/auth/auth-forms";
import { AuthHeading } from "@/components/auth/auth-heading";

export const metadata = { title: "Choose a new password" };

export default async function ResetPasswordPage({ searchParams }: PageProps<"/reset-password">) {
  const { token } = await searchParams;
  return (
    <div className="space-y-6">
      <AuthHeading title="Choose a new password" summary="You will be logged in once it is saved." />
      <ResetPasswordForm token={typeof token === "string" ? token : ""} />
    </div>
  );
}
