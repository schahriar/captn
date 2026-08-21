class Widget {
  id: string = "w";

  describe(prefix: string): string {
    return prefix + this.id;
  }
}

function label(w: Widget): string {
  return w.describe("widget:");
}
