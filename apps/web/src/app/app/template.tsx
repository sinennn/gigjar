/** A fresh wrapper per navigation, so each screen eases in. */
export default function AppTemplate({ children }: { children: React.ReactNode }) {
  return <div className="page-in">{children}</div>;
}
