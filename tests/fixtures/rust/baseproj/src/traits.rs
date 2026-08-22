pub trait Store {
    fn get(&self, key: &str) -> String;
}

pub fn use_store(s: &dyn Store) -> String {
    s.get("x")
}
