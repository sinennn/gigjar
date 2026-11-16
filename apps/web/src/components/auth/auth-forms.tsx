"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState, type FormEvent, type ReactNode } from "react";
import { Alert } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Field } from "@/components/ui/field";
import { apiRequest, describeError, type FormError, type Schemas } from "@/lib/api/client";
import { DEFAULT_RETURN } from "@/lib/return-to";

const NO_ERROR: FormError = { message: "", fields: {} };

/** Sends a form to the API and keeps its busy and error state. */
function useSubmit() {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<FormError>(NO_ERROR);
  const run = async (send: () => Promise<void>) => {
    setBusy(true);
    setError(NO_ERROR);
    try {
      await send();
    } catch (e) {
      setError(describeError(e));
      setBusy(false);
    }
  };
  /** Shows field messages found in the browser, without calling the API. */
  const reject = (fields: Record<string, string>) => setError({ message: "", fields });
  return { busy, error, run, reject };
}

/** Keeps "where you were going" on the links between the log in and register pages. */
function withReturn(path: string, returnTo: string) {
  return returnTo === DEFAULT_RETURN ? path : `${path}?return_to=${encodeURIComponent(returnTo)}`;
}

function values(e: FormEvent<HTMLFormElement>) {
  e.preventDefault();
  const data = new FormData(e.currentTarget);
  return (name: string) => String(data.get(name) ?? "");
}

function Footer({ children }: { children: ReactNode }) {
  return <p className="m-0 text-center text-sm text-ink-soft">{children}</p>;
}

const linkClass = "font-medium text-ink underline decoration-line underline-offset-4 hover:decoration-ink";

export function LoginForm({ returnTo = DEFAULT_RETURN }: { returnTo?: string }) {
  const router = useRouter();
  const { busy, error, run, reject } = useSubmit();
  const onSubmit = (e: FormEvent<HTMLFormElement>) => {
    const v = values(e);
    const email = v("email").trim();
    const password = v("password");
    // The API answers an empty login with the same "incorrect" message as a wrong one, which
    // is unhelpful for a form someone simply forgot to fill in.
    const missing: Record<string, string> = {};
    if (!email) missing.email = "Enter your email address.";
    if (!password) missing.password = "Enter your password.";
    if (Object.keys(missing).length) {
      reject(missing);
      return;
    }
    void run(async () => {
      await apiRequest<Schemas["User"]>("/v1/auth/login", { method: "POST", body: { email, password } });
      router.push(returnTo);
      router.refresh();
    });
  };
  return (
    <form onSubmit={onSubmit} className="space-y-4" noValidate>
      {error.message && !Object.keys(error.fields).length ? <Alert>{error.message}</Alert> : null}
      <Field label="Email" name="email" type="email" autoComplete="email" required error={error.fields.email} />
      <Field
        label="Password"
        name="password"
        type="password"
        autoComplete="current-password"
        required
        error={error.fields.password}
      />
      <div className="flex justify-end">
        <Link href="/forgot-password" className={`text-sm ${linkClass}`}>
          Forgot password?
        </Link>
      </div>
      <Button type="submit" size="lg" loading={busy} className="w-full">
        Log in
      </Button>
      <Footer>
        New here?{" "}
        <Link href={withReturn("/register", returnTo)} className={linkClass}>
          Create your jar
        </Link>
      </Footer>
    </form>
  );
}

export function RegisterForm({ returnTo = DEFAULT_RETURN }: { returnTo?: string }) {
  const router = useRouter();
  const { busy, error, run } = useSubmit();
  const onSubmit = (e: FormEvent<HTMLFormElement>) => {
    const v = values(e);
    void run(async () => {
      await apiRequest<Schemas["User"]>("/v1/auth/register", {
        method: "POST",
        body: { display_name: v("display_name").trim(), email: v("email").trim(), password: v("password") },
      });
      router.push(returnTo);
      router.refresh();
    });
  };
  return (
    <form onSubmit={onSubmit} className="space-y-4" noValidate>
      {error.message && !Object.keys(error.fields).length ? <Alert>{error.message}</Alert> : null}
      <Field
        label="Your name"
        name="display_name"
        autoComplete="name"
        required
        maxLength={80}
        error={error.fields.display_name}
      />
      <Field label="Email" name="email" type="email" autoComplete="email" required error={error.fields.email} />
      <Field
        label="Password"
        name="password"
        type="password"
        autoComplete="new-password"
        required
        minLength={10}
        maxLength={128}
        hint="At least 10 characters."
        error={error.fields.password}
      />
      <Button type="submit" size="lg" loading={busy} className="w-full">
        Create your jar
      </Button>
      <Footer>
        Already have a jar?{" "}
        <Link href={withReturn("/login", returnTo)} className={linkClass}>
          Log in
        </Link>
      </Footer>
    </form>
  );
}

export function ForgotPasswordForm() {
  const { busy, error, run } = useSubmit();
  const [sent, setSent] = useState(false);
  const onSubmit = (e: FormEvent<HTMLFormElement>) => {
    const v = values(e);
    void run(async () => {
      await apiRequest("/v1/auth/password/forgot", { method: "POST", body: { email: v("email").trim() } });
      setSent(true);
    });
  };
  if (sent) {
    return (
      <div className="space-y-4">
        <Alert tone="success">If an account uses that email, we have sent a link to it. It works for 30 minutes.</Alert>
        <Footer>
          <Link href="/login" className={linkClass}>
            Back to log in
          </Link>
        </Footer>
      </div>
    );
  }
  return (
    <form onSubmit={onSubmit} className="space-y-4" noValidate>
      {error.message && !Object.keys(error.fields).length ? <Alert>{error.message}</Alert> : null}
      <Field label="Email" name="email" type="email" autoComplete="email" required error={error.fields.email} />
      <Button type="submit" size="lg" loading={busy} className="w-full">
        Send the link
      </Button>
      <Footer>
        Remembered it?{" "}
        <Link href="/login" className={linkClass}>
          Log in
        </Link>
      </Footer>
    </form>
  );
}

export function ResetPasswordForm({ token }: { token: string }) {
  const router = useRouter();
  const { busy, error, run } = useSubmit();
  if (!token) {
    return (
      <div className="space-y-4">
        <Alert>This link is missing its code. Open the link from the email again, or ask for a new one.</Alert>
        <Footer>
          <Link href="/forgot-password" className={linkClass}>
            Send a new link
          </Link>
        </Footer>
      </div>
    );
  }
  const onSubmit = (e: FormEvent<HTMLFormElement>) => {
    const v = values(e);
    void run(async () => {
      await apiRequest<Schemas["User"]>("/v1/auth/password/reset", {
        method: "POST",
        body: { token, password: v("password") },
      });
      router.push("/app");
      router.refresh();
    });
  };
  const expired = error.message && !Object.keys(error.fields).length;
  return (
    <form onSubmit={onSubmit} className="space-y-4" noValidate>
      {expired ? (
        <Alert>
          {error.message}{" "}
          <Link href="/forgot-password" className={linkClass}>
            Send a new link
          </Link>
        </Alert>
      ) : null}
      <Field
        label="New password"
        name="password"
        type="password"
        autoComplete="new-password"
        required
        minLength={10}
        maxLength={128}
        hint="At least 10 characters."
        error={error.fields.password}
      />
      <Button type="submit" size="lg" loading={busy} className="w-full">
        Save and log in
      </Button>
    </form>
  );
}
