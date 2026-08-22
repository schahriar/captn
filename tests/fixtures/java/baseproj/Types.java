import java.util.List;
import java.util.Map;

public class Types {
    Map<String, List<Widget>> index;
    Widget[] all;

    static Widget first(Widget[] all) {
        return all[0];
    }

    static Map.Entry<String, Widget> entry(Map<String, Widget> m) {
        return null;
    }

    enum Color {
        RED,
        GREEN
    }

    record Point(int x, int y) {
    }
}
