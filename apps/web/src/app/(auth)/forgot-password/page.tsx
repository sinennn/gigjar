import { ForgotPasswordForm } from "@/components/auth/auth-forms";
import { AuthHeading } from "@/components/auth/auth-heading";

export const metadata = { title: "Forgot password" };

export default function ForgotPasswordPage() {
  return (
    <div className="space-y-6">
      <AuthHeading
        title="Forgot your password?"
        summary="Enter your email and we will send you a link to choose a new one."
      />
      <ForgotPasswordForm />
    </div>
  );
}
