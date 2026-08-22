pub fn map<T, U>(items: Vec<T>, f: impl Fn(T) -> U) -> Vec<U> {
    let mut out = Vec::new();
    for item in items {
        out.push(f(item));
    }
    out
}
