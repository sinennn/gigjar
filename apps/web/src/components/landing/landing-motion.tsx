"use client";

import { useRef, type ReactNode } from "react";
import gsap from "gsap";
import { ScrollTrigger } from "gsap/ScrollTrigger";
import { useGSAP } from "@gsap/react";
import { liquidLens, supportsLiquidLens } from "@/lib/liquid-glass";
import {
  JAR_AMOUNT_EVENT,
  JAR_PAYMENT_EVENT,
  JAR_PAYOUT_EVENT,
  JAR_POUR_EVENT,
  type JarAmountDetail,
  type JarPaymentDetail,
  type JarPayoutDetail,
} from "./jar-scene";

gsap.registerPlugin(useGSAP, ScrollTrigger);

const q = (name: string) => `[data-a="${name}"]`;

/**
 * Wraps the landing page and plays its motion: the intro, the jar chips and
 * their count, scroll reveals, and the liquid glass refraction. The markup
 * stays server rendered; this only animates it.
 */
export function LandingMotion({ children }: { children: ReactNode }) {
  const root = useRef<HTMLDivElement>(null);

  useGSAP(
    () => {
      const el = root.current;
      if (!el) return;
      const $ = gsap.utils.selector(root);
      const one = (name: string) => $(q(name))[0] as HTMLElement | undefined;
      const mm = gsap.matchMedia();

      // Refraction on wide screens with a mouse, where it stays smooth.
      mm.add("(min-width: 768px) and (pointer: fine)", () => {
        if (!supportsLiquidLens()) return;
        const cleanups = $("[data-lens]").map((node) =>
          liquidLens(node as HTMLElement, { strength: Number((node as HTMLElement).dataset.lens) || 28 }),
        );
        return () => cleanups.forEach((c) => c());
      });

      mm.add("(prefers-reduced-motion: no-preference)", () => {
        const intro = gsap.timeline({ defaults: { ease: "expo.out", duration: 1.1 } });
        intro
          .fromTo(q("nav"), { autoAlpha: 0, y: -16 }, { autoAlpha: 1, y: 0 })
          .fromTo(q("h1"), { autoAlpha: 0 }, { autoAlpha: 1, duration: 0.01 }, 0.1)
          .fromTo(q("line"), { yPercent: 108 }, { yPercent: 0, stagger: 0.1, duration: 1.25 }, 0.1)
          .fromTo(q("fade"), { autoAlpha: 0, y: 22 }, { autoAlpha: 1, y: 0, stagger: 0.08 }, 0.4)
          .fromTo(q("stage"), { autoAlpha: 0, y: 24 }, { autoAlpha: 1, y: 0, duration: 1.4 }, 0.2);

        // Chips arrive with the pour, then follow the jar: its total and
        // each gig payment as it drops in.
        const count = one("count");
        const progress = one("progress");
        const payLabel = one("pay-label");
        const payAmount = one("pay-amount");
        const payChip = $(q("chip"))[0];
        const setAmount = (amount: number) => {
          if (count) count.textContent = `$${amount.toFixed(2)}`;
          if (progress) gsap.to(progress, { scaleX: Math.min(1, amount / 12), duration: 0.35, ease: "power2.out" });
        };
        const onAmount = (e: Event) => setAmount((e as CustomEvent<JarAmountDetail>).detail.amount);
        const payIcon = one("pay-icon");
        const showPay = (label: string, amount: string, out: boolean) => {
          if (payLabel) payLabel.textContent = label;
          if (payAmount) payAmount.textContent = amount;
          payIcon?.toggleAttribute("data-out", out);
          if (payChip) gsap.fromTo(payChip, { scale: 0.94 }, { scale: 1, duration: 0.6, ease: "back.out(2)" });
        };
        const onPayment = (e: Event) => {
          const { label, amount } = (e as CustomEvent<JarPaymentDetail>).detail;
          showPay(label, `+$${amount.toFixed(2)} USDT`, false);
        };
        const onPayout = (e: Event) => {
          const { amount } = (e as CustomEvent<JarPayoutDetail>).detail;
          showPay("Sent to Cleva", `$${amount.toFixed(2)} USDT`, true);
        };
        el.addEventListener(JAR_AMOUNT_EVENT, onAmount);
        el.addEventListener(JAR_PAYMENT_EVENT, onPayment);
        el.addEventListener(JAR_PAYOUT_EVENT, onPayout);

        let shown = false;
        const showChips = (pouring: boolean) => {
          if (shown) return;
          shown = true;
          if (pouring) setAmount(0);
          gsap.fromTo(
            q("chip"),
            { autoAlpha: 0, y: 14, scale: 0.94 },
            { autoAlpha: 1, y: 0, scale: 1, stagger: 0.2, duration: 0.9, ease: "back.out(1.6)", delay: 0.1 },
          );
          gsap.utils.toArray<HTMLElement>(q("chip")).forEach((chip, i) => {
            gsap.to(chip, {
              y: -6,
              duration: 3.2,
              ease: "sine.inOut",
              yoyo: true,
              repeat: -1,
              delay: 1.2 + i * 1.6,
            });
          });
        };
        const onPour = () => showChips(true);
        el.addEventListener(JAR_POUR_EVENT, onPour);
        // Without WebGL there is no pour; show the chips anyway.
        const fallback = window.setTimeout(() => showChips(false), 3500);

        // Scroll reveals.
        gsap.set($(q("reveal")), { autoAlpha: 0, y: 36 });
        ScrollTrigger.batch($(q("reveal")), {
          start: "top 88%",
          once: true,
          onEnter: (els) =>
            gsap.to(els, { autoAlpha: 1, y: 0, stagger: 0.12, duration: 1.1, ease: "expo.out" }),
        });

        const fee = one("fee");
        if (fee) {
          gsap.fromTo(
            fee,
            { yPercent: 40, autoAlpha: 0 },
            {
              yPercent: 0,
              autoAlpha: 1,
              duration: 1.2,
              ease: "expo.out",
              scrollTrigger: { trigger: fee, start: "top 88%", once: true },
            },
          );
        }

        return () => {
          el.removeEventListener(JAR_POUR_EVENT, onPour);
          el.removeEventListener(JAR_AMOUNT_EVENT, onAmount);
          el.removeEventListener(JAR_PAYMENT_EVENT, onPayment);
          el.removeEventListener(JAR_PAYOUT_EVENT, onPayout);
          window.clearTimeout(fallback);
        };
      });

      // The nav glass firms up once the page scrolls.
      const nav = one("nav");
      if (nav) {
        ScrollTrigger.create({ start: 24, end: "max", toggleClass: { targets: nav, className: "is-scrolled" } });
      }
    },
    { scope: root },
  );

  return (
    <div ref={root} className="landing">
      {children}
    </div>
  );
}
