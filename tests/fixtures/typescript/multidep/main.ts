import { readFileSync } from "node:fs";
import { getExampleText as fixtureDep1 } from "./pkg/dep1";

export function main(): string {
  return readFileSync(fixtureDep1(), "utf8");
}
