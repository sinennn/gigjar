"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";
import { apiRequest, describeError, type FormError, type Schemas } from "@/lib/api/client";
import { formatDay } from "@/lib/format";
import { useUser } from "@/components/app/jar-context";
import { Alert } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Field } from "@/components/ui/field";
import { Glass } from "@/components/ui/glass";
import { PageHeader } from "@/components/ui/page";

export function SettingsScreen() {
  const user = useUser();
  const router = useRouter();
  const [name, setName] = useState(user.display_name);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<FormError | null>(null);
  const [saved, setSaved] = useState(false);
  const [leaving, setLeaving] = useState(false);

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const value = name.trim();
    if (!value) {
      setError({ message: "", fields: { display_name: "Enter the name you want on your payment pages." } });
      return;
    }
    setBusy(true);
    setError(null);
    setSaved(false);
    try {
      await apiRequest<Schemas["User"]>("/v1/me", { method: "PATCH", body: { display_name: value } });
      setSaved(true);
      router.refresh();
    } catch (failure) {
      setError(describeError(failure));
    } finally {
      setBusy(false);
    }
  }

  async function logOut() {
    setLeaving(true);
    try {
      await apiRequest<void>("/v1/auth/logout", { method: "POST" });
    } catch {
      // The session is gone or the API is unreachable; either way, leave this screen.
    }
    router.replace("/login");
    router.refresh();
  }

  return (
    <>
      <PageHeader title="Account settings" summary="Your profile and session." />

      <div className="flex max-w-2xl flex-col gap-4 sm:gap-5">
        <Glass as="section" size="xl" className="p-5 sm:p-8">
          <form onSubmit={save} noValidate className="flex flex-col gap-5">
            <h2 className="m-0 font-display text-xl font-bold tracking-[-0.02em]">Profile</h2>
            {saved ? <Alert tone="success">Saved.</Alert> : null}
            {error?.message && Object.keys(error.fields).length === 0 ? <Alert>{error.message}</Alert> : null}
            <Field
              label="Display name"
              name="display_name"
              required
              maxLength={80}
              autoComplete="name"
              hint="Clients see this on your payment pages."
              value={name}
              onChange={(e) => {
                setName(e.target.value);
                setSaved(false);
                setError(null);
              }}
              error={error?.fields.display_name}
            />
            <Field
              label="Email"
              name="email"
              type="email"
              readOnly
              value={user.email}
              hint={`Member since ${formatDay(user.created_at)}. Your email cannot be changed here.`}
            />
            <div>
              <Button type="submit" loading={busy} disabled={name.trim() === user.display_name}>
                Save changes
              </Button>
            </div>
          </form>
        </Glass>

        <Glass as="section" size="lg" lens={false} className="space-y-4 p-5 sm:p-6">
          <h2 className="m-0 font-display text-xl font-bold tracking-[-0.02em]">Session</h2>
          <p className="m-0 max-w-xl text-[15px] leading-relaxed text-ink-soft">
            Want a new password, or to add one to an account you made with Google?{" "}
            <Link href="/forgot-password" className="font-semibold text-ink underline underline-offset-4">
              Send yourself a reset link
            </Link>
            .
          </p>
          <Button variant="secondary" loading={leaving} onClick={logOut}>
            Log out
          </Button>
        </Glass>
      </div>
    </>
  );
}
