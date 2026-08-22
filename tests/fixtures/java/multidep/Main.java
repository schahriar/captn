import java.util.List;

import pkg.Dep1;

public class Main {
    public static void main(String[] args) {
        System.out.println(Dep1.getExampleText());
    }

    static List<String> examples() {
        return List.of(Dep1.getExampleText());
    }
}
