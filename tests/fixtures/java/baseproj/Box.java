public class Box<T> {
    T value;

    T get() {
        return value;
    }

    static <U extends Comparable<U>> U pick(U a, U b) {
        return a.compareTo(b) > 0 ? a : b;
    }
}
