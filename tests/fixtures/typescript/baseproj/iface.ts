interface Store {
  get(key: string): string;
}

function use(s: Store): string {
  return s.get("x");
}
