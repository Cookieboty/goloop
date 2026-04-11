export function PageTitle({ children }: { children: React.ReactNode }) {
  return (
    <h1
      style={{
        fontSize: 20,
        fontWeight: "bold",
        marginBottom: 24,
        color: "var(--text)",
      }}
    >
      {children}
    </h1>
  );
}

export function SectionTitle({ children }: { children: React.ReactNode }) {
  return (
    <h2
      style={{
        fontSize: 14,
        fontWeight: "bold",
        marginBottom: 16,
        color: "var(--text)",
      }}
    >
      {children}
    </h2>
  );
}
