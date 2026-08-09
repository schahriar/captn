import json
import pkg.dep1 as fixture_dep1


def main() -> None:
    print(json.dumps(fixture_dep1.get_example_text()))
