public interface Store {
    String get(String key);
}

class StoreUser {
    static String use(Store s) {
        return s.get("x");
    }
}
