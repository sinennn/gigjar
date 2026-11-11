"use client";

import { useState, type FormEvent } from "react";
import { ApiError, apiRequest, describeError, type FormError, type Schemas } from "@/lib/api/client";
import { errorText, useApi } from "@/lib/api/hooks";
import { formatWhen, networkLabel } from "@/lib/format";
import { useJar } from "@/components/app/jar-context";
import { Alert } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { CopyButton } from "@/components/ui/copy-button";
import { Field } from "@/components/ui/field";
import { Glass } from "@/components/ui/glass";
import { ErrorCard, LoadingCard, PageHeader } from "@/components/ui/page";

type Cleva = Schemas["ClevaDestination"];

const TRON_ADDRESS = /^T[1-9A-HJ-NP-Za-km-z]{33}$/;

export function WalletScreen() {
  const jar = useJar();
  const { data, error, loading, reload } = useApi<Cleva>("/v1/wallet/cleva");
  const [saved, setSaved] = useState<Cleva | null>(null);
  const [address, setAddress] = useState("");
  const [busy, setBusy] = useState(false);
  const [formError, setFormError] = useState<FormError | null>(null);
  const [justSaved, setJustSaved] = useState(false);

  // A 404 here just means no address has been set yet.
  const notSet = error instanceof ApiError && error.status === 404;
  const current = saved ?? data;

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const value = address.trim();
    if (!TRON_ADDRESS.test(value)) {
      setFormError({
        message: "",
        fields: { address: "That does not look like a TRON address. It starts with T and is 34 characters long." },
      });
      return;
    }
    setBusy(true);
    setFormError(null);
    setJustSaved(false);
    try {
      const result = await apiRequest<Cleva>("/v1/wallet/cleva", { method: "PUT", body: { address: value } });
      setSaved(result);
      setAddress("");
      setJustSaved(true);
      jar.reload();
    } catch (failure) {
      setFormError(describeError(failure));
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <PageHeader title="Cleva wallet" summary="Where your jar sends money once it is full." />

      {error && !notSet && !current ? <ErrorCard message={errorText(error)} onRetry={reload} /> : null}
      {loading && !current && !error ? <LoadingCard lines={3} /> : null}

      {current || notSet ? (
        <div className="flex max-w-2xl flex-col gap-4 sm:gap-5">
          <Glass as="section" size="lg" className="space-y-4 p-5 sm:p-6">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <h2 className="m-0 font-display text-xl font-bold tracking-[-0.02em]">Current address</h2>
              {current ? <Badge tone="neutral">{networkLabel(current.network)}</Badge> : null}
            </div>
            {current ? (
              <>
                <p className="m-0 rounded-2xl bg-paper/80 px-4 py-3 font-mono text-[15px] leading-relaxed break-all text-ink">
                  {current.address}
                </p>
                <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
                  <CopyButton value={current.address} label="Copy address" />
                  <span className="text-sm text-muted">Updated {formatWhen(current.updated_at)}</span>
                </div>
              </>
            ) : (
              <Alert tone="warning">
                No Cleva address yet. Until you add one, your jar has nowhere to send its payout.
              </Alert>
            )}
          </Glass>

          <Glass as="section" size="xl" className="p-5 sm:p-8">
            <form onSubmit={submit} noValidate className="flex flex-col gap-5">
              <h2 className="m-0 font-display text-xl font-bold tracking-[-0.02em]">
                {current ? "Change address" : "Add your address"}
              </h2>
              {justSaved ? (
                <Alert tone="success">Saved. Payouts already on their way keep the address they started with.</Alert>
              ) : null}
              {formError?.message && Object.keys(formError.fields).length === 0 ? <Alert>{formError.message}</Alert> : null}
              <Field
                label="Cleva USDT deposit address (TRC20)"
                name="address"
                required
                spellCheck={false}
                autoCapitalize="none"
                autoComplete="off"
                placeholder="T..."
                className="font-mono"
                value={address}
                onChange={(e) => {
                  setAddress(e.target.value);
                  setFormError(null);
                  setJustSaved(false);
                }}
                error={formError?.fields.address}
              />
              <Alert tone="warning">
                <strong className="font-semibold">USDT on TRON (TRC20) only.</strong> Use the TRC20 deposit address from your Cleva
                account. An address for any other network can lose your money for good.
              </Alert>
              <p className="m-0 text-sm leading-relaxed text-muted">
                Changing the address affects future payouts only. A payout that is already on its way keeps the address it started with.
              </p>
              <div>
                <Button type="submit" size="lg" loading={busy}>
                  {current ? "Save new address" : "Save address"}
                </Button>
              </div>
            </form>
          </Glass>
        </div>
      ) : null}
    </>
  );
}
