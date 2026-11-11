"use client";

import { useEffect, useRef, useState } from "react";
import gsap from "gsap";
import type * as Three from "three";
import type RapierModule from "@dimforge/rapier3d-compat";
import { JarMark } from "@/components/jar-mark";

/** Fired on the host element when the first coins start pouring in. */
export const JAR_POUR_EVENT = "jar:pour";
/** Fired whenever the money in the jar changes. detail: { amount } */
export const JAR_AMOUNT_EVENT = "jar:amount";
/** Fired when a gig payment starts dropping in. detail: { label, amount } */
export const JAR_PAYMENT_EVENT = "jar:payment";
/** Fired when the jar reaches $12 and sweeps to Cleva. detail: { amount, fee } */
export const JAR_PAYOUT_EVENT = "jar:payout";

export type JarAmountDetail = { amount: number };
export type JarPaymentDetail = { label: string; amount: number };
export type JarPayoutDetail = { amount: number; fee: number };

const CREAM = "#f6f3ec";
const INK = "#13263a";
const LID = "#e8922f";
const GOLD = "#d6a347";
const GREEN = "#2b8468";
const TOP_FADE = "linear-gradient(to bottom, transparent 0%, #000 14%)";

// Jar measurements, in scene units (the jar is 2 units wide).
const BASE_TOP = 0.1;
const INNER_R = 0.955;
const COIN_R = 0.2;
const COIN_H = 0.045;
const LINE_Y = 2.0; // the $12 mark
const LID_Y = 2.61;
const LID_TOP = LID_Y + 0.345;

// Money. Each coin is worth 6 cents: the opening pour is $8.40 and a full
// jar, at the $12 payout, holds 200 coins.
const COIN_VALUE = 0.06;
const THRESHOLD = 12;
const FEE = 1;
const START_COINS = 140;
const CAPACITY = 210;
// Payments that take the opening $8.40 to $12, then each later cycle from $0 to $12.
const FIRST_PAYMENTS: JarPaymentDetail[] = [
  { label: "Logo touch-up", amount: 1.2 },
  { label: "Essay proofreading", amount: 2.4 },
];
const CYCLE_PAYMENTS: JarPaymentDetail[] = [
  { label: "Flyer design", amount: 3 },
  { label: "CV rewrite", amount: 1.8 },
  { label: "Logo touch-up", amount: 2.4 },
  { label: "Spreadsheet cleanup", amount: 1.8 },
  { label: "Essay proofreading", amount: 3 },
];

// Timing, in seconds.
const POUR_AT = 0.35;
const POUR_SPREAD = 2.1;
const LID_AT = POUR_AT + POUR_SPREAD + 1.1;
const FIRST_PAYMENT = LID_AT + 1.6;
const SLOT_EVERY = 0.07;
const AFTER_PAYMENT = 1.8; // pause between one payment's last coin and the next payment
const LEAVE_EVERY = 0.014; // payout: one coin out every 14 ms
const LEAVE_TIME = 0.75; // each coin's trip from the pile out through the slot
const AFTER_PAYOUT = 1.6;

// Physics.
const STEP = 1 / 60;
const GRAVITY = -16;
const GROUP_STATIC = 0x0001;
const GROUP_COIN = 0x0002;
const GROUP_ENTERING = 0x0004;
const groups = (member: number, filter: number) => (member << 16) | filter;

// Spin.
const BASE_SPIN = 0.38; // rad/s
const DRAG_RATE = 0.01; // rad per px

/**
 * A glass tip jar that fills with coins, rendered with three.js and
 * simulated with Rapier. It turns slowly all the time; dragging it changes
 * the direction. Gig payments drop in through the lid until the jar holds
 * $12; then every coin streams back out through the slot, the way a payout
 * sweeps the jar to Cleva, and it starts filling again. Falls back to the
 * flat mark when WebGL is unavailable.
 */
export function JarScene() {
  const host = useRef<HTMLDivElement>(null);
  const [state, setState] = useState<"loading" | "ready" | "failed">("loading");

  useEffect(() => {
    const el = host.current;
    if (!el) return;
    let disposed = false;
    let stop = () => {};
    mount(el, () => !disposed && setState("ready"))
      .then((cleanup) => {
        if (disposed) cleanup();
        else stop = cleanup;
      })
      .catch(() => {
        if (!disposed) setState("failed");
      });
    return () => {
      disposed = true;
      stop();
    };
  }, []);

  return (
    <div
      ref={host}
      className={`absolute inset-0 transition-opacity duration-700 ${state === "ready" ? "opacity-100" : "opacity-0"}`}
      // The top edge fades into the page, so coins falling in from above fade in instead of being cut off.
      style={{ maskImage: TOP_FADE, WebkitMaskImage: TOP_FADE }}
    >
      {state === "failed" && (
        <div className="flex h-full items-center justify-center">
          <JarMark className="h-1/2 w-1/2 text-ink" />
        </div>
      )}
    </div>
  );
}

type Coin = {
  index: number;
  body: RapierModule.RigidBody;
  collider: RapierModule.Collider;
  entering: boolean;
};

async function mount(host: HTMLDivElement, onReady: () => void): Promise<() => void> {
  const [THREE, { RoomEnvironment }, { RoundedBoxGeometry }, RAPIER] = await Promise.all([
    import("three"),
    import("three/addons/environments/RoomEnvironment.js"),
    import("three/addons/geometries/RoundedBoxGeometry.js"),
    import("@dimforge/rapier3d-compat").then(async (m) => {
      await m.default.init();
      return m.default;
    }),
  ]);

  const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  const rand = seeded(7);
  const disposables: { dispose: () => void }[] = [];
  const keep = <T extends { dispose: () => void }>(x: T) => (disposables.push(x), x);
  const emit = <T,>(name: string, detail?: T) =>
    host.dispatchEvent(new CustomEvent(name, { bubbles: true, detail }));

  // Renderer. Opaque, with the page's cream as the clear colour, so glass
  // transmission has a real background to refract.
  const canvas = document.createElement("canvas");
  canvas.setAttribute("aria-hidden", "true");
  canvas.style.cssText = "position:absolute;inset:0;width:100%;height:100%;display:block";
  const renderer = new THREE.WebGLRenderer({ canvas, antialias: true, powerPreference: "high-performance" });
  // It renders every frame while on screen, so phones get a lighter pixel ratio.
  const coarse = window.matchMedia("(pointer: coarse)").matches;
  renderer.setPixelRatio(Math.min(window.devicePixelRatio, coarse ? 1.5 : 2));
  renderer.toneMapping = THREE.NeutralToneMapping;
  renderer.toneMappingExposure = 1.05;
  host.appendChild(canvas);

  const scene = new THREE.Scene();
  scene.background = new THREE.Color(CREAM);

  const pmrem = keep(new THREE.PMREMGenerator(renderer));
  const env = keep(pmrem.fromScene(new RoomEnvironment(), 0.035));
  scene.environment = env.texture;
  scene.environmentIntensity = 0.95;

  const key = new THREE.DirectionalLight(0xfff1e0, 1.5);
  key.position.set(-3, 6, 4);
  scene.add(key);
  const rim = new THREE.DirectionalLight(0xe8f4ff, 0.6);
  rim.position.set(4, 3, -3);
  scene.add(rim);

  const camera = new THREE.PerspectiveCamera(26, 1, 0.1, 50);
  camera.position.set(0, 4.1, 9.7);
  camera.lookAt(0, 1.5, 0);

  // Everything that turns with the jar. The floor shadow stays put. Physics
  // runs in this group's frame, so the coins turn with the glass.
  const spinner = new THREE.Group();
  scene.add(spinner);

  // Floor shadow.
  const shadow = new THREE.Mesh(
    keep(new THREE.PlaneGeometry(3.6, 3.6)),
    keep(
      new THREE.MeshBasicMaterial({
        map: keep(shadowTexture(THREE)),
        transparent: true,
        depthWrite: false,
        toneMapped: false,
      }),
    ),
  );
  shadow.rotation.x = -Math.PI / 2;
  shadow.position.y = 0.002;
  scene.add(shadow);

  // The jar: a lathe of one closed profile, so both walls and the thick
  // base are real glass.
  const jarProfile = [
    [0, 0], [0.8, 0], [0.93, 0.03], [0.985, 0.1], [1, 0.2], [1, 2.05], [0.98, 2.2], [0.92, 2.32],
    [0.84, 2.42], [0.8, 2.48], [0.8, 2.8], [0.785, 2.83], [0.76, 2.83], [0.745, 2.8], [0.745, 2.48],
    [0.79, 2.4], [0.88, 2.3], [0.94, 2.18], [INNER_R, 2.05], [INNER_R, 0.24], [0.94, 0.15], [0.88, 0.11],
    [0, BASE_TOP],
  ].map(([x, y]) => new THREE.Vector2(x, y));
  const glass = keep(
    new THREE.MeshPhysicalMaterial({
      color: 0xffffff,
      metalness: 0,
      roughness: 0.02,
      transmission: 1,
      thickness: 0.12,
      ior: 1.5,
      attenuationColor: new THREE.Color("#d5e9e0"),
      attenuationDistance: 1.8,
      clearcoat: 1,
      clearcoatRoughness: 0.02,
      specularIntensity: 1,
    }),
  );
  spinner.add(new THREE.Mesh(keep(new THREE.LatheGeometry(jarProfile, 160)), glass));

  // The $12 mark printed on the glass.
  const mark = new THREE.Mesh(
    keep(new THREE.TorusGeometry(1.002, 0.0045, 6, 200)),
    keep(new THREE.MeshBasicMaterial({ color: INK, transparent: true, opacity: 0.4, toneMapped: false })),
  );
  mark.rotation.x = Math.PI / 2;
  mark.position.y = LINE_Y;
  spinner.add(mark);
  const labelAngle = 0.42;
  const label = new THREE.Mesh(
    keep(new THREE.PlaneGeometry(0.42, 0.21)),
    keep(
      new THREE.MeshBasicMaterial({
        map: keep(labelTexture(THREE)),
        transparent: true,
        depthWrite: false,
        toneMapped: false,
      }),
    ),
  );
  label.position.set(Math.sin(labelAngle) * 1.006, LINE_Y + 0.12, Math.cos(labelAngle) * 1.006);
  label.rotation.y = labelAngle;
  spinner.add(label);

  // Coin meshes: one instanced mesh, a slot per coin.
  const face = keep(coinFaceTexture(THREE));
  const reeds = keep(reedTexture(THREE));
  const faceMat = keep(
    new THREE.MeshStandardMaterial({ map: face, bumpMap: face, bumpScale: 2.5, metalness: 1, roughness: 0.3 }),
  );
  const sideMat = keep(
    new THREE.MeshStandardMaterial({ color: 0xffffff, bumpMap: reeds, bumpScale: 2, metalness: 1, roughness: 0.36 }),
  );
  const coinGeo = keep(new THREE.CylinderGeometry(COIN_R, COIN_R, COIN_H, 48));
  const coinMesh = new THREE.InstancedMesh(coinGeo, [sideMat, faceMat, faceMat], CAPACITY);
  coinMesh.instanceMatrix.setUsage(THREE.DynamicDrawUsage);
  coinMesh.frustumCulled = false;
  const gold = new THREE.Color(GOLD);
  const green = new THREE.Color(GREEN);
  const m = new THREE.Matrix4();
  const zero = new THREE.Matrix4().makeScale(0, 0, 0);
  for (let i = 0; i < CAPACITY; i++) {
    coinMesh.setColorAt(i, rand() < 0.3 ? green : gold);
    coinMesh.setMatrixAt(i, zero);
  }
  if (coinMesh.instanceColor) coinMesh.instanceColor.needsUpdate = true;
  spinner.add(coinMesh);

  // The lid.
  const lid = new THREE.Group();
  const lidProfile = [
    [0, 0.3], [0.81, 0.3], [0.81, 0.02], [0.815, 0.004], [0.85, 0], [0.868, 0.012], [0.876, 0.04],
    [0.876, 0.26], [0.872, 0.295], [0.85, 0.322], [0.8, 0.335], [0.6, 0.342], [0, 0.345],
  ].map(([x, y]) => new THREE.Vector2(x, y));
  const lidMat = keep(
    new THREE.MeshPhysicalMaterial({
      color: LID,
      metalness: 0.2,
      roughness: 0.34,
      clearcoat: 0.8,
      clearcoatRoughness: 0.2,
      bumpMap: keep(knurlTexture(THREE, 6 / 12, 7 / 12)),
      bumpScale: 3,
    }),
  );
  lid.add(new THREE.Mesh(keep(new THREE.LatheGeometry(lidProfile, 160)), lidMat));
  const slot = new THREE.Mesh(
    keep(new RoundedBoxGeometry(0.58, 0.02, 0.085, 2, 0.01)),
    keep(new THREE.MeshStandardMaterial({ color: "#22150a", roughness: 0.85 })),
  );
  slot.position.y = 0.343;
  lid.add(slot);
  lid.position.y = LID_Y;
  spinner.add(lid);

  // Physics: a floor and a ring of wall panels inside the glass. The lid has
  // no collider, so coins pass through its slot.
  const world = new RAPIER.World({ x: 0, y: GRAVITY, z: 0 });
  world.timestep = STEP;
  const staticGroups = groups(GROUP_STATIC, GROUP_COIN | GROUP_ENTERING);
  world.createCollider(
    RAPIER.ColliderDesc.cuboid(1.3, 0.1, 1.3)
      .setTranslation(0, BASE_TOP - 0.1, 0)
      .setFriction(0.6)
      .setCollisionGroups(staticGroups),
  );
  const PANELS = 32;
  const wallR = INNER_R + 0.06;
  const halfW = ((Math.PI * 2 * wallR) / PANELS) * 0.56;
  for (let i = 0; i < PANELS; i++) {
    const a = (i / PANELS) * Math.PI * 2;
    // Turned so the panel's thin axis points at the jar's centre.
    const turn = new THREE.Quaternion().setFromAxisAngle(new THREE.Vector3(0, 1, 0), a);
    world.createCollider(
      RAPIER.ColliderDesc.cuboid(halfW, 1.5, 0.06)
        .setTranslation(Math.sin(a) * wallR, BASE_TOP + 1.5, Math.cos(a) * wallR)
        .setRotation({ x: turn.x, y: turn.y, z: turn.z, w: turn.w })
        .setFriction(0.3)
        .setCollisionGroups(staticGroups),
    );
  }

  const coins: Coin[] = [];
  const free = Array.from({ length: CAPACITY }, (_, i) => CAPACITY - 1 - i);
  const tmpQ = new THREE.Quaternion();
  const tmpE = new THREE.Euler();

  const spawn = (
    pos: { x: number; y: number; z: number },
    rot: Three.Quaternion,
    vel: { x: number; y: number; z: number },
    spin: { x: number; y: number; z: number },
    entering: boolean,
  ): Coin | null => {
    const index = free.pop();
    if (index === undefined) return null;
    const desc = RAPIER.RigidBodyDesc.dynamic()
      .setTranslation(pos.x, pos.y, pos.z)
      .setRotation({ x: rot.x, y: rot.y, z: rot.z, w: rot.w })
      .setLinvel(vel.x, vel.y, vel.z)
      .setAngvel(spin)
      .setCcdEnabled(true)
      .setLinearDamping(0.15)
      .setAngularDamping(0.4);
    if (entering) desc.lockRotations();
    const body = world.createRigidBody(desc);
    const collider = world.createCollider(
      RAPIER.ColliderDesc.cylinder(COIN_H / 2, COIN_R)
        .setFriction(0.55)
        .setRestitution(0.12)
        .setDensity(8)
        .setCollisionGroups(
          entering ? groups(GROUP_ENTERING, GROUP_STATIC) : groups(GROUP_COIN, GROUP_STATIC | GROUP_COIN),
        ),
      body,
    );
    const coin = { index, body, collider, entering };
    coins.push(coin);
    return coin;
  };

  const remove = (coin: Coin) => {
    world.removeRigidBody(coin.body);
    coinMesh.setMatrixAt(coin.index, zero);
    free.push(coin.index);
    coins.splice(coins.indexOf(coin), 1);
    // Let the pile settle into the gap.
    for (const c of coins) c.body.wakeUp();
  };

  /** A coin poured in from above the open jar. */
  const pourCoin = () => {
    const r = 0.42 * Math.sqrt(rand());
    const a = rand() * Math.PI * 2;
    tmpQ.setFromEuler(tmpE.set(rand() * Math.PI, rand() * Math.PI, rand() * Math.PI));
    return spawn(
      { x: Math.cos(a) * r, y: 3.7 + rand() * 0.4, z: Math.sin(a) * r },
      tmpQ,
      { x: 0, y: -3, z: 0 },
      { x: (rand() - 0.5) * 10, y: (rand() - 0.5) * 6, z: (rand() - 0.5) * 10 },
      false,
    );
  };

  /** A coin dropped through the lid's slot, upright and lined up with it. */
  const slotCoin = () => {
    tmpQ.setFromEuler(tmpE.set(Math.PI / 2, 0, 0));
    return spawn(
      { x: (rand() - 0.5) * 0.12, y: LID_TOP + 0.75, z: 0 },
      tmpQ,
      { x: 0, y: -2.6, z: 0 },
      { x: 0, y: 0, z: 0 },
      true,
    );
  };

  // Payout: coins leave the physics world and fly up out through the slot.
  type Leaving = { index: number; t0: number; from: Three.Vector3; fromQ: Three.Quaternion; tilt: number };
  const leaving: Leaving[] = [];
  const upright = new THREE.Quaternion().setFromEuler(new THREE.Euler(Math.PI / 2, 0, 0));
  const leave = (coin: Coin, now: number) => {
    const t = coin.body.translation();
    const r = coin.body.rotation();
    world.removeRigidBody(coin.body);
    coins.splice(coins.indexOf(coin), 1);
    leaving.push({
      index: coin.index,
      t0: now,
      from: new THREE.Vector3(t.x, t.y, t.z),
      fromQ: new THREE.Quaternion(r.x, r.y, r.z, r.w),
      tilt: (rand() - 0.5) * 0.1,
    });
  };
  const riseEase = gsap.parseEase("power2.in");
  const gatherEase = gsap.parseEase("power2.out");
  const syncLeaving = (now: number) => {
    const p = new THREE.Vector3();
    const one = new THREE.Vector3(1, 1, 1);
    for (let i = leaving.length - 1; i >= 0; i--) {
      const l = leaving[i];
      const k = (now - l.t0) / LEAVE_TIME;
      if (k >= 1) {
        coinMesh.setMatrixAt(l.index, zero);
        free.push(l.index);
        leaving.splice(i, 1);
        continue;
      }
      // First gather under the slot and stand upright, then rise out of it.
      const g = gatherEase(Math.min(1, k / 0.45));
      const under = LID_Y - 0.15;
      p.set(
        gsap.utils.interpolate(l.from.x, l.tilt, g),
        gsap.utils.interpolate(l.from.y, under, g),
        gsap.utils.interpolate(l.from.z, 0, g),
      );
      if (k > 0.45) p.y = under + riseEase((k - 0.45) / 0.55) * (LID_TOP + 1.1 - under);
      tmpQ.copy(l.fromQ).slerp(upright, g);
      coinMesh.setMatrixAt(l.index, m.compose(p, tmpQ, one));
    }
  };

  const syncMeshes = () => {
    const p = new THREE.Vector3();
    const one = new THREE.Vector3(1, 1, 1);
    for (const c of [...coins]) {
      const t = c.body.translation();
      if (t.y < -2) {
        remove(c);
        continue;
      }
      // Once through the lid, the coin joins the pile and may tumble.
      if (c.entering && t.y < LID_Y - 0.25) {
        c.entering = false;
        c.collider.setCollisionGroups(groups(GROUP_COIN, GROUP_STATIC | GROUP_COIN));
        c.body.lockRotations(false, true);
        c.body.setAngvel({ x: (rand() - 0.5) * 4, y: 0, z: (rand() - 0.5) * 4 }, true);
      }
      const r = c.body.rotation();
      p.set(t.x, t.y, t.z);
      tmpQ.set(r.x, r.y, r.z, r.w);
      coinMesh.setMatrixAt(c.index, m.compose(p, tmpQ, one));
    }
    coinMesh.instanceMatrix.needsUpdate = true;
  };

  // Sizing.
  const resize = () => {
    const { width, height } = host.getBoundingClientRect();
    if (!width || !height) return;
    renderer.setSize(width, height, false);
    camera.aspect = width / height;
    camera.updateProjectionMatrix();
  };
  const ro = new ResizeObserver(resize);
  ro.observe(host);
  resize();

  let inView = true;
  const io = new IntersectionObserver(([entry]) => {
    inView = entry.isIntersecting;
  });
  io.observe(host);

  // Money shown in the chips.
  let amount = 0;
  const setAmount = (value: number) => {
    amount = Math.round(value * 100) / 100;
    emit<JarAmountDetail>(JAR_AMOUNT_EVENT, { amount });
  };

  if (reduced) {
    // Settle a full pile up front and show it still.
    for (let batch = 0; batch < START_COINS / 14; batch++) {
      for (let k = 0; k < 14; k++) {
        const c = pourCoin();
        if (c) c.body.setTranslation({ ...c.body.translation(), y: 2.4 + k * 0.08 }, true);
      }
      for (let s = 0; s < 45; s++) world.step();
    }
    for (let s = 0; s < 240; s++) world.step();
    syncMeshes();
    renderer.compile(scene, camera);
    renderer.render(scene, camera);
    onReady();
    return () => {
      ro.disconnect();
      io.disconnect();
      world.free();
      coinMesh.dispose();
      for (const d of disposables) d.dispose();
      renderer.dispose();
      canvas.remove();
    };
  }

  // The script: pour $8.40, lid on, payments up to $12, payout to Cleva,
  // then fill from $0 again, forever.
  const calls = new Set<gsap.core.Tween>();
  const later = (seconds: number, fn: () => void) => {
    const call = gsap.delayedCall(seconds, () => {
      calls.delete(call);
      fn();
    });
    calls.add(call);
  };

  lid.visible = false;
  const lidIn = gsap.timeline({ paused: true });
  lidIn
    .set(lid, { visible: true })
    .fromTo(lid.position, { y: LID_Y + 0.9 }, { y: LID_Y, duration: 0.7, ease: "power3.out" }, 0)
    .fromTo(lid.rotation, { y: -1.1 }, { y: 0, duration: 0.9, ease: "power2.out" }, 0);

  for (let i = 0; i < START_COINS; i++) {
    later(POUR_AT + (i / START_COINS) * POUR_SPREAD, () => {
      if (pourCoin()) setAmount(amount + COIN_VALUE);
    });
  }
  later(LID_AT, () => lidIn.play(0));

  /** Plays a list of payments in turn, then pays out. */
  const payments = (list: JarPaymentDetail[], n: number) => {
    if (n >= list.length) {
      later(0.4, payout);
      return;
    }
    const pay = list[n];
    const count = Math.round(pay.amount / COIN_VALUE);
    emit<JarPaymentDetail>(JAR_PAYMENT_EVENT, pay);
    for (let k = 0; k < count; k++) {
      later(k * SLOT_EVERY, () => {
        if (slotCoin()) setAmount(amount + COIN_VALUE);
      });
    }
    later(count * SLOT_EVERY + AFTER_PAYMENT, () => payments(list, n + 1));
  };

  /** At $12 the whole jar sweeps to Cleva, less the flat fee, top coins first. */
  const payout = () => {
    emit<JarPayoutDetail>(JAR_PAYOUT_EVENT, { amount: THRESHOLD - FEE, fee: FEE });
    const order = [...coins].sort((a, b) => b.body.translation().y - a.body.translation().y);
    order.forEach((coin, k) => {
      later(k * LEAVE_EVERY, () => {
        if (!coins.includes(coin)) return;
        leave(coin, performance.now() / 1000);
        setAmount(Math.max(0, amount - COIN_VALUE));
        // Wake the pile so it slumps as the top empties.
        if (k % 10 === 0) for (const c of coins) c.body.wakeUp();
      });
    });
    later(order.length * LEAVE_EVERY + LEAVE_TIME + AFTER_PAYOUT, () => {
      setAmount(0);
      payments(CYCLE_PAYMENTS, 0);
    });
  };

  later(FIRST_PAYMENT, () => payments(FIRST_PAYMENTS, 0));

  // The jar always turns. Dragging it (mouse or finger) spins it; on release
  // it keeps that momentum and eases back to a steady turn that way.
  const stage = host.parentElement ?? host;
  let spinVel = BASE_SPIN;
  let spinDir = 1;
  let dragging = false;
  let lastX = 0;
  let lastMove = 0;
  let dragVel = 0;
  const onDown = (e: PointerEvent) => {
    if (e.pointerType === "mouse" && e.button !== 0) return;
    dragging = true;
    lastX = e.clientX;
    lastMove = performance.now();
    dragVel = 0;
    stage.setPointerCapture(e.pointerId);
    stage.style.cursor = "grabbing";
  };
  const onMove = (e: PointerEvent) => {
    if (!dragging) return;
    const now = performance.now();
    const turn = (e.clientX - lastX) * DRAG_RATE;
    spinner.rotation.y += turn;
    const dt = Math.max(8, now - lastMove) / 1000;
    dragVel = dragVel * 0.5 + (turn / dt) * 0.5;
    lastX = e.clientX;
    lastMove = now;
  };
  const onUp = () => {
    if (!dragging) return;
    dragging = false;
    stage.style.cursor = "grab";
    // A drag that stopped before release leaves no momentum.
    const v = performance.now() - lastMove > 120 ? 0 : dragVel;
    if (Math.abs(v) > 0.05) {
      spinVel = gsap.utils.clamp(-8, 8, v);
      spinDir = Math.sign(v);
    }
  };
  stage.style.touchAction = "pan-y";
  stage.style.cursor = "grab";
  stage.addEventListener("pointerdown", onDown);
  stage.addEventListener("pointermove", onMove);
  stage.addEventListener("pointerup", onUp);
  stage.addEventListener("pointercancel", onUp);

  let acc = 0;
  let last = performance.now() / 1000;
  const tick = () => {
    const now = performance.now() / 1000;
    const dt = Math.min(0.05, now - last);
    last = now;

    if (!dragging) {
      spinVel += (BASE_SPIN * spinDir - spinVel) * (1 - Math.exp(-dt * 1.4));
      spinner.rotation.y += spinVel * dt;
    }

    acc += dt;
    let steps = 0;
    while (acc >= STEP && steps < 3) {
      world.step();
      acc -= STEP;
      steps++;
    }
    if (steps === 3) acc = 0;
    syncLeaving(now);
    syncMeshes();

    if (inView) renderer.render(scene, camera);
  };

  renderer.compile(scene, camera);
  renderer.render(scene, camera);
  onReady();
  gsap.ticker.add(tick);
  emit(JAR_POUR_EVENT, { delay: POUR_AT, duration: POUR_SPREAD });

  return () => {
    gsap.ticker.remove(tick);
    for (const c of calls) c.kill();
    lidIn.kill();
    stage.removeEventListener("pointerdown", onDown);
    stage.removeEventListener("pointermove", onMove);
    stage.removeEventListener("pointerup", onUp);
    stage.removeEventListener("pointercancel", onUp);
    stage.style.cursor = "";
    stage.style.touchAction = "";
    ro.disconnect();
    io.disconnect();
    world.free();
    coinMesh.dispose();
    for (const d of disposables) d.dispose();
    renderer.dispose();
    canvas.remove();
  };
}

function seeded(seed: number) {
  let a = seed;
  return () => {
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

type ThreeModule = typeof Three;

function canvasTexture(THREE: ThreeModule, w: number, h: number, draw: (g: CanvasRenderingContext2D) => void) {
  const c = document.createElement("canvas");
  c.width = w;
  c.height = h;
  const g = c.getContext("2d");
  if (!g) throw new Error("2d canvas unavailable");
  draw(g);
  const tex = new THREE.CanvasTexture(c);
  tex.anisotropy = 4;
  return tex;
}

function shadowTexture(THREE: ThreeModule) {
  return canvasTexture(THREE, 256, 256, (g) => {
    const grad = g.createRadialGradient(128, 128, 0, 128, 128, 128);
    grad.addColorStop(0, "rgba(19,38,58,0.42)");
    grad.addColorStop(0.42, "rgba(19,38,58,0.26)");
    grad.addColorStop(0.62, "rgba(19,38,58,0.08)");
    grad.addColorStop(1, "rgba(19,38,58,0)");
    g.fillStyle = grad;
    g.fillRect(0, 0, 256, 256);
  });
}

function labelTexture(THREE: ThreeModule) {
  const family = getComputedStyle(document.body).getPropertyValue("--font-space-grotesk").trim() || "system-ui";
  const tex = canvasTexture(THREE, 256, 128, (g) => {
    g.fillStyle = "rgba(19,38,58,0.78)";
    g.font = `600 64px ${family}, system-ui, sans-serif`;
    g.textBaseline = "alphabetic";
    g.fillText("$12", 8, 92);
  });
  tex.colorSpace = THREE.SRGBColorSpace;
  return tex;
}

/** A raised rim and a "$" on the face, light where raised. */
function coinFaceTexture(THREE: ThreeModule) {
  const tex = canvasTexture(THREE, 256, 256, (g) => {
    g.fillStyle = "#d9d9d9";
    g.fillRect(0, 0, 256, 256);
    g.strokeStyle = "#ffffff";
    g.lineWidth = 16;
    g.beginPath();
    g.arc(128, 128, 112, 0, Math.PI * 2);
    g.stroke();
    g.strokeStyle = "#bfbfbf";
    g.lineWidth = 3;
    g.beginPath();
    g.arc(128, 128, 98, 0, Math.PI * 2);
    g.stroke();
    g.fillStyle = "#f4f4f4";
    g.font = "700 132px system-ui, sans-serif";
    g.textAlign = "center";
    g.textBaseline = "middle";
    g.fillText("$", 128, 136);
  });
  tex.colorSpace = THREE.SRGBColorSpace;
  return tex;
}

/** Fine ridges around a coin's edge. */
function reedTexture(THREE: ThreeModule) {
  const tex = canvasTexture(THREE, 512, 8, (g) => {
    for (let x = 0; x < 512; x += 4) {
      g.fillStyle = "#ffffff";
      g.fillRect(x, 0, 2, 8);
      g.fillStyle = "#808080";
      g.fillRect(x + 2, 0, 2, 8);
    }
  });
  tex.wrapS = THREE.RepeatWrapping;
  return tex;
}

/** Vertical grip ridges, only on the lid's side band (v between v0 and v1). */
function knurlTexture(THREE: ThreeModule, v0: number, v1: number) {
  const H = 256;
  return canvasTexture(THREE, 1024, H, (g) => {
    g.fillStyle = "#808080";
    g.fillRect(0, 0, 1024, H);
    const top = Math.round((1 - v1) * H) + 1;
    const bottom = Math.round((1 - v0) * H) - 1;
    for (let x = 0; x < 1024; x += 8) {
      g.fillStyle = "#ffffff";
      g.fillRect(x, top, 4, bottom - top);
      g.fillStyle = "#606060";
      g.fillRect(x + 4, top, 4, bottom - top);
    }
  });
}
