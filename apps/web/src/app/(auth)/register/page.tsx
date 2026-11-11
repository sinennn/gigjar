import { RegisterForm } from "@/components/auth/auth-forms";
import { AuthHeading } from "@/components/auth/auth-heading";
import { GoogleGate } from "@/components/auth/google-gate";

export const metadata = { title: "Create your jar" };

export default function RegisterPage() {
  return (
    <div className="space-y-6">
      <AuthHeading title="Create your jar" summary="One account, one jar, every gig." />
      <GoogleGate divider="or with email" />
      <RegisterForm />
    </div>
  );
}
