"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useRef, useState, type FormEvent } from "react";
import { apiRequest, describeError, type FormError, type Schemas } from "@/lib/api/client";
import { parseUsdtInput } from "@/lib/money";
import { Alert } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Field, TextArea } from "@/components/ui/field";
import { Glass } from "@/components/ui/glass";
import { Icon } from "@/components/ui/icons";
import { PageHeader } from "@/components/ui/page";

export function NewLinkScreen() {
  const router = useRouter();
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [amount, setAmount] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<FormError | null>(null);
  // One key per form state: a retry after a dropped connection reuses it (the API then returns the
  // original link instead of making a duplicate), and any edit starts a new one.
  const idempotencyKey = useRef<string | null>(null);

  const edit = (apply: () => void) => {
    idempotencyKey.current = null;
    setError(null);
    apply();
  };

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const parsed = parseUsdtInput(amount);
    if (!parsed) {
      setError({ message: "", fields: { amount: "Enter an amount above zero, such as 4 or 12.50. Up to 6 decimals." } });
      return;
    }
    idempotencyKey.current ??= crypto.randomUUID();
    setBusy(true);
    setError(null);
    try {
      const link = await apiRequest<Schemas["PaymentLink"]>("/v1/payment-links", {
        method: "POST",
        idempotencyKey: idempotencyKey.current,
        body: { title: title.trim(), description: description.trim(), amount: parsed },
      });
      router.push(`/app/links/${link.id}`);
    } catch (failure) {
      setError(describeError(failure));
      setBusy(false);
    }
  }

  return (
    <>
      <PageHeader
        title="New payment link"
        summary="Describe the gig and set the price in USDT."
        back={
          <Link href="/app/links" className="inline-flex min-h-11 items-center gap-1.5 text-sm font-medium text-ink-soft no-underline hover:text-ink">
            <Icon name="back" className="size-4" /> Payment links
          </Link>
        }
      />

      <Glass as="section" size="xl" className="max-w-xl p-6 sm:p-8">
        <form onSubmit={submit} noValidate className="flex flex-col gap-5">
          {error?.message && Object.keys(error.fields).length === 0 ? <Alert>{error.message}</Alert> : null}
          <Field
            label="What is the gig?"
            name="title"
            required
            maxLength={120}
            autoComplete="off"
            placeholder="Flyer design for the youth event"
            value={title}
            onChange={(e) => edit(() => setTitle(e.target.value))}
            error={error?.fields.title}
          />
          <TextArea
            label="Details (optional)"
            name="description"
            maxLength={1000}
            placeholder="What the client gets, and anything they should know."
            value={description}
            onChange={(e) => edit(() => setDescription(e.target.value))}
            error={error?.fields.description}
          />
          <Field
            label="Price"
            name="amount"
            required
            inputMode="decimal"
            autoComplete="off"
            placeholder="4.00"
            suffix="USDT"
            hint="Your client pays exactly this amount, so it can be matched to this gig."
            value={amount}
            onChange={(e) => edit(() => setAmount(e.target.value))}
            error={error?.fields.amount}
          />
          <Alert tone="warning">
            <strong className="font-semibold">USDT on TRON (TRC20) only.</strong> Clients must pay on that network. Money sent on any other
            network is lost.
          </Alert>
          <div className="flex flex-col gap-3 sm:flex-row">
            <Button type="submit" size="lg" loading={busy} className="sm:flex-none">
              Create link
            </Button>
            <Button variant="quiet" size="lg" onClick={() => router.push("/app/links")}>
              Cancel
            </Button>
          </div>
        </form>
      </Glass>
    </>
  );
}
