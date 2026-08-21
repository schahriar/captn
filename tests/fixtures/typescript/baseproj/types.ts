class Gadget {
  serial: string = "";
}

enum Mode {
  Idle,
  Busy = "busy",
}

type GadgetAlias = Gadget;

type GadgetPair = { left: Gadget; right: Gadget };

type Labeler<T extends Gadget> = (v: T) => string;

interface Inventory extends Iterable<Gadget> {
  count: number;
  find(serial: string): Gadget | undefined;
}

const describe: Labeler<Gadget> = (v) => v.serial;

const { serial: firstSerial } = new Gadget();

function tally(pair: GadgetPair, mode: Mode): GadgetAlias {
  return mode === Mode.Idle ? pair.left : pair.right;
}
