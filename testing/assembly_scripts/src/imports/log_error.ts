import { log } from "@graphprotocol/graph-ts"

export function main(): void {
  log.error("log error {} - {}", ["abc", "123"])
}
