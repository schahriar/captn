const double = (v) => v * 2;

function apply(values) {
  return values.map((v) => double(v));
}
