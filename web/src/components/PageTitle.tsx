export function PageTitle({ children }: { children: React.ReactNode }) {
  return (
    <h1 className="text-3xl font-bold text-white tracking-tight">
      {children}
    </h1>
  );
}

export function SectionTitle({ children }: { children: React.ReactNode }) {
  return (
    <h2 className="text-lg font-semibold text-white mb-4">
      {children}
    </h2>
  );
}
