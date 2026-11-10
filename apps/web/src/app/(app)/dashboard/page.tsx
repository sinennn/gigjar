import { ScreenScaffold } from "@/components/screen-scaffold";

export const metadata = { title: "Overview" };

export default function DashboardPage() {
  return (
    <ScreenScaffold
      title={"Overview"}
      summary={"Everything your gigs have earned, at a glance."}
      mustShow={["Total jar balance (available + pending)", "Available balance", "Pending payments, labelled as not yet confirmed", "Amount remaining until automatic settlement, with the threshold", "Recent payments", "Latest settlement and its state", "A clear Simulated badge when the network is simulated", "A prompt to add a Cleva address if none is set"]}
      endpoints={["GET /v1/jar", "GET /v1/incoming-transfers?limit=5", "GET /v1/settlements?limit=1"]}
    />
  );
}
