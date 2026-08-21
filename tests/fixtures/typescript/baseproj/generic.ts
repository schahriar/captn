function mapValues<T, U>(s: T[], f: (v: T) => U): U[] {
  return s.map(f);
}
