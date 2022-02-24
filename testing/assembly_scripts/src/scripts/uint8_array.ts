export function main(input: Uint8Array): Uint8Array {
  let bytes = new Uint8Array(3)
  bytes[0] = 0xe6
  bytes[1] = 0xf5
  bytes[2] = 0xaf

  return bytes
}
